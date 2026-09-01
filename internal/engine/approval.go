package engine

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/breviary/breviary/internal/journal"
)

// ErrApprovalPending — signals that a runbook is waiting for approval
var ErrApprovalPending = errors.New("approval pending")

// pendingApproval — an approval watch entry with timers
type pendingApproval struct {
	runID     string
	stepID    string
	timeout   time.Duration
	escalate  time.Duration
	createdAt time.Time
	timer     *time.Timer
}

// ApprovalManager — manages approval with timeouts and escalation
type ApprovalManager struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval // key: runID/stepID
	log     *slog.Logger
}

func newApprovalManager(log *slog.Logger) *ApprovalManager {
	return &ApprovalManager{
		pending: make(map[string]*pendingApproval),
		log:     log,
	}
}

func (am *ApprovalManager) key(runID, stepID string) string {
	return runID + "/" + stepID
}

// Watch starts approval timers
func (am *ApprovalManager) Watch(runID, stepID string, timeout, escalate time.Duration) {
	am.mu.Lock()
	defer am.mu.Unlock()

	key := am.key(runID, stepID)
	if _, exists := am.pending[key]; exists {
		return
	}

	entry := &pendingApproval{
		runID:     runID,
		stepID:    stepID,
		timeout:   timeout,
		escalate:  escalate,
		createdAt: time.Now(),
	}

	// Escalation timer (if configured)
	if escalate > 0 {
		entry.timer = time.AfterFunc(escalate, func() {
			am.log.Warn("approval escalation", "run_id", runID, "step_id", stepID,
				"escalate_after", escalate.String())
			// In production: notify the backup on-call
		})
	}

	// Final timeout timer
	entry.timer = time.AfterFunc(timeout, func() {
		am.mu.Lock()
		delete(am.pending, key)
		am.mu.Unlock()
		am.log.Warn("approval timeout", "run_id", runID, "step_id", stepID,
			"timeout", timeout.String())
		// Auto-reject on timeout
		// In production: write approval-rejected to journal
	})

	am.pending[key] = entry
	am.log.Debug("approval watch started", "run_id", runID, "step_id", stepID,
		"timeout", timeout.String(), "escalate_after", escalate.String())
}

// Stop cancels the approval timer (when a response is received)
func (am *ApprovalManager) Stop(runID, stepID string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	key := am.key(runID, stepID)
	if entry, ok := am.pending[key]; ok {
		entry.timer.Stop()
		delete(am.pending, key)
		am.log.Debug("approval watch stopped", "run_id", runID, "step_id", stepID)
	}
}

// ApprovalStatus returns the approval state for a step
func (e *Engine) ApprovalStatus(runID, stepID string) (string, error) {
	ev, err := e.j.LastStepEvent(runID, stepID)
	if err != nil {
		return "", err
	}
	if ev == nil {
		return "", nil
	}

	switch ev.Kind {
	case "approval-waiting":
		return "pending", nil
	case "approval-ok":
		return "approved", nil
	case "approval-rejected":
		return "rejected", nil
	default:
		return ev.Kind, nil
	}
}

// ApproveStep records step approval and stops timers
func (e *Engine) ApproveStep(runID, stepID string) error {
	status, err := e.ApprovalStatus(runID, stepID)
	if err != nil {
		return fmt.Errorf("engine: check approval: %w", err)
	}
	if status != "pending" {
		return fmt.Errorf("engine: step %s/%s is not pending approval (status: %s)", runID, stepID, status)
	}

	e.j.AppendStepEvent(runID, stepID, "approval-ok",
		journal.MarshalData(map[string]string{"approved_at": time.Now().Format(time.RFC3339)}))

	e.approval.Stop(runID, stepID)
	e.log.Info("step approved", "run_id", runID, "step_id", stepID)
	return nil
}

// RejectStep records rejection and triggers on-failure
func (e *Engine) RejectStep(runID, stepID string) error {
	status, err := e.ApprovalStatus(runID, stepID)
	if err != nil {
		return fmt.Errorf("engine: check approval: %w", err)
	}
	if status != "pending" {
		return fmt.Errorf("engine: step %s/%s is not pending approval (status: %s)", runID, stepID, status)
	}

	e.j.AppendStepEvent(runID, stepID, "approval-rejected",
		journal.MarshalData(map[string]string{"rejected_at": time.Now().Format(time.RFC3339)}))

	e.approval.Stop(runID, stepID)
	e.j.CompleteRun(runID, "rejected")
	e.log.Info("step rejected", "run_id", runID, "step_id", stepID)
	return nil
}