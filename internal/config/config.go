// Package config — загрузка и типы конфигурации Breviary
// Файл: ~/.config/breviary/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config — корневая конфигурация
type Config struct {
	DBPath   string           `yaml:"db_path"`
	Serve    ServeConfig      `yaml:"serve"`
	Logging  LoggingConfig    `yaml:"logging"`
	Approval ApprovalConfig   `yaml:"approval"`
	Runbooks []RunbookRef     `yaml:"runbooks"`
}

// ServeConfig — HTTP сервер
type ServeConfig struct {
	Addr string `yaml:"addr"` // :8080
}

// LoggingConfig — логи
type LoggingConfig struct {
	Format string `yaml:"format"` // text или json
	Level  string `yaml:"level"`  // debug, info, warn, error
}

// ApprovalConfig — глобальные настройки approval
type ApprovalConfig struct {
	DefaultTimeout string `yaml:"default_timeout"` // 30m
	EscalationAfter string `yaml:"escalation_after"` // 10m
}

// RunbookRef — ссылка на runbook для загрузки
type RunbookRef struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// Default возвращает конфигурацию по умолчанию
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
			DefaultTimeout:   "30m",
			EscalationAfter: "10m",
		},
	}
}

// Load загружает конфиг из файла (с дефолтами для пропущенных полей)
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // нет файла — дефолты
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return cfg, nil
}

// PathsToCheck возвращает возможные пути к конфигу
func PathsToCheck() []string {
	return []string{
		filepath.Join(os.Getenv("HOME"), ".config", "breviary", "config.yaml"),
		"breviary.yaml",
		"breviary.yml",
	}
}

// LoadBest загружает первый найденный конфиг
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