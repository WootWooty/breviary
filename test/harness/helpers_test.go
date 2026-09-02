package harness_test

import (
	"bytes"
	"fmt"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

const binName = "breviary-test"

var update = flag.Bool("update", false, "update golden files with current output")

// findModuleRoot walks up from the test package location to find go.mod.
func findModuleRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found from %s", cwd)
}

// BuildBinary compiles breviary into a temp binary and returns its path.
// The binary is cleaned up when the test/tool ends.
func BuildBinary(tb testing.TB) string {
	tb.Helper()

	binPath := filepath.Join(tb.TempDir(), binName)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/breviary/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("build breviary: %v\n%s", err, out)
	}
	return binPath
}

// NormalizeOutput strips runtime-variable content (timestamps, log lines)
// from CLI output so it can be compared deterministically.
// Currently strips lines matching Go's default slog timestamp format.
func NormalizeOutput(raw []byte) []byte {
	tsLine := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)
	lines := bytes.Split(raw, []byte("\n"))
	var clean [][]byte
	for _, line := range lines {
		if len(line) == 0 {
			continue // skip empty trailing lines
		}
		if tsLine.Match(line) {
			continue // skip log lines
		}
		clean = append(clean, line)
	}
	return bytes.Join(clean, []byte("\n"))
}

// GoldenPath returns the path to a golden file inside testdata/.
func GoldenPath(name string) string {
	return filepath.Join("testdata", name+".golden")
}

// CompareWithGolden compares got against the golden file for name.
// If -update flag is set, it overwrites the golden file with got.
// Otherwise, it fails the test if they differ.
func CompareWithGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	golden := GoldenPath(name)

	if *update {
		// Ensure testdata/ exists before writing
		goldenDir := filepath.Dir(golden)
		if err := os.MkdirAll(goldenDir, 0755); err != nil {
			t.Fatalf("create golden dir %s: %v", goldenDir, err)
		}
		if err := os.WriteFile(golden, got, 0644); err != nil {
			t.Fatalf("update golden %s: %v", name, err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", name, err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("output differs from golden %s:\n--- want (golden) / +++ got (actual)\n%s",
			golden, diffLines(want, got))
	}
}

// diffLines returns a minimal unified diff between want and got byte slices.
func diffLines(want, got []byte) string {
	wantLines := bytes.Split(want, []byte("\n"))
	gotLines := bytes.Split(got, []byte("\n"))

	var buf bytes.Buffer
	max := len(wantLines)
	if len(gotLines) > max {
		max = len(gotLines)
	}

	for i := 0; i < max; i++ {
		w := []byte{}
		g := []byte{}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if !bytes.Equal(w, g) {
			if len(w) > 0 {
				buf.WriteString("-" + string(w) + "\n")
			}
			if len(g) > 0 {
				buf.WriteString("+" + string(g) + "\n")
			}
		}
	}
	return buf.String()
}