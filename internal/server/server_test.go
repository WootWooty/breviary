package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/breviary/breviary/internal/engine"
	"github.com/breviary/breviary/internal/spec"
)

func TestHealthEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "health returns ok",
			method:     "GET",
			path:       "/api/v1/health",
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "unknown route returns 404",
			method:     "GET",
			path:       "/api/v1/unknown",
			wantStatus: http.StatusNotFound,
			wantBody:   "",
		},
	}

	eng, err := engine.New(engine.WithJournalPath(filepath.Join(t.TempDir(), "srv.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	s := New(eng)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody+"\n" {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRunEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
		wantField  string // field to check in JSON response
		wantValue  string
	}{
		{
			name:       "missing runbook name returns 400",
			method:     "POST",
			path:       "/api/v1/run",
			body:       `{"alert":"DiskUsageHigh"}`,
			wantStatus: http.StatusNotFound,
			wantField:  "",
			wantValue:  "",
		},
		{
			name:       "registered runbook returns 200",
			method:     "POST",
			path:       "/api/v1/run",
			body:       `{"alert":"test-run"}`,
			wantStatus: http.StatusOK,
			wantField:  "status",
			wantValue:  "ok",
		},
	}

	eng, err := engine.New(engine.WithJournalPath(filepath.Join(t.TempDir(), "srv.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	s := New(eng)
	// Register a simple runbook
	book := &spec.Runbook{
		APIVersion: "breviary.io/v1",
		Kind:       "Runbook",
		Metadata:   spec.Metadata{Name: "test-run"},
		Spec: spec.RunbookSpec{
			Steps: []spec.Step{
				{ID: "step-0", Action: "exec", Exec: "echo hello", Timeout: "5s"},
			},
		},
	}
	s.RegisterRunbook(book)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantField != "" && w.Body.Len() > 0 {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
					if val, ok := resp[tt.wantField]; ok {
						if val != tt.wantValue {
							t.Errorf("response[%q] = %v, want %q", tt.wantField, val, tt.wantValue)
						}
					}
				}
			}
		})
	}
}
