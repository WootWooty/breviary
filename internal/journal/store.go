// Package journal — персистентный журнал шагов (SQLite WAL)
// design: journal-first (checkpoint до side-effect, record после).
// SQLite WAL достаточно для single-server до ~10k DAU (Turso/rqlite валидируют).
package journal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Config — параметры подключения к БД
type Config struct {
	Path string // путь к файлу SQLite (по умолч.: ~/.config/breviary/breviary.db)
}

// Journal — хранилище запусков и шагов
type Journal struct {
	db *sql.DB
}

// RunRow — запись запуска runbook
type RunRow struct {
	ID        string    // "run-<ts>-<rand>"
	BookName  string    // имя runbook
	Spec      string    // PINNED spec (версионирование: spec фиксируется при старте)
	Status    string    // running / succeeded / failed / aborted
	CreatedAt time.Time
	UpdatedAt time.Time
}

// StepEvent — событие шага (каждый checkpoint/success/failure)
type StepEvent struct {
	RunID  string
	StepID string
	Kind   string // checkpoint / success / failure / skipped / approval-waiting / approval-ok / approval-rejected
	Data   string // JSON с результатом (Result из spec)
	Seq    int    // порядковый номер для хронологии
}

// Open открывает SQLite, создаёт схему, включает WAL
func Open(cfg Config) (*Journal, error) {
	if cfg.Path == "" {
		cfg.Path = "breviary.db"
	}

	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("journal: open %s: %w", cfg.Path, err)
	}

	// Включаем WAL + busy timeout + синхронность
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA busy_timeout = 5000")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA foreign_keys = ON")

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("journal: migrate: %w", err)
	}

	return &Journal{db: db}, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		book_name TEXT NOT NULL,
		spec TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'running',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS step_events (
		run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
		step_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		data TEXT,
		seq INTEGER NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (run_id, step_id, seq)
	);

	CREATE INDEX IF NOT EXISTS idx_step_events_run ON step_events(run_id);
	CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
	`
	_, err := db.Exec(schema)
	return err
}

// Close закрывает БД
func (j *Journal) Close() error {
	return j.db.Close()
}

// CreateRun создаёт запись запуска с закреплённой spec
func (j *Journal) CreateRun(id, bookName, specJSON string) (*RunRow, error) {
	_, err := j.db.Exec(
		"INSERT INTO runs (id, book_name, spec, status) VALUES (?, ?, ?, 'running')",
		id, bookName, specJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	return &RunRow{ID: id, BookName: bookName, Spec: specJSON, Status: "running"}, nil
}

// AppendStepEvent записывает событие шага в append-only журнал
// Важно: Checkpoint пишется ДО side-effect (journal-first)
func (j *Journal) AppendStepEvent(runID, stepID, kind, dataJSON string) error {
	// seq = count существующих событий для этого run+step
	var seq int
	err := j.db.QueryRow(
		"SELECT COALESCE(MAX(seq), 0) + 1 FROM step_events WHERE run_id = ? AND step_id = ?",
		runID, stepID,
	).Scan(&seq)
	if err != nil {
		return fmt.Errorf("seq: %w", err)
	}

	_, err = j.db.Exec(
		"INSERT INTO step_events (run_id, step_id, kind, data, seq) VALUES (?, ?, ?, ?, ?)",
		runID, stepID, kind, dataJSON, seq,
	)
	return err
}

// HasStep возвращает true, если шаг уже завершён (для memoization/resume)
func (j *Journal) HasStep(runID, stepID string) (bool, error) {
	var count int
	err := j.db.QueryRow(
		"SELECT COUNT(*) FROM step_events WHERE run_id = ? AND step_id = ? AND kind = 'success'",
		runID, stepID,
	).Scan(&count)
	return count > 0, err
}

// LastStepEvent возвращает последнее событие для шага (для resume)
func (j *Journal) LastStepEvent(runID, stepID string) (*StepEvent, error) {
	row := j.db.QueryRow(
		"SELECT run_id, step_id, kind, COALESCE(data,''), seq FROM step_events WHERE run_id = ? AND step_id = ? ORDER BY seq DESC LIMIT 1",
		runID, stepID,
	)
	var se StepEvent
	err := row.Scan(&se.RunID, &se.StepID, &se.Kind, &se.Data, &se.Seq)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &se, nil
}

// LoadRun загружает запуск по id (для resume)
func (j *Journal) LoadRun(runID string) (*RunRow, error) {
	row := j.db.QueryRow(
		"SELECT id, book_name, COALESCE(spec,'{}'), status, created_at, updated_at FROM runs WHERE id = ?",
		runID,
	)
	var r RunRow
	var createdStr, updatedStr string
	err := row.Scan(&r.ID, &r.BookName, &r.Spec, &r.Status, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
	r.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
	return &r, nil
}

// CompleteRun отмечает запуск как завершённый
func (j *Journal) CompleteRun(runID string, status string) error {
	_, err := j.db.Exec(
		"UPDATE runs SET status = ?, updated_at = datetime('now') WHERE id = ?",
		status, runID,
	)
	return err
}

// StepEvents возвращает все события для запуска (audit trail)
func (j *Journal) StepEvents(runID string) ([]StepEvent, error) {
	rows, err := j.db.Query(
		"SELECT run_id, step_id, kind, COALESCE(data,''), seq FROM step_events WHERE run_id = ? ORDER BY seq ASC",
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []StepEvent
	for rows.Next() {
		var se StepEvent
		if err := rows.Scan(&se.RunID, &se.StepID, &se.Kind, &se.Data, &se.Seq); err != nil {
			return nil, err
		}
		events = append(events, se)
	}
	return events, nil
}

// HasRecentRun проверяет, был ли успешный запуск bookName в окне window
// (для dedup: если alert пришёл повторно в dedup-окне — пропускаем)
func (j *Journal) HasRecentRun(bookName string, window time.Duration) (bool, error) {
	var count int
	err := j.db.QueryRow(
		`SELECT COUNT(*) FROM runs 
		 WHERE book_name = ? AND status = 'succeeded' 
		 AND created_at >= datetime('now', ?)`,
		bookName, fmt.Sprintf("-%d seconds", int(window.Seconds())),
	).Scan(&count)
	return count > 0, err
}

// CountRecentRuns считает количество запусков bookName в окне window
// (для throttle: max N запусков в час)
func (j *Journal) CountRecentRuns(bookName string, window time.Duration) (int, error) {
	var count int
	err := j.db.QueryRow(
		`SELECT COUNT(*) FROM runs 
		 WHERE book_name = ? 
		 AND created_at >= datetime('now', ?)`,
		bookName, fmt.Sprintf("-%d seconds", int(window.Seconds())),
	).Scan(&count)
	return count, err
}

// RunningCount считает количество запусков bookName в статусе running
// (для concurrency: максимум 1 одновременный запуск)
func (j *Journal) RunningCount(bookName string) (int, error) {
	var count int
	err := j.db.QueryRow(
		"SELECT COUNT(*) FROM runs WHERE book_name = ? AND status = 'running'",
		bookName,
	).Scan(&count)
	return count, err
}

// MarshalData упаковывает произвольные данные в JSON для журнала
func MarshalData(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}