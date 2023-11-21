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
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	crdutil "github.com/crdsdev/doc/pkg/crd"
	"github.com/crdsdev/doc/pkg/models"
	// "github.com/google/uuid"
	flag "github.com/spf13/pflag"
	"github.com/unrolled/render"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	// v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
	_ "github.com/mattn/go-sqlite3"
)

// redis connection
var (
	envAnalytics   = "ANALYTICS"
	envDevelopment = "IS_DEV"

	address   string
	analytics bool = false
)

// SchemaPlusParent is a JSON schema plus the name of the parent field.
type SchemaPlusParent struct {
	Parent string
	Schema map[string]apiextensions.JSONSchemaProps
}

var page = render.New(render.Options{
	Extensions:    []string{".html"},
	Directory:     "template",
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

type pageData struct {
	Analytics     bool
	DisableNavBar bool
	IsDarkMode    bool
	Title         string
}

type baseData struct {
	Page pageData
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
	Page  pageData
	Repo  string
	Tag   string
	At    string
	Tags  []string
	CRDs  map[string]models.RepoCRD
	Total int
}

type homeData struct {
	Page  pageData
	Repos []string
}

func main() {
	flag.Parse()
	db, err := sql.Open("sqlite3", "doc.db")
	if err != nil {
		panic(err)
	}

	var outDir = "out"
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		log.Println("Error creating output directory:", err)
		return
	}

	home(outDir)

	yamlFile, err := ioutil.ReadFile("repos.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}

	var config map[string]map[string][]string

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
	}

	for orgname, repos := range config {
		for repo, tags := range repos {
			org(db, outDir, orgname, repo, "")
			for _, tag := range tags {
				org(db, outDir, orgname, repo, tag)
			}
		}
	}

	//r.PathPrefix("/").HandlerFunc(doc)
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

func home(outDir string) {
	// Open the file for writing
	file, err := os.Create(fmt.Sprintf("%s/%s", outDir, "index.html"))
	if err != nil {
		log.Printf("Error creating index.html: %v", err)
		return
	}
	defer file.Close()
	data := homeData{Page: getPageData("Doc", true)}
	// TODO .. this seems to use go templates, and also "unroller". Maybe I don't need unroller?

	if err := page.HTML(file, http.StatusOK, "home", data); err != nil {
		log.Printf("homeTemplate.Execute(): %v", err)
		return
	}
	log.Print("successfully rendered home page")
}

func org(db *sql.DB, outDir string, org string, repo string, tag string) {
	fullDir := fmt.Sprintf("%s/%s/%s", outDir, org, repo)
	if tag != "" {
		fullDir = fmt.Sprintf("%s/%s/%s/%s", outDir, org, repo, tag)
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
	log.Printf("fullRepo: '%s'", fullRepo)
	log.Printf("'%s'", tag)
	var c *sql.Rows
	if tag == "" {
		c, err = db.Query("SELECT t.name, c.'group', c.version, c.kind FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.id = (SELECT id FROM tags WHERE LOWER(repo) = LOWER($1) ORDER BY time ASC LIMIT 1);", fullRepo)
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
		doc(db, outDir, org, repo, tag, g, k, v)
	}
	if c.Err() != nil {
		log.Printf("Error in Next: %s", err)
		panic(err)
	}
	c, err = db.Query("SELECT name FROM tags WHERE LOWER(repo)=LOWER($1) ORDER BY time DESC;", fullRepo)
	if err != nil {
		log.Printf("failed to get tags for %s : %v", repo, err)
		panic(err)  // something went wrong, there should be tags
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
	if err := page.HTML(file, http.StatusOK, "org", orgData{
		Page:  pageData,
		Repo:  strings.Join([]string{org, repo}, "/"),
		Tag:   foundTag,
		Tags:  tags,
		CRDs:  repoCRDs,
		Total: len(repoCRDs),
	}); err != nil {
		log.Printf("orgTemplate.Execute(): %v", err)
		return
	}
	log.Printf("successfully rendered org template")
}

func doc(db *sql.DB, outDir string, org string, repo string, tag string, group string, kind string, version string) {
	fullDir := fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", outDir, org, repo, tag, group, kind, version)
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
			if version.Storage == true {
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
