package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"DBPath", cfg.DBPath, "breviary.db"},
		{"Serve.Addr", cfg.Serve.Addr, ":8080"},
		{"Logging.Format", cfg.Logging.Format, "text"},
		{"Logging.Level", cfg.Logging.Level, "info"},
		{"Approval.DefaultTimeout", cfg.Approval.DefaultTimeout, "30m"},
		{"Approval.EscalationAfter", cfg.Approval.EscalationAfter, "10m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got = %v, want %v", tt.got, tt.expected)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string // empty = file doesn't exist
		wantErr bool
		checkDB string // expected DBPath after load
	}{
		{
			name:    "config file with custom DB path",
			content: "db_path: custom.db\n",
			wantErr: false,
			checkDB: "custom.db",
		},
		{
			name:    "empty config file uses defaults",
			content: "{}\n",
			wantErr: false,
			checkDB: "breviary.db",
		},
		{
			name:    "invalid YAML returns error",
			content: "db_path: [invalid\n",
			wantErr: true,
			checkDB: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && tt.checkDB != "" && cfg.DBPath != tt.checkDB {
				t.Errorf("DBPath = %q, want %q", cfg.DBPath, tt.checkDB)
			}
		})
	}
}

func TestLoadBest(t *testing.T) {
	t.Run("returns defaults when no config exists", func(t *testing.T) {
		cfg, path, err := LoadBest()
		if err != nil {
			t.Fatalf("LoadBest() error = %v", err)
		}
		if cfg == nil {
			t.Fatal("LoadBest() returned nil config")
		}
		if path != "" {
			t.Logf("loaded from: %s", path)
		}
		if cfg.DBPath != "breviary.db" {
			t.Errorf("expected default DBPath, got %q", cfg.DBPath)
		}
	})
}
