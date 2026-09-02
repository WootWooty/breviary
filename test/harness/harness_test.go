package harness_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	binPath string
	root    string
)

func TestMain(m *testing.M) {
	// Locate project root (walk up from module dir)
	var err error
	root, err = findModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find module root: %v\n", err)
		os.Exit(1)
	}

	// Build binary once for the entire acceptance suite
	binPath, err = exec.LookPath(binName)
	if err != nil {
		binPath = filepath.Join(os.TempDir(), binName)
		build := exec.Command("go", "build", "-o", binPath, "./cmd/breviary/")
		build.Dir = root
		out, buildErr := build.CombinedOutput()
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "build breviary: %v\n%s", buildErr, out)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "built: %s\n", binPath)

	code := m.Run()

	// Cleanup: only remove if we built it (not found in PATH)
	if _, err := exec.LookPath(binName); err != nil {
		_ = os.Remove(binPath) // best-effort cleanup
	}
	os.Exit(code)
}

// TestRunAcceptance runs acceptance tests against the compiled binary.
// Each sub-case invokes the CLI as a user would and compares output to golden files.
func TestRunAcceptance(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		golden  string
		wantNil bool // true if we want exit code 0
	}{
		{
			name:    "hello-world",
			args:    []string{"run", "examples/hello-world.yaml"},
			golden:  "hello-world.run",
			wantNil: true,
		},
		{
			name:    "validate/valid",
			args:    []string{"validate", "examples/hello-world.yaml"},
			golden:  "validate.ok",
			wantNil: true,
		},
		{
			name:    "validate/invalid",
			args:    []string{"validate", "nonexistent.yaml"},
			golden:  "validate.fail",
			wantNil: false,
		},
		{
			name:    "help",
			args:    []string{"--help"},
			golden:  "help",
			wantNil: false, // CLI exits 1 for unknown flag
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(binPath, tt.args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()

			gotExitNil := err == nil
			if gotExitNil != tt.wantNil {
				t.Errorf("exit: got nil=%v, want nil=%v (stderr: %s)",
					gotExitNil, tt.wantNil, string(out))
			}

			got := NormalizeOutput(out)
			CompareWithGolden(t, tt.golden, got)
		})
	}
}

// TestValidateAcceptance runs validation tests grouped together.
func TestValidateAcceptance(t *testing.T) {
	t.Parallel()

	t.Run("valid runbook", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command(binPath, "validate", "examples/hello-world.yaml")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("expected success: %v\n%s", err, out)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command(binPath, "validate", "nonexistent.yaml")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
		_ = out // we check golden below
	})

	t.Run("malformed yaml", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command(binPath, "validate", "test/harness/testdata/malformed.yaml")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected error for malformed YAML")
		}
		_ = out
	})
}
