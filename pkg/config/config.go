package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v2"
)

type SyncMode int

const (
	Downld SyncMode = iota
	DownldDel
	Validte
)

type Config struct {
	App struct {
		SyncMode SyncMode
	}
	Storage struct {
		Root string `yaml:"storage_root"`
	} `yaml:"storage_config"`
	ODrive struct {
		ClientID string `yaml:"client_id"`
		TenantID string `yaml:"tenant_id"`
	} `yaml:"onedrive_config"`
	DirMap []string `yaml:"directory_mappings"`
}

func LoadConfig() (*Config, error) {
	conf, err := parseConfigFile()
	if err != nil {
		return nil, err
	}

	return processConfig(conf)
}

func parseConfigFile() (*Config, error) {
	// read the file config/{app_env}.yaml and parse it with Config struct
	data, err := os.ReadFile("config/e0.yaml")
	if err != nil {
		return nil, err
	}

	var cnf Config
	err = yaml.Unmarshal(data, &cnf)

	return &cnf, err
}

func processConfig(c *Config) (*Config, error) {
	root := os.Getenv("SNOWHIVE")
	if root == "" {
		return nil, errors.New("SNOWHIVE environment variable is not set")
	}
	c.Storage.Root = root
	return c, nil
}
