// Package journal — persistent step journal (SQLite WAL)
// design: journal-first (checkpoint before side-effect, record after).
// SQLite WAL is sufficient for single-server up to ~10k DAU (validated by Turso/rqlite).
package journal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Config — database connection parameters
type Config struct {
	Path string // path to SQLite file (default: ~/.config/breviary/breviary.db)
}

// Journal — run and step storage
type Journal struct {
	db *sql.DB
}

// RunRow — a runbook run record
type RunRow struct {
	ID        string // "run-<ts>-<rand>"
	BookName  string // runbook name
	Spec      string // PINNED spec (versioning: spec is frozen at run start)
	Status    string // running / succeeded / failed / aborted
	CreatedAt time.Time
	UpdatedAt time.Time
}

// StepEvent — a step event (each checkpoint/success/failure)
type StepEvent struct {
	RunID  string
	StepID string
	Kind   string // checkpoint / success / failure / skipped / approval-waiting / approval-ok / approval-rejected
	Data   string // JSON with the result (Result from spec)
	Seq    int    // sequence number for chronology
}

// Open opens SQLite, creates the schema, enables WAL
func Open(cfg Config) (*Journal, error) {
	if cfg.Path == "" {
		cfg.Path = "breviary.db"
	}

	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("journal: open %s: %w", cfg.Path, err)
	}

	// Enable WAL + busy timeout + synchronous mode
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

// Close closes the database
func (j *Journal) Close() error {
	return j.db.Close()
}

// CreateRun creates a run record with a pinned spec
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

// AppendStepEvent writes a step event to the append-only journal
// Important: Checkpoint is written BEFORE the side-effect (journal-first)
func (j *Journal) AppendStepEvent(runID, stepID, kind, dataJSON string) error {
	// seq = count of existing events for this run+step
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

// HasStep returns true if the step has a success event (for memoization/resume)
func (j *Journal) HasStep(runID, stepID string) (bool, error) {
	var count int
	err := j.db.QueryRow(
		"SELECT COUNT(*) FROM step_events WHERE run_id = ? AND step_id = ? AND kind = 'success'",
		runID, stepID,
	).Scan(&count)
	return count > 0, err
}

// LastStepEvent returns the last event for a step (for resume)
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

// LoadRun loads a run by ID (for resume)
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

// CompleteRun marks a run as completed
func (j *Journal) CompleteRun(runID string, status string) error {
	_, err := j.db.Exec(
		"UPDATE runs SET status = ?, updated_at = datetime('now') WHERE id = ?",
		status, runID,
	)
	return err
}

// StepEvents returns all events for a run (audit trail)
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

// HasRecentRun checks if a successful run for bookName exists within the window
// (for dedup: if an alert comes in within the dedup window — skip)
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

// CountRecentRuns counts the number of runs for bookName within the window
// (for throttle: max N runs per hour)
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

// RunningCount counts the number of runs for bookName in running status
// (for concurrency: max 1 simultaneous run)
func (j *Journal) RunningCount(bookName string) (int, error) {
	var count int
	err := j.db.QueryRow(
		"SELECT COUNT(*) FROM runs WHERE book_name = ? AND status = 'running'",
		bookName,
	).Scan(&count)
	return count, err
}

// MarshalData packs arbitrary data into JSON for the journal
func MarshalData(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
