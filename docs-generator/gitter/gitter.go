/*
Copyright 2020 The CRDS Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"regexp"

	"docs-generator/pkg/config"
	"docs-generator/pkg/crd"
	"docs-generator/pkg/models"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/square/go-jose.v2/json"
	yaml "gopkg.in/yaml.v3"
)

const (
	crdArgCount = 6
)

func main() {
	// Define command-line flags
	var dbFile string
	var configFile string

	flag.StringVar(&dbFile, "db", "", "Specify an SQLite3 database with the correct tables initialized")
	flag.StringVar(&configFile, "config", "", "Specify a yaml config file containing the repos to index")

	flag.Parse()

	// Check for mandatory flags
	if dbFile == "" || configFile == "" {
		fmt.Println("Error: db and config flags are required.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// open database
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		panic(err)
	}

	// read config
	var conf config.Config
	err = conf.NewConfigFromFile(configFile)
	if err != nil {
		log.Fatalf("Error loading config: %s: %v", configFile, err)
		panic(err)
	}

	// index repos
	for repo, tags := range conf.Repos {
		log.Printf("Indexing repo %s ...\n", repo)
		for _, tag := range tags {
			log.Printf("... at tag: %s ...\n", tag)
			err = Index(db, repo, tag)
			// Check for errors
			if err != nil {
				fmt.Println("Error:", err)
			}
		}
	}
}

// Index indexes a git repo at the specified url.
func Index(db *sql.DB, repo string, tag string) error {
	dir, err := os.MkdirTemp(os.TempDir(), "doc-gitter")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cloneOpts := &git.CloneOptions{
		URL:               fmt.Sprintf("https://github.com/stackabletech/%s", repo),
		Depth:             1,
		Progress:          nil, // suppress progress output as it clogs up stdout otherwise
		RecurseSubmodules: git.NoRecurseSubmodules,
		ReferenceName:     plumbing.NewTagReferenceName(tag),
		SingleBranch:      true,
	}
	if tag == "nightly" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName("main")
	}
	gitRepo, err := git.PlainClone(dir, false, cloneOpts)
	if err != nil {
		log.Printf("Failed to clone repo: %v", err)
		return err
	}
	h, err := gitRepo.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil || h == nil {
		log.Printf("Unable to resolve revision: %s (%v)", tag, err)
		return err
	}
	c, err := gitRepo.CommitObject(*h)
	if err != nil || c == nil {
		log.Printf("Unable to resolve revision: %s (%v)", tag, err)
		return err
	}
	time := c.Committer.When
	if tag == "nightly" {
		time = time.AddDate(-50, 0, 0) // backdate the nightly so it comes last in the sorting
	}
	var tagID int
	r := db.QueryRow("INSERT INTO tags(name, repo, time) VALUES ($1, $2, $3) RETURNING id", tag, repo, time)
	if err := r.Scan(&tagID); err != nil {
		return err
	}
	w, err := gitRepo.Worktree()
	if err != nil {
		log.Printf("Failed to get worktree: %v", err)
		return err
	}
	repoCRDs, err := getCRDsFromTag(dir, w)
	if err != nil {
		log.Printf("Unable to get CRDs: %s@%s (%v)", repo, tag, err)
		return err
	}
	log.Printf("Found %d CRDs", len(repoCRDs))
	if len(repoCRDs) > 0 {
		allArgs := make([]interface{}, 0, len(repoCRDs)*crdArgCount)
		for _, crd := range repoCRDs {
			allArgs = append(allArgs, crd.Group, crd.Version, crd.Kind, tagID, crd.Filename, crd.CRD)
		}
		if _, err := db.Exec(buildInsert("INSERT INTO crds(\"group\", version, kind, tag_id, filename, data) VALUES ", crdArgCount, len(repoCRDs))+"ON CONFLICT DO NOTHING", allArgs...); err != nil {
			return err
		}
	}

	return nil
}

func getCRDsFromTag(dir string, w *git.Worktree) (map[string]models.RepoCRD, error) {
	reg := regexp.MustCompile("kind: CustomResourceDefinition")
	regPath := regexp.MustCompile(`^deploy/helm/.*\.yaml`)
	g, _ := w.Grep(&git.GrepOptions{
		Patterns:  []*regexp.Regexp{reg},
		PathSpecs: []*regexp.Regexp{regPath},
	})
	repoCRDs := map[string]models.RepoCRD{}
	files := getYAMLs(g, dir)
	log.Printf("found files: %d", len(files))
	for file, yamls := range files {
		for _, y := range yamls {
			crder, err := crd.NewCRDer(y, crd.StripLabels(), crd.StripAnnotations(), crd.StripConversion())
			if err != nil || crder.CRD == nil {
				log.Printf("error: %v", err)
				continue
			}
			cbytes, err := json.Marshal(crder.CRD)
			if err != nil {
				log.Printf("Error marshalling: %v", err)
				continue
			}
			repoCRDs[crd.PrettyGVK(crder.GVK)] = models.RepoCRD{
				Path:     crd.PrettyGVK(crder.GVK),
				Filename: path.Base(file),
				Group:    crder.GVK.Group,
				Version:  crder.GVK.Version,
				Kind:     crder.GVK.Kind,
				CRD:      cbytes,
			}
		}
	}
	return repoCRDs, nil
}

func getYAMLs(greps []git.GrepResult, dir string) map[string][][]byte {
	allCRDs := map[string][][]byte{}
	for _, res := range greps {
		b, err := os.ReadFile(dir + "/" + res.FileName)
		if err != nil {
			log.Printf("failed to read CRD file: %s", res.FileName)
			continue
		}

		yamls, err := splitYAML(b, res.FileName)
		if err != nil {
			log.Printf("failed to split/parse CRD file: %s", res.FileName)
			continue
		}

		allCRDs[res.FileName] = yamls
	}
	return allCRDs
}

func splitYAML(file []byte, filename string) ([][]byte, error) {
	var yamls [][]byte
	var err error = nil
	defer func() {
		if err := recover(); err != nil {
			yamls = make([][]byte, 0)
			err = fmt.Errorf("panic while processing yaml file: %v", err)
		}
	}()

	decoder := yaml.NewDecoder(bytes.NewReader(file))
	for {
		var node map[string]interface{}
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("failed to decode part of CRD file: %s\n%s", filename, err)
			continue
		}

		doc, err := yaml.Marshal(node)
		if err != nil {
			log.Printf("failed to encode part of CRD file: %s\n%s", filename, err)
			continue
		}
		yamls = append(yamls, doc)
	}
	return yamls, err
}

func buildInsert(query string, argsPerInsert, numInsert int) string {
	absArg := 1
	for i := 0; i < numInsert; i++ {
		query += "("
		for j := 0; j < argsPerInsert; j++ {
			query += "$" + fmt.Sprint(absArg)
			if j != argsPerInsert-1 {
				query += ","
			}
			absArg++
		}
		query += ")"
		if i != numInsert-1 {
			query += ","
		}
	}
	return query
}
