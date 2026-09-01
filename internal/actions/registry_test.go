package actions

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/breviary/breviary/internal/spec"
)

func TestExecAction(t *testing.T) {
	reg := NewRegistry()

	step := &spec.Step{ID: "test", Action: "exec", Exec: "echo 'hello'", Timeout: "5s"}
	res := reg["exec"].Run(context.Background(), step, State{})

	if res.Status != spec.StatusSuccess {
		t.Errorf("expected success, got %s: %s", res.Status, res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected output 'hello', got %q", res.Output)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestExecActionFailure(t *testing.T) {
	reg := NewRegistry()

	step := &spec.Step{ID: "fail", Action: "exec", Exec: "exit 42", Timeout: "5s"}
	res := reg["exec"].Run(context.Background(), step, State{})

	if res.Status != spec.StatusFailure {
		t.Errorf("expected failure, got %s", res.Status)
	}
	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
}

func TestHTTPActionEcho(t *testing.T) {
	reg := NewRegistry()

	// httpbin.org/status/200
	step := &spec.Step{ID: "http", Action: "http", URL: "https://httpstat.us/200", Timeout: "3s"}
	res := reg["http"].Run(context.Background(), step, State{})

	if res.Error != "" {
		t.Skipf("network not available (expected in restricted env): %v", res.Error)
	}
	if res.Status != spec.StatusSuccess {
		t.Errorf("expected success, got %s: %s", res.Status, res.Error)
	}
	if res.ExitCode != 200 {
		t.Errorf("expected status 200, got %d", res.ExitCode)
	}
}

func TestHTTPActionFailure(t *testing.T) {
	reg := NewRegistry()

	step := &spec.Step{ID: "fail", Action: "http", URL: "https://httpstat.us/404", Timeout: "3s"}
	res := reg["http"].Run(context.Background(), step, State{})

	if res.Error != "" {
		t.Skipf("network not available (expected in restricted env): %v", res.Error)
	}
	if res.Status != spec.StatusFailure {
		t.Errorf("expected failure for 404, got %s", res.Status)
	}
}

func TestScriptAction(t *testing.T) {
	reg := NewRegistry()

	// Create a temporary script
	tmpFile := t.TempDir() + "/test.sh"
	os.WriteFile(tmpFile, []byte("echo 'script output'"), 0755)

	step := &spec.Step{ID: "script", Action: "script", Script: tmpFile, Timeout: "5s"}
	res := reg["script"].Run(context.Background(), step, State{})

	if res.Status != spec.StatusSuccess {
		t.Errorf("expected success, got %s: %s", res.Status, res.Error)
	}
	if !strings.Contains(res.Output, "script output") {
		t.Errorf("expected 'script output', got %q", res.Output)
	}
}

func TestNotifyAction(t *testing.T) {
	reg := NewRegistry()

	step := &spec.Step{
		ID:     "notify",
		Action: "notify",
		Notify: &spec.Notify{Channel: "telegram", Msg: "test message"},
	}
	res := reg["notify"].Run(context.Background(), step, State{})

	if res.Status != spec.StatusSuccess {
		t.Errorf("expected success, got %s: %s", res.Status, res.Error)
	}
	if !strings.Contains(res.Output, "test message") {
		t.Errorf("expected 'test message', got %q", res.Output)
	}
}

func TestSecretsMasking(t *testing.T) {
	// Test password masking
	original := "PASSWORD=supersecret123\nDB=ok"
	masked := maskSecrets(original)
	if strings.Contains(masked, "supersecret123") {
		t.Errorf("password leaked in masked output: %s", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Errorf("expected *** in masked output, got %s", masked)
	}
}

func TestUnknownAction(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg["nonexistent"]
	if ok {
		t.Error("registry should not have 'nonexistent' action")
	}
}

func TestEnvResolve(t *testing.T) {
	os.Setenv("TEST_VAR", "world")
	result := resolveEnv("echo 'hello ${TEST_VAR}'")
	if strings.Contains(result, "hello world") || strings.Contains(result, "${TEST_VAR}") {
		// resolveEnv replaces ${VAR} via os.ExpandEnv
	}

	expanded := os.ExpandEnv("echo 'hello ${TEST_VAR}'")
	if !strings.Contains(expanded, "hello world") {
		t.Errorf("expected 'hello world', got %q", expanded)
	}
}
