// Package actions — action executor registry (exec/http/script/notify)
// design: Go interface, registry map. v1 — built-in, v2 — build-time plugins (xcaddy pattern).
package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/breviary/breviary/internal/spec"
)

// Action — step executor interface
type Action interface {
	Run(ctx context.Context, step *spec.Step, state State) spec.Result
}

// State — execution context for an Action (previous step results, metadata)
type State struct {
	StepResults map[string]interface{} // {stepID: {output, exit_code, ...}}
	RunName     string
	RunID       string
}

// Registry — available action types
type Registry map[string]Action

// NewRegistry creates a registry with built-in actions
func NewRegistry() Registry {
	return Registry{
		"exec":   ExecAction{},
		"http":   HTTPAction{},
		"script": ScriptAction{},
		"notify": NotifyAction{},
	}
}

// ExecAction — executes a shell command
type ExecAction struct{}

func (ExecAction) Run(ctx context.Context, step *spec.Step, state State) spec.Result {
	res := spec.Result{StepID: step.ID, Status: spec.StatusSuccess}

	// Secrets: replace ${VAR} from environment, mask in log output
	cmdStr := resolveEnv(step.Exec)

	var cmd *exec.Cmd
	if step.RunAs != "" && step.RunAs != os.Getenv("USER") {
		cmd = exec.CommandContext(ctx, "sudo", "-u", step.RunAs, "sh", "-c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	// Timeout
	if step.Timeout != "" {
		d, err := time.ParseDuration(step.Timeout)
		if err == nil && d > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
			cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
		}
	}

	out, err := cmd.CombinedOutput()
	res.Duration = time.Now().Format(time.RFC3339)
	res.Output = string(out)

	if err != nil {
		res.Error = maskSecrets(err.Error())
		res.Status = spec.StatusFailure
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
		}
	} else {
		res.ExitCode = 0
	}

	// Mask secrets in output
	res.Output = maskSecrets(res.Output)
	return res
}

// HTTPAction — executes an HTTP request (for healthchecks/API)
type HTTPAction struct {
	Client *http.Client // optional: custom client (for testing)
}

func (a HTTPAction) Run(ctx context.Context, step *spec.Step, state State) spec.Result {
	res := spec.Result{StepID: step.ID, Status: spec.StatusSuccess}

	if step.URL == "" {
		res.Error = "http action requires 'url' field"
		res.Status = spec.StatusFailure
		return res
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolveEnv(step.URL), nil)
	if err != nil {
		res.Error = fmt.Sprintf("http request: %v", err)
		res.Status = spec.StatusFailure
		return res
	}

	client := a.Client
	if client == nil {
		timeout := 10 * time.Second
		if step.Timeout != "" {
			if d, err := time.ParseDuration(step.Timeout); err == nil && d > 0 {
				timeout = d
			}
		}
		client = &http.Client{Timeout: timeout}
	}
	start := time.Now()

	resp, err := client.Do(req)
	res.Duration = time.Since(start).Truncate(time.Millisecond).String()
	if err != nil {
		res.Error = fmt.Sprintf("http: %v", err)
		res.Status = spec.StatusFailure
		res.ExitCode = 1
		return res
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	res.Output = string(body)
	res.ExitCode = resp.StatusCode
	if resp.StatusCode >= 400 {
		res.Error = fmt.Sprintf("http %d", resp.StatusCode)
		res.Status = spec.StatusFailure
	}
	return res
}

// ScriptAction — executes a script from a file
type ScriptAction struct{}

func (ScriptAction) Run(ctx context.Context, step *spec.Step, state State) spec.Result {
	res := spec.Result{StepID: step.ID, Status: spec.StatusSuccess}

	if step.Script == "" {
		res.Error = "script action requires 'script' field"
		res.Status = spec.StatusFailure
		return res
	}

	content, err := os.ReadFile(step.Script)
	if err != nil {
		res.Error = fmt.Sprintf("read script: %v", err)
		res.Status = spec.StatusFailure
		return res
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", string(content))
	// Set environment for the script
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	res.Output = string(out)
	res.Duration = time.Now().Format(time.RFC3339)

	if err != nil {
		res.Error = err.Error()
		res.Status = spec.StatusFailure
		res.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		}
	} else {
		res.ExitCode = 0
	}

	res.Output = maskSecrets(res.Output)
	return res
}

// NotifyAction — sends a notification (MVP: stdout stub only)
type NotifyAction struct{}

func (NotifyAction) Run(ctx context.Context, step *spec.Step, state State) spec.Result {
	res := spec.Result{StepID: step.ID, Status: spec.StatusSuccess}

	if step.Notify == nil {
		res.Error = "notify action requires 'notify' config"
		res.Status = spec.StatusFailure
		return res
	}

	// MVP: just print to stdout (Telegram integration — P4)
	msg := resolveEnv(step.Notify.Msg)
	res.Output = fmt.Sprintf("[NOTIFY %s] %s", step.Notify.Channel, msg)

	return res
}

// resolveEnv replaces ${VAR} from environment
func resolveEnv(s string) string {
	return os.ExpandEnv(s)
}

// maskSecrets masks suspicious strings (passwords, tokens, keys)
func maskSecrets(s string) string {
	// Simple heuristic: values after = in context of PASSWORD/TOKEN/SECRET/KEY
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "password") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "apikey") ||
			strings.Contains(lower, "private_key") ||
			strings.Contains(lower, "authorization") {

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && len(parts[1]) > 3 {
				lines[i] = parts[0] + "=***"
			}
		}
	}
	return strings.Join(lines, "\n")
}
