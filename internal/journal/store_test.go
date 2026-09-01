package journal

import (
	"os"
	"testing"
)

func setupTest(t *testing.T) *Journal {
	t.Helper()
	tmp := t.TempDir() + "/test.db"
	j, err := Open(Config{Path: tmp})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return j
}

func TestCreateRunAndResume(t *testing.T) {
	j := setupTest(t)
	defer j.Close()

	// Create a run with a pinned spec
	run, err := j.CreateRun("run-test-001", "db-disk-space", `{"steps":[]}`)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Status != "running" {
		t.Errorf("expected status running, got %s", run.Status)
	}

	// Write checkpoint for the first step (BEFORE side-effect — journal-first)
	err = j.AppendStepEvent("run-test-001", "check-usage", "checkpoint", `{}`)
	if err != nil {
		t.Fatalf("AppendStepEvent checkpoint: %v", err)
	}

	// Write success for the step (AFTER side-effect)
	err = j.AppendStepEvent("run-test-001", "check-usage", "success", `{"exit_code":0,"output":"/dev 10G 2G 8G 25%"}`)
	if err != nil {
		t.Fatalf("AppendStepEvent success: %v", err)
	}

	// Verify: step is marked as done (memoization)
	done, err := j.HasStep("run-test-001", "check-usage")
	if err != nil {
		t.Fatalf("HasStep: %v", err)
	}
	if !done {
		t.Error("expected step check-usage to be done after success")
	}

	// Verify: incomplete step is not marked done
	done, err = j.HasStep("run-test-001", "top-tables")
	if err != nil {
		t.Fatalf("HasStep: %v", err)
	}
	if done {
		t.Error("expected step top-tables to NOT be done")
	}

	// Complete the run
	err = j.CompleteRun("run-test-001", "succeeded")
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	// Verify load for resume
	loaded, err := j.LoadRun("run-test-001")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loaded.Status != "succeeded" {
		t.Errorf("expected status succeeded, got %s", loaded.Status)
	}
	if loaded.BookName != "db-disk-space" {
		t.Errorf("expected book_name db-disk-space, got %s", loaded.BookName)
	}

	// Verify audit trail
	events, err := j.StepEvents("run-test-001")
	if err != nil {
		t.Fatalf("StepEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Kind != "checkpoint" {
		t.Errorf("event[0] kind: expected checkpoint, got %s", events[0].Kind)
	}
	if events[1].Kind != "success" {
		t.Errorf("event[1] kind: expected success, got %s", events[1].Kind)
	}
}

func TestResumeAfterCrash(t *testing.T) {
	j := setupTest(t)
	defer j.Close()

	// Simulate: run created, steps 1-2 completed, crash on step 3
	j.CreateRun("run-crash-001", "test", `{}`)

	// Step 1: success
	j.AppendStepEvent("run-crash-001", "step-1", "checkpoint", `{}`)
	j.AppendStepEvent("run-crash-001", "step-1", "success", `{"exit_code":0}`)

	// Step 2: success
	j.AppendStepEvent("run-crash-001", "step-2", "checkpoint", `{}`)
	j.AppendStepEvent("run-crash-001", "step-2", "success", `{"exit_code":0}`)

	// Step 3: checkpoint was written, but success — no (crash occurred)
	j.AppendStepEvent("run-crash-001", "step-3", "checkpoint", `{}`)
	// ← process crashed here. success NOT written.

	// Resume: verify step-1 and step-2 are done, step-3 is NOT
	for _, sid := range []string{"step-1", "step-2"} {
		done, err := j.HasStep("run-crash-001", sid)
		if err != nil || !done {
			t.Errorf("expected step %s to be done (has success event)", sid)
		}
	}

	// step-3: checkpoint exists, but success does not → NOT done → needs retry
	done, err := j.HasStep("run-crash-001", "step-3")
	if err != nil {
		t.Fatalf("HasStep step-3: %v", err)
	}
	if done {
		t.Error("expected step-3 to NOT be done (checkpoint without success)")
	}
}

func TestRunLifecycle(t *testing.T) {
	j := setupTest(t)
	defer j.Close()

	// Use a temp file
	tmpDB := t.TempDir() + "/lifecycle.db"
	j2, err := Open(Config{Path: tmpDB})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer j2.Close()

	run, err := j2.CreateRun("run-life-001", "test", `{}`)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.ID != "run-life-001" {
		t.Errorf("expected id run-life-001, got %s", run.ID)
	}

	// Verify: run without events → HasStep false
	done, err := j2.HasStep("run-life-001", "step-1")
	if err != nil {
		t.Fatalf("HasStep: %v", err)
	}
	if done {
		t.Error("expected step-1 NOT done for empty journal")
	}
}

func TestCleanup(t *testing.T) {
	// Verify: journal opens and closes without errors
	j := setupTest(t)
	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close does not panic
	j.db.Close()
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}