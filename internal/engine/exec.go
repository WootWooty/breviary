// Package engine — core executor (journal-first, memoization, resume, retry)
// design: ~400 LOC ядро, action dispatch через Registry.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/breviary/breviary/internal/actions"
	"github.com/breviary/breviary/internal/expr"
	"github.com/breviary/breviary/internal/journal"
	"github.com/breviary/breviary/internal/spec"
	"github.com/breviary/breviary/internal/templates"
)

// Config — параметры движка
type Config struct {
	JournalPath string // путь к SQLite БД
	Logger      *slog.Logger
}

// Engine — исполнитель runbook
type Engine struct {
	j        *journal.Journal
	actions  actions.Registry
	cel      *expr.Evaluator
	log      *slog.Logger
	approval *ApprovalManager
}

// New создаёт Engine с открытым журналом, реестром actions и CEL-движком
func New(cfg Config) (*Engine, error) {
	j, err := journal.Open(journal.Config{Path: cfg.JournalPath})
	if err != nil {
		return nil, err
	}
	cel, err := expr.New()
	if err != nil {
		j.Close()
		return nil, fmt.Errorf("engine: cel: %w", err)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	log.Info("engine", "journal", cfg.JournalPath)
	approvalMgr := newApprovalManager(log)
	return &Engine{
		j:        j,
		actions:  actions.NewRegistry(),
		cel:      cel,
		log:      log,
		approval: approvalMgr,
	}, nil
}

// Close закрывает журнал и CEL
func (e *Engine) Close() error {
	e.cel.Close()
	return e.j.Close()
}

// Run выполняет runbook от начала до конца.
// runID: переиспользует существующий (resume) или создаёт новый (fresh run).
func (e *Engine) Run(ctx context.Context, book *spec.Runbook, runID string) error {
	if runID == "" {
		runID = fmt.Sprintf("run-%d-%x", time.Now().Unix(), time.Now().UnixNano()%0xffff)
	}

	// TRIGGER CHECKS: dedup/concurrency/throttle
	// Проверяем перед созданием run, чтобы не плодить записи
	if book.Spec.Trigger.Dedup != "" || book.Spec.Trigger.Throttle != "" || book.Spec.Trigger.Concurrency > 0 {
		tr := book.Spec.Trigger

		// Concurrency: максимум N одновременных запусков
		concurrency := 1
		if tr.Concurrency > 0 {
			concurrency = tr.Concurrency
		}
		running, err := e.j.RunningCount(book.Metadata.Name)
		if err != nil {
			return fmt.Errorf("engine: concurrency check: %w", err)
		}
		if running >= concurrency {
			return fmt.Errorf("engine: concurrency limit hit (%d running, max %d)", running, concurrency)
		}

		// Dedup: если успешный запуск был в окне dedup — пропускаем
		if tr.Dedup != "" {
			d, err := time.ParseDuration(tr.Dedup)
			if err == nil && d > 0 {
				recent, err2 := e.j.HasRecentRun(book.Metadata.Name, d)
				if err2 != nil {
					return fmt.Errorf("engine: dedup check: %w", err2)
				}
				if recent {
					return fmt.Errorf("engine: dedup: run already succeeded within %s", tr.Dedup)
				}
			}
		}

		// Throttle: не больше 1 запуска в окне throttle
		if tr.Throttle != "" {
			d, err := time.ParseDuration(tr.Throttle)
			if err == nil && d > 0 {
				count, err2 := e.j.CountRecentRuns(book.Metadata.Name, d)
				if err2 != nil {
					return fmt.Errorf("engine: throttle check: %w", err2)
				}
				if count > 0 {
					return fmt.Errorf("engine: throttle: run already attempted within %s", tr.Throttle)
				}
			}
		}
	}

	// Pinned spec для versioning
	specJSON := journal.MarshalData(book)

	// Создаём или переиспользуем run
	existing, _ := e.j.LoadRun(runID)
	if existing == nil {
		if _, err := e.j.CreateRun(runID, book.Metadata.Name, specJSON); err != nil {
			return fmt.Errorf("engine: create run: %w", err)
		}
	}

	// Аккумулятор результатов шагов (для CEL-контекста)
	stepResults := make(map[string]interface{})

	// Выполнение шагов
	for i := range book.Spec.Steps {
		step := &book.Spec.Steps[i]

		// MEMOIZATION: шаг уже выполнен → пропускаем (resume)
		done, err := e.j.HasStep(runID, step.ID)
		if err != nil {
			return fmt.Errorf("engine: check step %s: %w", step.ID, err)
		}
		if done {
			// Загружаем предыдущий результат для контекста CEL
			if ev, _ := e.j.LastStepEvent(runID, step.ID); ev != nil && ev.Data != "" && ev.Kind == "success" {
				var res spec.Result
				if json.Unmarshal([]byte(ev.Data), &res) == nil {
					stepResults[step.ID] = resultToCEL(res)
				}
			}
			continue
		}

		// CONDITION: CEL-выражение в when
		if step.When != "" {
			ok, err := e.evalWhen(step.When, stepResults, book)
			if err != nil {
				return fmt.Errorf("engine: when %s: %w", step.ID, err)
			}
			if !ok {
				e.j.AppendStepEvent(runID, step.ID, "skipped", `{}`)
				continue
			}
		}

		// APPROVAL GATE
		if step.Approval != nil {
			status, err2 := e.ApprovalStatus(runID, step.ID)
			if err2 != nil {
				return fmt.Errorf("engine: approval check %s: %w", step.ID, err2)
			}
			if status == "rejected" {
				e.j.AppendStepEvent(runID, step.ID, "failure", `{"error":"approval rejected"}`)
				e.j.CompleteRun(runID, "rejected")
				return fmt.Errorf("engine: step %s: approval rejected", step.ID)
			}
			if status != "approved" {
				// Рендер шаблона show с результатами предыдущих шагов
				showMsg := step.Approval.Show
				if templates.HasTemplate(showMsg) {
					if rendered, err := templates.Render(showMsg, stepResults); err == nil {
						showMsg = rendered
					}
				}
				e.j.AppendStepEvent(runID, step.ID, "approval-waiting",
					journal.MarshalData(map[string]string{
						"show":    showMsg,
						"channel": step.Approval.Channel,
					}))

				// Запускаем таймеры таймаута и эскалации
				timeout := step.Approval.Timeout
				if timeout <= 0 {
					timeout = 30 * time.Minute
				}
				escalate := step.Approval.EscalateAfter
				e.approval.Watch(runID, step.ID, timeout, escalate)

				e.j.CompleteRun(runID, "pending-approval")
				return fmt.Errorf("engine: step %s: %w", step.ID, ErrApprovalPending)
			}
		}

		// JOURNAL-FIRST: checkpoint ДО side-effect
		e.j.AppendStepEvent(runID, step.ID, "checkpoint", `{}`)

		// EXECUTION через actions.Registry
		state := actions.State{
			StepResults: stepResults,
			RunName:     book.Metadata.Name,
			RunID:       runID,
		}
		res := e.runStep(ctx, step, state)
		if res.Error != "" && step.Retry != nil {
			for attempt := 1; attempt <= step.Retry.Max; attempt++ {
				e.j.AppendStepEvent(runID, step.ID, "retry",
					journal.MarshalData(map[string]interface{}{
						"attempt": attempt,
						"error":   res.Error,
					}))
				time.Sleep(step.Retry.Backoff * time.Duration(attempt))
				res = e.runStep(ctx, step, state)
				if res.Error == "" {
					break
				}
			}
		}

		// RECORD: записываем результат
		status := "success"
		if res.Error != "" {
			status = "failure"
		}
		e.j.AppendStepEvent(runID, step.ID, status, journal.MarshalData(res))

		// Сохраняем результат для последующих CEL-выражений
		stepResults[step.ID] = resultToCEL(res)

		// FAILURE HANDLING
		if res.Error != "" {
			switch step.OnFailure {
			case "rollback":
				e.rollback(ctx, runID, book, i)
				e.j.CompleteRun(runID, "rolled-back")
				return fmt.Errorf("engine: step %s failed, rolled back: %s", step.ID, res.Error)
			case "continue":
				continue
			case "stop":
				fallthrough
			default:
				e.j.CompleteRun(runID, "failed")
				return fmt.Errorf("engine: step %s failed: %s", step.ID, res.Error)
			}
		}
	}

	return e.j.CompleteRun(runID, "succeeded")
}

// evalWhen вычисляет CEL-выражение when с контекстом шагов, env и run
func (e *Engine) evalWhen(expr string, stepResults map[string]interface{}, book *spec.Runbook) (bool, error) {
	ctx := makeCELContext(stepResults, book)
	return e.cel.EvalBool(expr, ctx)
}

// makeCELContext собирает карту для CEL из результатов шагов, окружения и метаданных
func makeCELContext(stepResults map[string]interface{}, book *spec.Runbook) map[string]interface{} {
	// Переменные окружения
	envMap := make(map[string]interface{})
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Метаданные запуска
	runMap := map[string]interface{}{
		"name":  book.Metadata.Name,
		"owner": book.Metadata.Owner,
	}

	return map[string]interface{}{
		"steps": stepResults,
		"env":   envMap,
		"run":   runMap,
	}
}

// resultToCEL конвертирует Result в CEL-friendly map
func resultToCEL(res spec.Result) map[string]interface{} {
	m := map[string]interface{}{
		"exit_code": int64(res.ExitCode),
		"status":    string(res.Status),
		"output":    res.Output,
	}
	if res.Duration != "" {
		m["duration"] = res.Duration
	}
	if res.Error != "" {
		m["error"] = res.Error
	}
	return m
}

// runStep делегирует выполнение шага в actions.Registry
func (e *Engine) runStep(ctx context.Context, step *spec.Step, state actions.State) spec.Result {
	action, ok := e.actions[step.Action]
	if !ok {
		return spec.Result{
			StepID: step.ID,
			Status: spec.StatusFailure,
			Error:  fmt.Sprintf("unknown action %q", step.Action),
		}
	}
	return action.Run(ctx, step, state)
}

// Resume восстанавливает прерванный запуск по runID
func (e *Engine) Resume(ctx context.Context, runID string) error {
	run, err := e.j.LoadRun(runID)
	if err != nil {
		return fmt.Errorf("engine: load run %s: %w", runID, err)
	}

	var book spec.Runbook
	if err := json.Unmarshal([]byte(run.Spec), &book); err != nil {
		return fmt.Errorf("engine: parse pinned spec: %w", err)
	}

	return e.Run(ctx, &book, runID)
}

// Events возвращает журнал шагов для audit trail
func (e *Engine) Events(runID string) ([]journal.StepEvent, error) {
	return e.j.StepEvents(runID)
}

// rollback выполняет undo-шаги в обратном порядке (Saga-компенсация)
func (e *Engine) rollback(ctx context.Context, runID string, book *spec.Runbook, failedIdx int) {
	for i := failedIdx; i >= 0; i-- {
		step := &book.Spec.Steps[i]
		if step.Undo == nil {
			continue
		}
		e.j.AppendStepEvent(runID, step.Undo.ID, "undo", `{}`)
		_ = e.runStep(ctx, step.Undo, actions.State{})
	}
}

// Backup executeStep — временный мост для тестов engine
func executeStep(ctx context.Context, step *spec.Step, attempt int) spec.Result {
	var stdout, stderr bytes.Buffer
	res := spec.Result{StepID: step.ID, Status: spec.StatusSuccess}

	cmd := exec.CommandContext(ctx, "sh", "-c", step.Exec)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	res.Duration = time.Since(start).Round(time.Millisecond).String()
	res.Output = stdout.String()

	if err != nil {
		res.Error = stderr.String()
		if res.Error == "" {
			res.Error = err.Error()
		}
		res.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		}
		res.Status = spec.StatusFailure
	} else {
		res.ExitCode = 0
	}
	return res
}