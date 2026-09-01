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

func copyFile(dst, src string) {
	data, _ := os.ReadFile(src)
	os.WriteFile(dst, data, 0644)
	os.Chmod(dst, 0755)
}

func TestMain(m *testing.M) {
	// Build binary in module root (2 levels up from cmd/breviary/)
	pkgDir, _ := os.Getwd()
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
