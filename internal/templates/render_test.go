package templates

import (
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name       string
		tmpl       string
		steps      map[string]interface{}
		want       string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "plain text returns as-is",
			tmpl:    "hello world",
			steps:   map[string]interface{}{},
			want:    "hello world",
			wantErr: false,
		},
		{
			name: "step output interpolation",
			tmpl: "result: {{(index .steps \"step-0\").output}}",
			steps: map[string]interface{}{
				"step-0": map[string]interface{}{
					"output": "done",
				},
			},
			want:    "result: done",
			wantErr: false,
		},
		{
			name: "exit code interpolation",
			tmpl: "exit: {{(index .steps \"step-0\").exit_code}}",
			steps: map[string]interface{}{
				"step-0": map[string]interface{}{
					"exit_code": 0,
				},
			},
			want:    "exit: 0",
			wantErr: false,
		},
		{
			name:    "invalid template syntax returns error",
			tmpl:    "hello {{.bad",
			steps:   map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, tt.steps)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Render() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
				t.Errorf("error msg = %q, want %q", err.Error(), tt.wantErrMsg)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasTemplate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "hello", want: false},
		{input: "{{.output}}", want: true},
		{input: "{{ }}", want: true},
		{input: "text {{ with }} }} end", want: true},
		{input: "{not a template}", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := HasTemplate(tt.input)
			if got != tt.want {
				t.Errorf("HasTemplate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
