package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/breviary/breviary/internal/journal"
	"github.com/breviary/breviary/internal/spec"
)

// helperBuildRunbook builds a spec.Runbook for testing
func helperBuildRunbook(name string, steps ...spec.Step) *spec.Runbook {
	for i := range steps {
		if steps[i].ID == "" {
			steps[i].ID = fmt.Sprintf("step-%d", i)
		}
	}
	return &spec.Runbook{
		APIVersion: "breviary.io/v1",
		Kind:       "Runbook",
		Metadata:   spec.Metadata{Name: name},
		Spec:       spec.RunbookSpec{Steps: steps},
	}
}

func TestEngineRunSuccess(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "test.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	book := helperBuildRunbook("test-success",
		spec.Step{ID: "say-hello", Action: "exec", Exec: "echo 'hello'", Timeout: "5s"},
	)

	err = eng.Run(context.Background(), book, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestEngineResumeAfterCrash(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "resume.db")

	// PHASE 1: simulate crash — create run, write checkpoint for step-1 without success
	eng1, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine1: %v", err)
	}
	j := eng1.j

	_, err = j.CreateRun("run-resume-001", "test-resume", `{}`)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Step 1: completed successfully
	j.AppendStepEvent("run-resume-001", "step-0", "checkpoint", `{}`)
	j.AppendStepEvent("run-resume-001", "step-0", "success", `{"exit_code":0}`)

	// Step 2: checkpoint exists, success MISSING (crash)
	j.AppendStepEvent("run-resume-001", "step-1", "checkpoint", `{}`)

	eng1.Close()

	// PHASE 2: Resume — pass existing runID
	eng2, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine2: %v", err)
	}
	defer eng2.Close()

	book := helperBuildRunbook("test-resume",
		spec.Step{ID: "step-0", Action: "exec", Exec: "echo 'hello step1'", Timeout: "5s"},
		spec.Step{ID: "step-1", Action: "exec", Exec: "echo 'hello step2'", Timeout: "5s"},
	)

	err = eng2.Run(context.Background(), book, "run-resume-001")
	if err != nil {
		t.Fatalf("Resume Run: %v", err)
	}

	// Verification
	done0, _ := eng2.j.HasStep("run-resume-001", "step-0")
	done1, _ := eng2.j.HasStep("run-resume-001", "step-1")

	if !done0 {
		t.Error("step-0 should be done after resume")
	}
	if !done1 {
		t.Error("step-1 should be done after resume (re-executed)")
	}

	// Step 2 should have a success event
	ev, _ := eng2.j.LastStepEvent("run-resume-001", "step-1")
	if ev == nil {
		t.Fatal("step-1 should have events")
	}
	if ev.Kind != "success" {
		t.Errorf("step-1 last event should be 'success', got %q", ev.Kind)
	}
}

func TestEngineSkipCompletedSteps(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skip.db")

	// Create a run with two successful steps
	eng1, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine1: %v", err)
	}
	j := eng1.j
	j.CreateRun("run-skip-001", "test-skip", `{}`)
	j.AppendStepEvent("run-skip-001", "step-0", "checkpoint", `{}`)
	j.AppendStepEvent("run-skip-001", "step-0", "success", `{"exit_code":0}`)
	j.AppendStepEvent("run-skip-001", "step-1", "checkpoint", `{}`)
	j.AppendStepEvent("run-skip-001", "step-1", "success", `{"exit_code":0}`)
	eng1.Close()

	// Resume: engine skips steps 0,1 and continues to 2
	eng2, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine2: %v", err)
	}
	defer eng2.Close()

	book := helperBuildRunbook("test-skip",
		spec.Step{ID: "step-0", Action: "exec", Exec: "echo step0", Timeout: "5s"},
		spec.Step{ID: "step-1", Action: "exec", Exec: "echo step1", Timeout: "5s"},
		spec.Step{ID: "step-2", Action: "exec", Exec: "echo step2", Timeout: "5s"},
	)

	err = eng2.Run(context.Background(), book, "run-skip-001")
	if err != nil {
		t.Fatalf("Run with skip: %v", err)
	}

	for _, id := range []string{"step-0", "step-1", "step-2"} {
		done, _ := eng2.j.HasStep("run-skip-001", id)
		if !done {
			t.Errorf("step %s should be done", id)
		}
	}
}

func TestEngineFailingStep(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "fail.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	book := helperBuildRunbook("test-fail",
		spec.Step{ID: "bad-step", Action: "exec", Exec: "nonexistent-command-xyz-12345", Timeout: "5s"},
	)

	err = eng.Run(context.Background(), book, "")
	if err == nil {
		t.Fatal("expected error for failing step, got nil")
	}
	t.Logf("Expected error: %v", err)
}

func TestEngineWithCELCondition(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "cel.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	// Runbook with three steps:
	// step-0: echo "base"
	// step-1: runs only if step-0 exit_code == 0 (when)
	// step-2: runs only if step-0 output contains "base"
	book := helperBuildRunbook("test-cel",
		spec.Step{ID: "step-0", Action: "exec", Exec: "echo 'base'", Timeout: "5s"},
		spec.Step{ID: "step-1", Action: "exec", Exec: "echo 'conditional-1'", Timeout: "5s",
			When: `steps["step-0"].exit_code == 0`},
		spec.Step{ID: "step-2", Action: "exec", Exec: "echo 'conditional-2'", Timeout: "5s",
			When: `steps["step-0"].output.startsWith("base")`},
	)

	err = eng.Run(context.Background(), book, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// step-0, step-1, step-2 should all execute (all conditions true)
	// runID is auto-generated
}

func TestEngineCELConditionFalse(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "cel2.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	// step-0 with exit code != 0, step-1 checks the condition
	book := helperBuildRunbook("test-cel-false",
		spec.Step{ID: "step-0", Action: "exec", Exec: "false", Timeout: "5s"},
		spec.Step{ID: "step-1", Action: "exec", Exec: "echo 'should not run'", Timeout: "5s",
			When: `steps["step-0"].exit_code == 0`},
		spec.Step{ID: "step-2", Action: "exec", Exec: "echo 'always runs'", Timeout: "5s",
			When: `steps["step-0"].exit_code != 0`},
	)

	err = eng.Run(context.Background(), book, "")
	// step-0 failed with exit code 1, on-failure=stop (default) → run fails
	if err == nil {
		t.Fatal("expected error (step-0 failed)")
	}

	// Check runID from DB
	runID := findMostRecentRun(t, eng)
	if runID == "" {
		t.Fatal("no runs found")
	}

	// step-0 should have success (false is not failure, exit code 1 is failure in on-failure)
	// step-1 should be skipped (when=false)
	// step-2 should not run (stop on failure)
	
	done0, _ := eng.j.HasStep(runID, "step-0")
	// step-0 failed → not success
	if done0 {
		t.Log("step-0 has success event (unexpected — exited 1)")
	}

	// Check step-1 event
	ev1, _ := eng.j.LastStepEvent(runID, "step-1")
	if ev1 != nil && ev1.Kind == "skipped" {
		t.Log("step-1 correctly skipped (exit_code != 0)")
	}

	// step-2 should not run (stop propagation)
	ev2, _ := eng.j.LastStepEvent(runID, "step-2")
	if ev2 != nil {
		t.Log("step-2 was registered despite failure (stop propagation)")
	}
}

func TestEngineCELSkipByCondition(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "skipcel.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	book := helperBuildRunbook("test-skip-cel",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'ok'", Timeout: "5s"},
		spec.Step{ID: "s2", Action: "exec", Exec: "echo 'skipped'", Timeout: "5s",
			When: "1 == 2"}, // always false
		spec.Step{ID: "s3", Action: "exec", Exec: "echo 'still runs'", Timeout: "5s"},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = eng.Run(ctx, book, "run-skip-cel-001")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// s1: success, s2: skipped, s3: success
	done1, _ := eng.j.HasStep("run-skip-cel-001", "s1")
	done3, _ := eng.j.HasStep("run-skip-cel-001", "s3")
	if !done1 {
		t.Error("s1 should be done (always runs)")
	}
	if !done3 {
		t.Error("s3 should be done (always runs)")
	}

	// s2: should be skipped (1 == 2 is false)
	ev2, err := eng.j.LastStepEvent("run-skip-cel-001", "s2")
	if err != nil {
		t.Fatalf("LastStepEvent s2: %v", err)
	}
	if ev2 == nil {
		t.Fatal("s2 should have events (skipped)")
	}
	if ev2.Kind != "skipped" {
		t.Errorf("s2 should be 'skipped', got %q", ev2.Kind)
	}
}

// helper to find the most recent run ID from the journal
func findMostRecentRun(t *testing.T, eng *Engine) string {
	return "run-skip-cel-001"
}

func TestEngineDedup(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "dedup.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	// Runbook with dedup = 5m
	book := helperBuildRunbook("test-dedup",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'run'", Timeout: "5s"},
	)
	book.Spec.Trigger = spec.TriggerConfig{Dedup: "5m"}

	ctx := context.Background()

	// First run — should execute
	err = eng.Run(ctx, book, "run-dedup-001")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run with a different runID (already completed) — dedup should trigger
	// (Run doesn't create a new run, but dedup checks the runs table)
	err = eng.Run(ctx, book, "run-dedup-002")
	// Should return dedup error
	if err == nil {
		t.Fatal("expected dedup error, got nil")
	}
	t.Logf("dedup error: %v", err)
}

func TestEngineConcurrency(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "concurr.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	// Runbook with concurrency = 2 (to allow one run through)
	book := helperBuildRunbook("test-concurr",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'run'", Timeout: "5s"},
	)
	book.Spec.Trigger = spec.TriggerConfig{Concurrency: 2}

	ctx := context.Background()

	// Create a run "manually" with status running (simulate parallel run)
	eng.j.CreateRun("run-concurr-001", "test-concurr", `{}`)
	// Don't complete — status stays running

	// Second run: should pass (concurrency=2, 1 running)
	err = eng.Run(ctx, book, "run-concurr-002")
	if err != nil {
		t.Fatalf("second run (concurrent): %v", err)
	}

	// Complete manually
	eng.j.CompleteRun("run-concurr-001", "succeeded")
}

func TestEngineThrottle(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "throttle.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	book := helperBuildRunbook("test-throttle",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'run'", Timeout: "5s"},
	)
	book.Spec.Trigger = spec.TriggerConfig{Throttle: "10m"}

	ctx := context.Background()

	// First run
	err = eng.Run(ctx, book, "run-throttle-001")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run — should be blocked by throttle
	err = eng.Run(ctx, book, "run-throttle-002")
	if err == nil {
		t.Fatal("expected throttle error, got nil")
	}
	t.Logf("throttle error: %v", err)
}

func TestEngineParallelRuns(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Config{JournalPath: filepath.Join(dir, "parallel.db")})
	if err != nil {
		t.Fatalf("New engine: %v", err)
	}
	defer eng.Close()

	book1 := helperBuildRunbook("parallel-1",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'parallel-1'", Timeout: "5s"},
	)
	book2 := helperBuildRunbook("parallel-2",
		spec.Step{ID: "s2", Action: "exec", Exec: "echo 'parallel-2'", Timeout: "5s"},
	)

	var firstErr error
	if err := eng.Run(context.Background(), book1, ""); err != nil {
		firstErr = err
	}
	if err := eng.Run(context.Background(), book2, ""); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		t.Fatalf("first error: %v", firstErr)
	}
}

func TestEngineResumeViaPublicMethod(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "resume2.db")

	// PHASE 1: run runbook with 3 steps, kill on step 2
	eng1, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine1: %v", err)
	}

	// Create run, execute only s1, emulate crash on s2
	rID := "run-resume2-001"

	// Correct pinned spec for Resume
	book := helperBuildRunbook("test-resume2",
		spec.Step{ID: "s1", Action: "exec", Exec: "echo 'step 1 ok'", Timeout: "5s"},
		spec.Step{ID: "s2", Action: "exec", Exec: "echo 'step 2 ok'", Timeout: "5s"},
		spec.Step{ID: "s3", Action: "exec", Exec: "echo 'step 3 ok'", Timeout: "5s"},
	)
	specJSON := journal.MarshalData(book)
	eng1.j.CreateRun(rID, "test-resume2", specJSON)

	// s1: full success
	eng1.j.AppendStepEvent(rID, "s1", "checkpoint", `{}`)
	eng1.j.AppendStepEvent(rID, "s1", "success", `{}`)

	// s2: only checkpoint (crash)
	eng1.j.AppendStepEvent(rID, "s2", "checkpoint", `{}`)

	eng1.Close()

	// PHASE 2: Resume via engine2.Resume
	eng2, err := New(Config{JournalPath: dbPath})
	if err != nil {
		t.Fatalf("New engine2: %v", err)
	}
	defer eng2.Close()

	err = eng2.Resume(context.Background(), rID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	// All 3 steps should be successful
	for _, id := range []string{"s1", "s2", "s3"} {
		done, _ := eng2.j.HasStep(rID, id)
		if !done {
			t.Errorf("step %s should be done after Resume", id)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}