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
	"errors"
	"fmt"
	"io/ioutil"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"database/sql"

	// crdutil "github.com/crdsdev/doc/pkg/crd"
	"github.com/crdsdev/doc/pkg/models"
	// "github.com/google/uuid"
	// "github.com/gorilla/mux"
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

	userEnv     = "PG_USER"
	passwordEnv = "PG_PASS"
	hostEnv     = "PG_HOST"
	portEnv     = "PG_PORT"
	dbEnv       = "PG_DB"

	cookieDarkMode = "halfmoon_preferredMode"

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

	//log.Println("Starting Doc server...")
	//r := mux.NewRouter().StrictSlash(true)
	// var outDir = "out"
	// TODO copy over static files
	// staticHandler := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))

	var gitterRepos = readConfig()

	home("index.html")

	for _, repo := range gitterRepos {
		org(db, repo.Org, repo.Repo, repo.Tag)
	}

	//r.PathPrefix("/static/").Handler(staticHandler)
	//r.HandleFunc("/github.com/{org}/{repo}@{tag}", org)
	//r.HandleFunc("/github.com/{org}/{repo}", org)
	//r.PathPrefix("/").HandlerFunc(doc)
	//log.Fatal(http.ListenAndServe(":5000", r))
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

func readConfig() []models.GitterRepo {
	yamlFile, err := ioutil.ReadFile("repos.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}

	var config map[string]map[string][]string

	// Unmarshal YAML into GitterRepoConfig struct
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
	}

	var gitterRepos []models.GitterRepo

	// Extract information from GitterRepoConfig and create GitterRepo instances
	for org, repos := range config {
		for repo, tags := range repos {
			for _, tag := range tags {
				gitterRepo := models.GitterRepo{
					Org:  org,
					Repo: repo,
					Tag:  tag,
				}
				log.Printf("Found repo in config: %+v\n", gitterRepo)
				gitterRepos = append(gitterRepos, gitterRepo)
			}
		}
	}
	return gitterRepos
}

func home(outFile string) {
	// Open the file for writing
	file, err := os.Create(outFile)
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

func org(db *sql.DB, org string, repo string, tag string) {
	pageData := getPageData(fmt.Sprintf("%s/%s", org, repo), false)
	fullRepo := fmt.Sprintf("%s/%s/%s", "github.com", org, repo)
	var c *sql.Rows
	var err error
	if tag == "" {
		c, err = db.Query("SELECT t.name, c.group, c.version, c.kind FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.id = (SELECT id FROM tags WHERE LOWER(repo) = LOWER($1) ORDER BY time ASC LIMIT 1);", fullRepo)
	} else {
		pageData.Title += fmt.Sprintf("@%s", tag)
		c, err = db.Query("SELECT t.name, c.group, c.version, c.kind FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.name=$2;", fullRepo, tag)
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
	file, err := os.Create("org.html")
	if err != nil {
		log.Printf("Error creating index.html: %v", err)
		return
	}
	defer file.Close()
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

// func doc(w http.ResponseWriter, r *http.Request) {
// 	var schema *apiextensions.CustomResourceValidation
// 	crd := &apiextensions.CustomResourceDefinition{}
// 	log.Printf("Request Received: %s\n", r.URL.Path)
// 	org, repo, group, kind, version, tag, err := parseGHURL(r.URL.Path)
// 	if err != nil {
// 		log.Printf("failed to parse Github path: %v", err)
// 		fmt.Fprint(w, "Invalid URL.")
// 		return
// 	}
// 	pageData := getPageData(fmt.Sprintf("%s.%s/%s", kind, group, version), false)
// 	fullRepo := fmt.Sprintf("%s/%s/%s", "github.com", org, repo)
// 	var c pgx.Row
// 	if tag == "" {
// 		c = db.QueryRow("SELECT t.name, c.data::jsonb FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.id = (SELECT id FROM tags WHERE repo = $1 ORDER BY time DESC LIMIT 1) AND c.group=$2 AND c.version=$3 AND c.kind=$4;", fullRepo, group, version, kind)
// 	} else {
// 		c = db.QueryRow("SELECT t.name, c.data::jsonb FROM tags t INNER JOIN crds c ON (c.tag_id = t.id) WHERE LOWER(t.repo)=LOWER($1) AND t.name=$2 AND c.group=$3 AND c.version=$4 AND c.kind=$5;", fullRepo, tag, group, version, kind)
// 	}
// 	foundTag := tag
// 	if err := c.Scan(&foundTag, crd); err != nil {
// 		log.Printf("failed to get CRDs for %s : %v", repo, err)
// 		if err := page.HTML(w, http.StatusOK, "doc", baseData{Page: pageData}); err != nil {
// 			log.Printf("newTemplate.Execute(): %v", err)
// 			fmt.Fprint(w, "Unable to render new template.")
// 		}
// 	}
// 	schema = crd.Spec.Validation
// 	if len(crd.Spec.Versions) > 1 {
// 		for _, version := range crd.Spec.Versions {
// 			if version.Storage == true {
// 				if version.Schema != nil {
// 					schema = version.Schema
// 				}
// 				break
// 			}
// 		}
// 	}

// 	if schema == nil || schema.OpenAPIV3Schema == nil {
// 		log.Print("CRD schema is nil.")
// 		fmt.Fprint(w, "Supplied CRD has no schema.")
// 		return
// 	}

// 	gvk := crdutil.GetStoredGVK(crd)
// 	if gvk == nil {
// 		log.Print("CRD GVK is nil.")
// 		fmt.Fprint(w, "Supplied CRD has no GVK.")
// 		return
// 	}

// 	if err := page.HTML(w, http.StatusOK, "doc", docData{
// 		Page:        pageData,
// 		Repo:        strings.Join([]string{org, repo}, "/"),
// 		Tag:         foundTag,
// 		Group:       gvk.Group,
// 		Version:     gvk.Version,
// 		Kind:        gvk.Kind,
// 		Description: string(schema.OpenAPIV3Schema.Description),
// 		Schema:      *schema.OpenAPIV3Schema,
// 	}); err != nil {
// 		log.Printf("docTemplate.Execute(): %v", err)
// 		fmt.Fprint(w, "Supplied CRD has no schema.")
// 		return
// 	}
// 	log.Printf("successfully rendered doc template")
// }

// TODO(hasheddan): add testing and more reliable parse
func parseGHURL(uPath string) (org, repo, group, version, kind, tag string, err error) {
	u, err := url.Parse(uPath)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	elements := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(elements) < 6 {
		return "", "", "", "", "", "", errors.New("invalid path")
	}

	tagSplit := strings.Split(u.Path, "@")
	if len(tagSplit) > 1 {
		tag = tagSplit[1]
	}

	return elements[1], elements[2], elements[3], elements[4], strings.Split(elements[5], "@")[0], tag, nil
}
