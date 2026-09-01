package engine

import (
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "explicit journal path",
			path:    filepath.Join(t.TempDir(), "test.db"),
			wantErr: false,
		},
		{
			name:    "empty journal path uses default",
			path:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := New(WithJournalPath(tt.path))
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if e == nil {
					t.Fatal("New() returned nil engine")
				}
				_ = e.Close()
			}
		})
	}
}

func TestNewWithLogger(t *testing.T) {
	t.Run("nil logger defaults to slog.Default", func(t *testing.T) {
		e, err := New(WithJournalPath(filepath.Join(t.TempDir(), "test.db")))
		if err != nil {
			t.Fatalf("New() with nil logger: %v", err)
		}
		if e == nil {
			t.Fatal("New() returned nil engine")
		}
		_ = e.Close()
	})
}
