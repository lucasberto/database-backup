package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Name           string   `yaml:"name"`
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	User           string   `yaml:"user"`
	AuthType       string   `yaml:"auth_type"`
	KeyPath        string   `yaml:"key_path"`
	Passphrase     string   `yaml:"passphrase"`
	OutputPath     string   `yaml:"output_path"`
	CredentialsKey string   `yaml:"credentials_key"`
	Database       Database `yaml:"database"`
}

type Database struct {
	Type           string `yaml:"type"`
	Port           int    `yaml:"port"`
	Name           string `yaml:"name"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	CredentialsKey string `yaml:"credentials_key"`
	BackupAll      bool   `yaml:"backup_all"`
}

type Config struct {
	PrivateKeyPath         string   `yaml:"private_key_path"`
	Servers                []Server `yaml:"servers"`
	MaxConcurrentServers   int      `yaml:"max_concurrent_servers"`
	MaxConcurrentDatabases int      `yaml:"max_concurrent_databases"`
}

func LoadConfig(filename string) (*Config, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(buf, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func SanitizeDirectoryName(name string) string {
	// Replace spaces and special characters with underscores
	invalidChars := []string{" ", "/", "\\", ":", "*", "?", "\"", "<", ">", "|", "&"}
	result := name
	for _, char := range invalidChars {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}
