// Package git — runbook synchronization from Git repositories
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Sync clones or pulls a runbook repository.
// Returns the path to the directory containing runbooks.
func Sync(cacheDir, repoURL, ref string) (string, error) {
	if ref == "" {
		ref = "main"
	}

	// Directory name derived from URL (e.g. "my-runbooks")
	dirName := repoName(repoURL)
	targetDir := filepath.Join(cacheDir, dirName)

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); err == nil {
		// Pull
		cmd := exec.Command("git", "pull", "--ff-only", "origin", ref)
		cmd.Dir = targetDir
		cmd.Stderr = os.Stderr
		if out, err := cmd.Output(); err != nil {
			return "", fmt.Errorf("git pull %s: %w\n%s", repoURL, err, string(out))
		}
		return targetDir, nil
	}

	// Clone
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("git: mkdir %s: %w", cacheDir, err)
	}
	cmd := exec.Command("git", "clone", "--depth=1", "-b", ref, repoURL, targetDir)
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		return "", fmt.Errorf("git clone %s: %w\n%s", repoURL, err, string(out))
	}

	return targetDir, nil
}

// ListRunbooks returns a list of .yaml files in the directory
func ListRunbooks(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			ext := filepath.Ext(e.Name())
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	return files, nil
}

func repoName(url string) string {
	base := filepath.Base(url)
	ext := filepath.Ext(base)
	if ext == ".git" {
		base = base[:len(base)-len(ext)]
	}
	return base
}