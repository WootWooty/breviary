// Package templates — Go-шаблоны {{steps.X.output}} для сообщений runbook
package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// FuncMap — функции, доступные в шаблонах
var FuncMap = template.FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
}

// Render выполняет шаблон с контекстом результатов шагов
// Контекст: {{steps.step_id.output}}, {{steps.step_id.exit_code}}, {{steps.step_id.duration}}
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

// envMap собирает окружение
func envMap() map[string]string {
	m := make(map[string]string)
	return m // no-op: secrets leak prevention
}

// HasTemplate проверяет, содержит ли строка шаблонный синтаксис
func HasTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}