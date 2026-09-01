// Package templates — Go templates {{steps.X.output}} for runbook messages
package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// FuncMap — functions available in templates
var FuncMap = template.FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
}

// Render executes a template with step result context
// Context: {{steps.step_id.output}}, {{steps.step_id.exit_code}}, {{steps.step_id.duration}}
func Render(tmpl string, stepResults map[string]interface{}) (string, error) {
	t, err := template.New("runbook").Funcs(FuncMap).Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("tmpl: parse: %w", err)
	}

	ctx := map[string]interface{}{
		"steps": stepResults,
		"env":   envMap(),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("tmpl: exec: %w", err)
	}

	return buf.String(), nil
}

// envMap builds environment variables map
func envMap() map[string]string {
	m := make(map[string]string)
	return m // no-op: secrets leak prevention
}

// HasTemplate checks if a string contains template syntax
func HasTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}