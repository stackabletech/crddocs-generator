package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Repos            map[string][]string `yaml:"repos"`
	PlatformVersions []string            `yaml:"platformVersions"`
}

func (c *Config) NewConfigFromFile(filePath string) error {
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading YAML file %s: %v", filePath, err)
		return err
	}

	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
		return err
	}

	return nil
}
