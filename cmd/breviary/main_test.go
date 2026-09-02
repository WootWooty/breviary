package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// path to built binary (set by TestMain)
var binPath string

// ---- Unit tests (direct function calls) ----

func TestCmdValidate_Valid(t *testing.T) {
	// Use actual example file from the project root
	root := findProjectRoot(t)
	path := filepath.Join(root, "examples/hello-world.yaml")

	err := cmdValidate([]string{path})
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestCmdValidate_Invalid(t *testing.T) {
	err := cmdValidate([]string{"/nonexistent/runbook.yaml"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestCmdValidate_NoArgs(t *testing.T) {
	err := cmdValidate(nil)
	if err == nil {
		t.Fatal("expected error for missing file argument")
	}
}

func TestCmdRun_Valid(t *testing.T) {
	// Run in a temp dir so DB side-effects don't pollute
	root := findProjectRoot(t)
	path := filepath.Join(root, "examples/hello-world.yaml")

	// Chdir to temp dir for DB isolation
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	err := cmdRun([]string{path})
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestCmdRun_NoArgs(t *testing.T) {
	err := cmdRun(nil)
	if err == nil {
		t.Fatal("expected error for missing file argument")
	}
}

func TestCmdRun_Nonexistent(t *testing.T) {
	err := cmdRun([]string{"/nonexistent/runbook.yaml"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestGetEngine(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	eng := getEngine("")
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
	_ = eng.Close()
}

// ---- Binary-based acceptance tests (verify CLI entry point) ----

func TestBinaryValidate(t *testing.T) {
	moduleRoot := filepath.Dir(binPath)
	examplePath := filepath.Join(moduleRoot, "examples/hello-world.yaml")
	out, err := exec.Command(binPath, "validate", examplePath).CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "valid") {
		t.Errorf("expected validation message, got %s", out)
	}
}

func TestBinaryRun(t *testing.T) {
	moduleRoot := filepath.Dir(binPath)
	dir := t.TempDir()
	dstBin := filepath.Join(dir, "breviary")
	copyFile(dstBin, binPath)
	copyFile(filepath.Join(dir, "hello.yaml"), filepath.Join(moduleRoot, "examples/hello-world.yaml"))

	cmd := exec.Command(dstBin, "run", "hello.yaml")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Done") {
		t.Errorf("expected completion message, got %s", out)
	}
}

func TestBinaryValidate_Nonexistent(t *testing.T) {
	cmd := exec.Command(binPath, "validate", "/nonexistent.yaml")
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for nonexistent file")
	}
}

func TestBinaryHelp(t *testing.T) {
	out, err := exec.Command(binPath, "--help").CombinedOutput()
	// --help is not a registered subcommand, so it calls usage() and exits 1
	if err == nil {
		t.Error("expected non-zero exit for --help flag")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage output, got: %s", out)
	}
}

func TestBinaryNoArgs(t *testing.T) {
	out, err := exec.Command(binPath).CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit when no args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage output, got: %s", out)
	}
}

// ---- helpers ----

func findProjectRoot(t *testing.T) string {
	t.Helper()
	// Walk up from cwd or use binary path as hint
	candidates := []string{
		os.Getenv("BREVIARY_ROOT"),
		".",
		"..",
		"../..",
		filepath.Dir(binPath),
	}
	for _, d := range candidates {
		if d == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs
		}
	}

	// Walk up from cwd
	cwd, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		cwd = filepath.Dir(cwd)
	}

	t.Fatal("cannot find project root (go.mod)")
	return ""
}

func copyFile(dst, src string) {
	data, err := os.ReadFile(src)
	if err != nil {
		return // caller handles
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return
	}
	if err := os.Chmod(dst, 0755); err != nil {
		return
	}
}

func TestMain(m *testing.M) {
	// Build binary in module root (2 levels up from cmd/breviary/)
	pkgDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	root := filepath.Dir(filepath.Dir(pkgDir))
	binPath = filepath.Join(root, "breviary")

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		build := exec.Command("go", "build", "-o", binPath, "./cmd/breviary/")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
