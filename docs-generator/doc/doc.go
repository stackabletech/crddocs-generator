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
//go:debug jstmpllitinterp=1

// The go:debug statement above is needed for inserting into JS templates.
// This is unsafe if the inserted object is external, but it isn't in our case.
// more info here: https://pkg.go.dev/html/template#hdr-Security_Model
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	crdutil "docs-generator/pkg/crd"
	"docs-generator/pkg/models"

	_ "github.com/mattn/go-sqlite3"
	"github.com/unrolled/render"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"sigs.k8s.io/yaml"
)

// redis connection
var (
	envDevelopment = "IS_DEV"

	analytics bool = false
)

// SchemaPlusParent is a JSON schema plus the name of the parent field.
type SchemaPlusParent struct {
	Parent string
	Schema map[string]apiextensions.JSONSchemaProps
}

type pageData struct {
	Analytics     bool
	DisableNavBar bool
	IsDarkMode    bool
	Title         string
}

type docData struct {
	Page        pageData
	Repo        string
	Tag         string
	At          string
	Group       string
	Version     string
	Kind        string
	Description string
	Schema      apiextensions.JSONSchemaProps
}

type orgData struct {
	Page     pageData
	Repo     string
	Tag      string
	At       string
	Tags     []string
	CRDs     map[string]models.RepoCRD
	Total    int
	JsonData string
}

type homeRow struct {
	Repo      string
	RepoShort string
	Group     string
	Version   string
	Kind      string
}

type homeData struct {
	Page             pageData
	Tag              string
	PlatformVersions []string
	Rows             []homeRow
	JsonData         string
}

var page *render.Render

func main() {
	var dbFile string
	var configFile string
	var outDir string
	var templateDir string

	flag.StringVar(&dbFile, "db", "", "Specify an SQLite3 database with the correct tables initialized")
	flag.StringVar(&configFile, "config", "", "Specify a yaml config file containing the repos to index")
	flag.StringVar(&outDir, "out", "", "Specify the directory where the site should be generated")
	flag.StringVar(&templateDir, "template", "", "Specify where the template files are located")

	flag.Parse()

	// Check for mandatory flags
	if dbFile == "" || configFile == "" || outDir == "" || templateDir == "" {
		fmt.Println("Error: db, config, out and template flags are required.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// open database
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		panic(err)
	}

	// create output directory
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		log.Println("Error creating output directory:", err)
		return
	}

	// initialize renderer
	page = render.New(render.Options{
		Extensions:    []string{".html"},
		Directory:     templateDir,
		Layout:        "layout",
		IsDevelopment: os.Getenv(envDevelopment) == "true",
		Funcs: []template.FuncMap{
			{
				"plusParent": func(p string, s map[string]apiextensions.JSONSchemaProps) *SchemaPlusParent {
					return &SchemaPlusParent{
						Parent: p,
						Schema: s,
					}
				},
			},
		},
	})

	versions := []string{"23.11.0", "nightly"}

	// generate landing page
	home(db, outDir, "", versions)

	for _, v := range versions {
		home(db, outDir, v, versions)
	}

	// read config file
	yamlFile, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}

	var config map[string]map[string][]string

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
	}

	// generate doc pages for all repos and CRDs
	for orgname, repos := range config {
		for repo, tags := range repos {
			org(db, outDir, orgname, repo, "")
			for _, tag := range tags {
				org(db, outDir, orgname, repo, tag)
			}
		}
	}
}

func getPageData(title string, disableNavBar bool) pageData {
	var isDarkMode = false
	return pageData{
		Analytics:     analytics,
		IsDarkMode:    isDarkMode,
		DisableNavBar: disableNavBar,
		Title:         title,
	}
}

func fetchHomeRows(db *sql.DB, version string) []homeRow {
	c, err := db.Query("SELECT tags.repo, crds.\"group\", crds.version, crds.kind FROM crds JOIN tags ON crds.tag_id = tags.id WHERE tags.name = $1 ORDER BY crds.kind;", version)
	if err != nil {
		log.Printf("failed to get crds for %s: %v", version, err)
		panic(err) // something went wrong, there should be CRDs
	}
	rows := []homeRow{}
	for c.Next() {
		log.Printf("LALALA")
		var r, g, v, k string
		if err := c.Scan(&r, &g, &v, &k); err != nil {
			log.Printf("newTemplate.Execute(): %v", err)
		}
		rows = append(rows, homeRow{
			Repo:      r,
			RepoShort: strings.Split(r, "/")[2],
			Group:     g,
			Version:   v,
			Kind:      k,
		})
	}
	if c.Err() != nil {
		log.Printf("Error in Next: %s", err)
		panic(err)
	}
	return rows
}

func home(db *sql.DB, outDir string, version string, versions []string) {
	fullDir := outDir
	if version != "" {
		fullDir = fmt.Sprintf("%s/%s", outDir, version)
	}
	err := os.MkdirAll(fullDir, 0755)
	if err != nil {
		log.Println("Error creating output directory:", err)
		return
	}
	// Open the file for writing
	file, err := os.Create(fmt.Sprintf("%s/%s", fullDir, "index.html"))
	if err != nil {
		log.Printf("Error creating index.html: %v", err)
		return
	}
	defer file.Close()

	if version == "" {
		version = versions[0]
	}

	dataTmp := homeData{
		Page:             getPageData("Doc", false),
		Tag:              version,
		PlatformVersions: versions,
		Rows:             fetchHomeRows(db, version),
		JsonData:         "",
	}

	jsonData, err := json.Marshal(dataTmp)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	dataTmp.JsonData = string(jsonData)

	if err := page.HTML(file, http.StatusOK, "home", dataTmp); err != nil {
		log.Printf("homeTemplate.Execute(): %v", err)
		return
	}
	log.Print("successfully rendered home page")
}

func org(db *sql.DB, outDir string, org string, repo string, tag string) {
	fullDir := fmt.Sprintf("%s/%s", outDir, repo)
	if tag != "" {
		fullDir = fmt.Sprintf("%s/%s/%s", outDir, repo, tag)
	}
	err := os.MkdirAll(fullDir, 0755)
	if err != nil {
		log.Println("Error creating output directory:", err)
		return
	}

	// Open the file for writing
	file, err := os.Create(fmt.Sprintf("%s/%s", fullDir, "index.html"))
	if err != nil {
		log.Printf("Error creating index.html: %v", err)
		return
	}
	defer file.Close()

	pageData := getPageData(fmt.Sprintf("%s/%s", org, repo), false)
	fullRepo := fmt.Sprintf("%s/%s/%s", "github.com", org, repo)
	var c *sql.Rows
	if tag == "" {
		c, err = db.Query("SELECT t.name, c.'group', c.version, c.kind FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.id = (SELECT id FROM tags WHERE LOWER(repo) = LOWER($1) ORDER BY time DESC LIMIT 1);", fullRepo)
	} else {
		pageData.Title += fmt.Sprintf("@%s", tag)
		c, err = db.Query("SELECT t.name, c.'group', c.version, c.kind FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.name=$2;", fullRepo, tag)
	}
	if err != nil {
		log.Printf("failed to get CRDs for %s : %v", repo, err)
		panic(err)
	}
	repoCRDs := map[string]models.RepoCRD{}
	foundTag := tag
	for c.Next() {
		var t, g, v, k string
		if err := c.Scan(&t, &g, &v, &k); err != nil {
			log.Printf("newTemplate.Execute(): %v", err)
		}
		foundTag = t
		repoCRDs[g+"/"+v+"/"+k] = models.RepoCRD{
			Group:   g,
			Version: v,
			Kind:    k,
		}
		// TODO I'm not happy about calling this function here, I'd rather call it in a different loop in main
		// but it works for now
		doc(db, outDir, org, repo, tag, g, k, v)
	}
	if c.Err() != nil {
		log.Printf("Error in Next: %s", err)
		panic(err)
	}
	c, err = db.Query("SELECT name FROM tags WHERE LOWER(repo)=LOWER($1) ORDER BY time DESC;", fullRepo)
	if err != nil {
		log.Printf("failed to get tags for %s : %v", repo, err)
		panic(err) // something went wrong, there should be tags
	}
	tags := []string{}
	tagExists := false
	for c.Next() {
		var t string
		if err := c.Scan(&t); err != nil {
			log.Printf("newTemplate.Execute(): %v", err)
		}
		if !tagExists && t == tag {
			tagExists = true
		}
		tags = append(tags, t)
	}
	if len(tags) == 0 || (!tagExists && tag != "") {
		panic("This shouldn't happend!")
	}
	if foundTag == "" {
		foundTag = tags[0]
	}

	orgDataTmp := orgData{
		Page:     pageData,
		Repo:     strings.Join([]string{org, repo}, "/"),
		Tag:      foundTag,
		Tags:     tags,
		CRDs:     repoCRDs,
		Total:    len(repoCRDs),
		JsonData: "",
	}

	jsonData, err := json.Marshal(orgDataTmp)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	orgDataTmp.JsonData = string(jsonData)

	if err := page.HTML(file, http.StatusOK, "org", orgDataTmp); err != nil {
		log.Printf("orgTemplate.Execute(): %v", err)
		return
	}
	log.Printf("successfully rendered org template")
}

func doc(db *sql.DB, outDir string, org string, repo string, tag string, group string, kind string, version string) {
	fullDir := fmt.Sprintf("%s/%s/%s/%s/%s", outDir, tag, group, kind, version)
	err := os.MkdirAll(fullDir, 0755)
	if err != nil {
		log.Println("Error creating output directory:", err)
		return
	}

	// Open the file for writing
	file, err := os.Create(fmt.Sprintf("%s/%s", fullDir, "index.html"))
	if err != nil {
		log.Printf("Error creating index.html: %v", err)
		return
	}
	defer file.Close()

	pageData := getPageData(fmt.Sprintf("%s.%s/%s", kind, group, version), false)
	fullRepo := fmt.Sprintf("%s/%s/%s", "github.com", org, repo)
	var c *sql.Row
	if tag == "" {
		c = db.QueryRow("SELECT t.name, c.data FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.id = (SELECT id FROM tags WHERE repo = $1 ORDER BY time DESC LIMIT 1) AND c.\"group\"=$2 AND c.version=$3 AND c.kind=$4;", fullRepo, group, version, kind)
	} else {
		c = db.QueryRow("SELECT t.name, c.data FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.name=$2 AND c.'group'=$3 AND c.version=$4 AND c.kind=$5;", fullRepo, tag, group, version, kind)
	}
	foundTag := tag
	var crdJSON string
	if err := c.Scan(&foundTag, &crdJSON); err != nil {
		log.Printf("failed to get CRDs for %s : %v", repo, err)
		panic(err)
	}
	crd := &apiextensions.CustomResourceDefinition{}
	err = json.Unmarshal([]byte(crdJSON), &crd)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return
	}
	var schema *apiextensions.CustomResourceValidation
	schema = crd.Spec.Validation
	if len(crd.Spec.Versions) > 1 {
		for _, version := range crd.Spec.Versions {
			if version.Storage {
				if version.Schema != nil {
					schema = version.Schema
				}
				break
			}
		}
	}

	if schema == nil || schema.OpenAPIV3Schema == nil {
		log.Print("CRD schema is nil.")
		return
	}

	gvk := crdutil.GetStoredGVK(crd)
	if gvk == nil {
		log.Print("CRD GVK is nil.")
		return
	}

	if err := page.HTML(file, http.StatusOK, "doc", docData{
		Page:        pageData,
		Repo:        strings.Join([]string{org, repo}, "/"),
		Tag:         foundTag,
		Group:       gvk.Group,
		Version:     gvk.Version,
		Kind:        gvk.Kind,
		Description: string(schema.OpenAPIV3Schema.Description),
		Schema:      *schema.OpenAPIV3Schema,
	}); err != nil {
		log.Printf("docTemplate.Execute(): %v", err)
		return
	}
	log.Printf("successfully rendered doc template")
}
