package store

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite implements persistence for run metadata, attempted paths, and hits.
type SQLite struct {
	db *sql.DB
}

// Run represents a persisted execution within the database.
type Run struct {
	db    *sql.DB
	id    int64
	runID string
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
	RunID       string
	ConfigList  []string
	PayloadList []string
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

	runIdentifier := strings.TrimSpace(meta.RunID)
	if runIdentifier == "" {
		runIdentifier = meta.hash()
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

	// Try updating an existing row first so repeated runs with the same identifier
	// refresh their metadata.
	res, err := s.db.ExecContext(ctx, `
UPDATE runs SET started_at = ?, target_url = ?, wordlist = ?, concurrency = ?, timeout_ms = ?, profile = ?, beginner = ?, binary_name = ?
WHERE run_id = ?
`, startedAt.Format(time.RFC3339Nano), meta.TargetURL, meta.Wordlist, meta.Concurrency, timeoutMs, meta.Profile, beginner, meta.BinaryName, runIdentifier)
	if err != nil {
		return nil, fmt.Errorf("update run metadata: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("run metadata rows affected: %w", err)
	}

	if rows == 0 {
		res, err = s.db.ExecContext(ctx, `
INSERT INTO runs (run_id, started_at, target_url, wordlist, concurrency, timeout_ms, profile, beginner, binary_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, runIdentifier, startedAt.Format(time.RFC3339Nano), meta.TargetURL, meta.Wordlist, meta.Concurrency, timeoutMs, meta.Profile, beginner, meta.BinaryName)
		if err != nil {
			return nil, fmt.Errorf("insert run metadata: %w", err)
		}

		runPK, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("obtain run id: %w", err)
		}

		return &Run{db: s.db, id: runPK, runID: runIdentifier}, nil
	}

	var runPK int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM runs WHERE run_id = ?`, runIdentifier).Scan(&runPK); err != nil {
		return nil, fmt.Errorf("lookup run id: %w", err)
	}

	return &Run{db: s.db, id: runPK, runID: runIdentifier}, nil
}

// ID returns the run identifier within the database.
func (r *Run) ID() int64 {
	if r == nil {
		return 0
	}
	return r.id
}

// RunID returns the stable identifier associated with the run.
func (r *Run) RunID() string {
	if r == nil {
		return ""
	}
	return r.runID
}

// Hash returns the deterministic identifier derived from the supplied metadata.
func (m RunMetadata) Hash() string {
	return m.hash()
}

func (m RunMetadata) hash() string {
	return hashFromLists(m.normalizedConfig(), m.normalizedPayloads())
}

// ConfigEntries returns a normalized copy of the configuration list used to
// derive the run hash.
func (m RunMetadata) ConfigEntries() []string {
	entries := m.normalizedConfig()
	return append([]string(nil), entries...)
}

// PayloadEntries returns a normalized copy of the payload list used to derive
// the run hash.
func (m RunMetadata) PayloadEntries() []string {
	entries := m.normalizedPayloads()
	return append([]string(nil), entries...)
}

func (m RunMetadata) normalizedConfig() []string {
	configList := normalizeList(m.ConfigList)
	if len(configList) == 0 {
		configList = normalizeList(m.defaultConfigList())
	}
	return configList
}

func (m RunMetadata) normalizedPayloads() []string {
	payloadList := normalizeList(m.PayloadList)
	if len(payloadList) == 0 {
		payloadList = normalizeList(m.defaultPayloadList())
	}
	return payloadList
}

func (m RunMetadata) defaultConfigList() []string {
	entries := []string{}

	if m.TargetURL != "" {
		entries = append(entries, "target_url="+m.TargetURL)
	}
	if m.Wordlist != "" {
		entries = append(entries, "wordlist="+m.Wordlist)
	}
	if m.Concurrency > 0 {
		entries = append(entries, fmt.Sprintf("concurrency=%d", m.Concurrency))
	}
	if m.Timeout > 0 {
		entries = append(entries, fmt.Sprintf("timeout=%s", m.Timeout))
	}
	if m.Profile != "" {
		entries = append(entries, "profile="+m.Profile)
	}
	if m.Beginner {
		entries = append(entries, "beginner=true")
	}
	if m.BinaryName != "" {
		entries = append(entries, "binary="+m.BinaryName)
	}

	return entries
}

func (m RunMetadata) defaultPayloadList() []string {
	if m.Wordlist == "" {
		return nil
	}
	return []string{m.Wordlist}
}

func normalizeList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		normalized = append(normalized, v)
	}
	return normalized
}

func hashFromLists(configList, payloadList []string) string {
	hasher := sha1.New()

	sort.Strings(configList)
	sort.Strings(payloadList)

	for _, entry := range configList {
		_, _ = hasher.Write([]byte(entry))
		_, _ = hasher.Write([]byte{'\n'})
	}

	_, _ = hasher.Write([]byte("--payloads--\n"))

	for _, entry := range payloadList {
		_, _ = hasher.Write([]byte(entry))
		_, _ = hasher.Write([]byte{'\n'})
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

func ensureRunIDColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(runs)`)
	if err != nil {
		return fmt.Errorf("inspect runs table: %w", err)
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)

		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}

		if strings.EqualFold(name, "run_id") {
			hasColumn = true
			break
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return fmt.Errorf("iterate table info: %w", rowsErr)
	}

	if hasColumn {
		return nil
	}

	if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN run_id TEXT`); err != nil {
		return fmt.Errorf("add run_id column: %w", err)
	}

	return nil
}

func backfillRunIDs(db *sql.DB) error {
	rows, err := db.Query(`
SELECT id, target_url, wordlist, concurrency, timeout_ms, profile, beginner, binary_name
FROM runs
WHERE run_id IS NULL OR run_id = ''
`)
	if err != nil {
		return fmt.Errorf("select runs missing id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          int64
			targetURL   sql.NullString
			wordlist    sql.NullString
			concurrency sql.NullInt64
			timeoutMs   sql.NullInt64
			profile     sql.NullString
			beginner    sql.NullInt64
			binary      sql.NullString
		)

		if err := rows.Scan(&id, &targetURL, &wordlist, &concurrency, &timeoutMs, &profile, &beginner, &binary); err != nil {
			return fmt.Errorf("scan run metadata: %w", err)
		}

		meta := RunMetadata{}
		if targetURL.Valid {
			meta.TargetURL = targetURL.String
		}
		if wordlist.Valid {
			meta.Wordlist = wordlist.String
		}
		if concurrency.Valid {
			meta.Concurrency = int(concurrency.Int64)
		}
		if timeoutMs.Valid {
			meta.Timeout = time.Duration(timeoutMs.Int64) * time.Millisecond
		}
		if profile.Valid {
			meta.Profile = profile.String
		}
		if beginner.Valid {
			meta.Beginner = beginner.Int64 != 0
		}
		if binary.Valid {
			meta.BinaryName = binary.String
		}

		generatedID := meta.hash()
		if _, err := db.Exec(`UPDATE runs SET run_id = ? WHERE id = ?`, generatedID, id); err != nil {
			return fmt.Errorf("backfill run_id for %d: %w", id, err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate runs missing id: %w", err)
	}

	return nil
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
                        run_id TEXT,
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
		`CREATE INDEX IF NOT EXISTS idx_runs_run_id ON runs(run_id)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}

	if err := ensureRunIDColumn(db); err != nil {
		return err
	}

	if err := backfillRunIDs(db); err != nil {
		return err
	}

	return nil
}
