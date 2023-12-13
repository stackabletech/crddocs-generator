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
	"strings"

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

	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		panic(err)
	}

	gitter := &Gitter{
		db: db,
	}

	var conf config.Config

	err = conf.NewConfigFromFile(configFile)
	if err != nil {
		log.Fatalf("Error loading config: %s: %v", configFile, err)
		panic(err)
	}

	var gitterRepos []models.GitterRepo

	// Extract information from GitterRepoConfig and create GitterRepo instances
	for repo, tags := range conf.Repos {
		for _, tag := range tags {
			gitterRepo := models.GitterRepo{
				Org:  "stackabletech", // TODO maybe we could remove this entirely?
				Repo: repo,
				Tag:  tag,
			}
			log.Printf("Found repo in config: %+v\n", gitterRepo)
			gitterRepos = append(gitterRepos, gitterRepo)
		}
	}

	for _, repo := range gitterRepos {
		fmt.Printf("Indexing repo: %+v\n", repo)
		// Call the Index method on the Gitter instance
		var replyString string
		err = gitter.Index(repo, &replyString)

		// Check for errors
		if err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("Reply:", replyString)
		}
	}
}

// Gitter indexes git repos.
type Gitter struct {
	db *sql.DB
}

// Index indexes a git repo at the specified url.
func (g *Gitter) Index(gRepo models.GitterRepo, reply *string) error {
	log.Printf("Indexing repo %s/%s...\n", gRepo.Org, gRepo.Repo)

	dir, err := os.MkdirTemp(os.TempDir(), "doc-gitter")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	fullRepo := fmt.Sprintf("%s/%s/%s", "github.com", strings.ToLower(gRepo.Org), strings.ToLower(gRepo.Repo))
	cloneOpts := &git.CloneOptions{
		URL:               fmt.Sprintf("https://%s", fullRepo),
		Depth:             1,
		Progress:          os.Stdout,
		RecurseSubmodules: git.NoRecurseSubmodules,
		ReferenceName:     plumbing.NewTagReferenceName(gRepo.Tag),
		SingleBranch:      true,
	}
	if gRepo.Tag == "nightly" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName("main")
	}
	repo, err := git.PlainClone(dir, false, cloneOpts)
	if err != nil {
		log.Printf("Failed to clone repo: %v", err)
		return err
	}
	h, err := repo.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil || h == nil {
		log.Printf("Unable to resolve revision: %s (%v)", gRepo.Tag, err)
		return err
	}
	c, err := repo.CommitObject(*h)
	if err != nil || c == nil {
		log.Printf("Unable to resolve revision: %s (%v)", gRepo.Tag, err)
		return err
	}
	time := c.Committer.When
	if gRepo.Tag == "nightly" {
		time = time.AddDate(-50, 0, 0) // backdate the nightly so it comes last in the sorting
	}
	var tagID int
	r := g.db.QueryRow("INSERT INTO tags(name, repo, time) VALUES ($1, $2, $3) RETURNING id", gRepo.Tag, fullRepo, time)
	if err := r.Scan(&tagID); err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		log.Printf("Failed to get worktree: %v", err)
		return err
	}
	repoCRDs, err := getCRDsFromTag(dir, w)
	if err != nil {
		log.Printf("Unable to get CRDs: %s@%s (%v)", repo, gRepo.Tag, err)
		return err
	}
	log.Printf("Found %d CRDs", len(repoCRDs))
	if len(repoCRDs) > 0 {
		allArgs := make([]interface{}, 0, len(repoCRDs)*crdArgCount)
		for _, crd := range repoCRDs {
			allArgs = append(allArgs, crd.Group, crd.Version, crd.Kind, tagID, crd.Filename, crd.CRD)
		}
		if _, err := g.db.Exec(buildInsert("INSERT INTO crds(\"group\", version, kind, tag_id, filename, data) VALUES ", crdArgCount, len(repoCRDs))+"ON CONFLICT DO NOTHING", allArgs...); err != nil {
			return err
		}
	}

	log.Printf("Finished indexing %s/%s\n", gRepo.Org, gRepo.Repo)

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
