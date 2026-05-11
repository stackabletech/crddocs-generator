package config

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"gopkg.in/yaml.v2"
)

const NightlyTag = "nightly"

type Config struct {
	Repos []string `yaml:"repos"`

	// Tags and PlatformVersions are populated by ResolveTags.
	Tags             map[string][]string `yaml:"-"`
	PlatformVersions []string            `yaml:"-"`
}

func (c *Config) NewConfigFromFile(filePath string) error {
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading YAML file %s: %v", filePath, err)
		return err
	}

	if err := yaml.Unmarshal(yamlFile, c); err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
		return err
	}

	return nil
}

// ResolveTags lists remote tags for each configured repo, keeps only the
// latest patch per YY.M release line, appends `nightly`, and fills
// c.Tags and c.PlatformVersions. PlatformVersions is the union across all
// repos, newest calver first, with nightly last (matching the previous
// hand-maintained order).
func (c *Config) ResolveTags() error {
	c.Tags = map[string][]string{}
	platformSet := map[string]struct{}{}

	for _, repo := range c.Repos {
		url := "https://github.com/stackabletech/" + repo
		log.Printf("Listing tags for %s ...", repo)
		tags, err := listLatestPatches(url)
		if err != nil {
			return fmt.Errorf("listing tags for %s: %w", repo, err)
		}
		c.Tags[repo] = append(tags, NightlyTag)

		for _, t := range tags {
			platformSet[t] = struct{}{}
		}
	}

	platform := make([]string, 0, len(platformSet))
	for v := range platformSet {
		platform = append(platform, v)
	}
	sortCalverDesc(platform)
	c.PlatformVersions = append(platform, NightlyTag)

	return nil
}

type calver struct{ y, m, p int }

var calverRe = regexp.MustCompile(`^(\d{2})\.(\d{1,2})\.(\d+)$`)

func parseCalver(s string) (calver, bool) {
	m := calverRe.FindStringSubmatch(s)
	if m == nil {
		return calver{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	p, _ := strconv.Atoi(m[3])
	return calver{y, mo, p}, true
}

func calverLess(a, b calver) bool {
	if a.y != b.y {
		return a.y < b.y
	}
	if a.m != b.m {
		return a.m < b.m
	}
	return a.p < b.p
}

func sortCalverDesc(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		a, _ := parseCalver(versions[i])
		b, _ := parseCalver(versions[j])
		return calverLess(b, a)
	})
}

// listLatestPatches returns the latest-patch calver tag per YY.M release
// line for the given remote, newest first.
func listLatestPatches(url string) ([]string, error) {
	rem := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	refs, err := rem.List(&git.ListOptions{})
	if err != nil {
		return nil, err
	}

	type line struct{ y, m int }
	latest := map[line]calver{}
	names := map[line]string{}

	for _, ref := range refs {
		if !ref.Name().IsTag() {
			continue
		}
		name := strings.TrimPrefix(ref.Name().String(), "refs/tags/")
		cv, ok := parseCalver(name)
		if !ok {
			continue
		}
		key := line{cv.y, cv.m}
		if cur, exists := latest[key]; !exists || calverLess(cur, cv) {
			latest[key] = cv
			names[key] = name
		}
	}

	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, n)
	}
	sortCalverDesc(out)
	return out, nil
}
