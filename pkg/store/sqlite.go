package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite implements persistence for run metadata, attempted paths, and hits.
type SQLite struct {
	db *sql.DB
}

// Run represents a persisted execution within the database.
type Run struct {
	db *sql.DB
	id int64
}

// RunMetadata captures contextual information for a fuzzing execution.
type RunMetadata struct {
	TargetURL   string
	Wordlist    string
	Concurrency int
	Timeout     time.Duration
	Profile     string
	Beginner    bool
	BinaryName  string
	StartedAt   time.Time
}

// HitRecord stores information about a detected hit.
type HitRecord struct {
	Path          string
	StatusCode    int
	ContentLength int64
	Duration      time.Duration
}

// OpenSQLite initializes (or connects to) the SQLite database located at the given path.
func OpenSQLite(path string) (*SQLite, error) {
	if path == "" {
		return nil, errors.New("sqlite path must not be empty")
	}

	if err := ensureDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", pragma, err)
		}
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLite{db: db}, nil
}

// Close releases any resources associated with the database connection.
func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

// StartRun records metadata for a new execution and returns a handle for recording activity.
func (s *SQLite) StartRun(ctx context.Context, meta RunMetadata) (*Run, error) {
	if s == nil {
		return nil, errors.New("sqlite store is nil")
	}

	startedAt := meta.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	timeoutMs := int64(meta.Timeout / time.Millisecond)
	beginner := 0
	if meta.Beginner {
		beginner = 1
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO runs (started_at, target_url, wordlist, concurrency, timeout_ms, profile, beginner, binary_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, startedAt.Format(time.RFC3339Nano), meta.TargetURL, meta.Wordlist, meta.Concurrency, timeoutMs, meta.Profile, beginner, meta.BinaryName)
	if err != nil {
		return nil, fmt.Errorf("insert run metadata: %w", err)
	}

	runID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("obtain run id: %w", err)
	}

	return &Run{db: s.db, id: runID}, nil
}

// ID returns the run identifier within the database.
func (r *Run) ID() int64 {
	if r == nil {
		return 0
	}
	return r.id
}

// MarkAttempt records that a path has been attempted. It returns true if the path is new.
func (r *Run) MarkAttempt(ctx context.Context, path string) (bool, error) {
	if r == nil {
		return false, errors.New("run is nil")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.db.ExecContext(ctx, `
INSERT OR IGNORE INTO path_attempted (path, run_id, attempted_at)
VALUES (?, ?, ?)
`, path, r.id, now)
	if err != nil {
		return false, fmt.Errorf("insert path attempt: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("path attempt rows affected: %w", err)
	}

	if rows == 0 {
		// Update the metadata to reflect the latest run even if the path already existed.
		if _, err := r.db.ExecContext(ctx, `
UPDATE path_attempted SET run_id = ?, attempted_at = ? WHERE path = ?
`, r.id, now, path); err != nil {
			return false, fmt.Errorf("update existing path attempt: %w", err)
		}
		return false, nil
	}

	return true, nil
}

// RecordHit saves information about a confirmed hit for the run.
func (r *Run) RecordHit(ctx context.Context, hit HitRecord) error {
	if r == nil {
		return errors.New("run is nil")
	}

	recordedAt := time.Now().UTC().Format(time.RFC3339Nano)
	durationMs := hit.Duration.Milliseconds()
	if hit.Duration < 0 {
		durationMs = 0
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO hits (run_id, path, status_code, content_length, duration_ms, recorded_at)
VALUES (?, ?, ?, ?, ?, ?)
`, r.id, hit.Path, hit.StatusCode, hit.ContentLength, durationMs, recordedAt)
	if err != nil {
		return fmt.Errorf("insert hit: %w", err)
	}

	return nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sqlite directory: %w", err)
	}
	return nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS runs (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        started_at TEXT NOT NULL,
                        target_url TEXT,
                        wordlist TEXT,
                        concurrency INTEGER,
                        timeout_ms INTEGER,
                        profile TEXT,
                        beginner INTEGER,
                        binary_name TEXT
                )`,
		`CREATE TABLE IF NOT EXISTS path_attempted (
                        path TEXT PRIMARY KEY,
                        run_id INTEGER NOT NULL,
                        attempted_at TEXT NOT NULL,
                        FOREIGN KEY(run_id) REFERENCES runs(id)
                )`,
		`CREATE TABLE IF NOT EXISTS hits (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        run_id INTEGER NOT NULL,
                        path TEXT NOT NULL,
                        status_code INTEGER,
                        content_length INTEGER,
                        duration_ms INTEGER,
                        recorded_at TEXT NOT NULL,
                        FOREIGN KEY(run_id) REFERENCES runs(id)
                )`,
		`CREATE INDEX IF NOT EXISTS idx_hits_run_id ON hits(run_id)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}

	return nil
}
