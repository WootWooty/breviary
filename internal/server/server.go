// Package server — HTTP daemon for webhook ingest and CLI approval
// Endpoints:
//   POST /api/v1/run      — accept alert, execute runbook
//   POST /api/v1/approve   — approve a step
//   POST /api/v1/reject    — reject a step
//   GET  /api/v1/health    — liveness probe
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/breviary/breviary/internal/engine"
	"github.com/breviary/breviary/internal/spec"
)

// Server — HTTP server
type Server struct {
	eng      *engine.Engine
	mux      *http.ServeMux
	srv      *http.Server
	runbooks map[string]*spec.Runbook // bookName → parsed runbook
}

// New creates an HTTP server with the given engine
func New(eng *engine.Engine) *Server {
	s := &Server{
		eng:      eng,
		mux:      http.NewServeMux(),
		runbooks: make(map[string]*spec.Runbook),
	}

	s.mux.HandleFunc("POST /api/v1/run", s.handleRun)
	s.mux.HandleFunc("POST /api/v1/approve", s.handleApprove)
	s.mux.HandleFunc("POST /api/v1/reject", s.handleReject)
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	return s
}

// RegisterRunbook adds a runbook to the routing (by metadata.name)
func (s *Server) RegisterRunbook(book *spec.Runbook) {
	s.runbooks[book.Metadata.Name] = book
}

// Listen starts the HTTP server on addr
func (s *Server) Listen(addr string) error {
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// handleRun — accept a webhook alert and execute the runbook
// POST /api/v1/run
// Body: {"alert":"DiskUsageHigh","severity":"critical","labels":{"host":"db1"}}
// Header: X-Runbook-Name: db-disk-space (optional, for direct invocation)
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Alert    string            `json:"alert"`
		Severity string            `json:"severity"`
		Labels   map[string]string `json:"labels,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Determine runbook by header or by alert → name matching
	bookName := r.Header.Get("X-Runbook-Name")
	if bookName == "" && payload.Alert != "" {
		bookName = payload.Alert // simplest mapping: alert name = runbook name
	}
	if bookName == "" {
		http.Error(w, `{"error":"no runbook name or alert"}`, http.StatusBadRequest)
		return
	}

	book, ok := s.runbooks[bookName]
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"runbook %q not found"}`, bookName), http.StatusNotFound)
		return
	}

	ctx := r.Context()
	runID := fmt.Sprintf("hook-%s-%d", bookName, time.Now().Unix())

	if err := s.eng.Run(ctx, book, runID); err != nil {
		// Pending approval is not a server error
		if err.Error() == "approval pending" {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "pending-approval",
				"run_id": runID,
			})
			return
		}
		// Dedup/throttle is not a 500 either
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "rejected",
			"error":  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"run_id": runID,
	})
}

// handleApprove — approve a step
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RunID  string `json:"run_id"`
		StepID string `json:"step_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.eng.ApproveStep(payload.RunID, payload.StepID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

// handleReject — reject a step
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RunID  string `json:"run_id"`
		StepID string `json:"step_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.eng.RejectStep(payload.RunID, payload.StepID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
}

// handleHealth — liveness probe
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ValidateRunbook connects engine + spec validation
func ValidateRunbook(path string) error {
	book, err := spec.ValidateYAML(path)
	if err != nil {
		return err
	}
	_ = book
	return nil
}