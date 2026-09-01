// Package config — configuration loading and types for Breviary
// File: ~/.config/breviary/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config — root configuration
type Config struct {
	DBPath   string         `yaml:"db_path"`
	Serve    ServeConfig    `yaml:"serve"`
	Logging  LoggingConfig  `yaml:"logging"`
	Approval ApprovalConfig `yaml:"approval"`
	Runbooks []RunbookRef   `yaml:"runbooks"`
}

// ServeConfig — HTTP server
type ServeConfig struct {
	Addr string `yaml:"addr"` // :8080
}

// LoggingConfig — logging settings
type LoggingConfig struct {
	Format string `yaml:"format"` // text or json
	Level  string `yaml:"level"`  // debug, info, warn, error
}

// ApprovalConfig — global approval settings
type ApprovalConfig struct {
	DefaultTimeout  string `yaml:"default_timeout"`  // 30m
	EscalationAfter string `yaml:"escalation_after"` // 10m
}

// RunbookRef — a reference to a runbook for loading
type RunbookRef struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		DBPath: "breviary.db",
		Serve: ServeConfig{
			Addr: ":8080",
		},
		Logging: LoggingConfig{
			Format: "text",
			Level:  "info",
		},
		Approval: ApprovalConfig{
			DefaultTimeout:  "30m",
			EscalationAfter: "10m",
		},
	}
}

// Load loads configuration from a file (with defaults for missing fields)
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // no file — defaults
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return cfg, nil
}

// PathsToCheck returns possible config file paths
func PathsToCheck() []string {
	return []string{
		filepath.Join(os.Getenv("HOME"), ".config", "breviary", "config.yaml"),
		"breviary.yaml",
		"breviary.yml",
	}
}

// LoadBest loads the first found config file
func LoadBest() (*Config, string, error) {
	for _, p := range PathsToCheck() {
		cfg, err := Load(p)
		if err == nil {
			return cfg, p, nil
		}
		if !os.IsNotExist(err) {
			return nil, "", err
		}
	}
	return Default(), "", nil
}
