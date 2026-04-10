package main

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func telemetryDBPath(homeDir func() (string, error)) (string, bool) {
	if xdgStateHome := os.Getenv("XDG_STATE_HOME"); xdgStateHome != "" && filepath.IsAbs(xdgStateHome) {
		return filepath.Join(xdgStateHome, "claude-bash-approve", "telemetry.db"), true
	}

	home, err := homeDir()
	if err != nil || home == "" {
		return "", false
	}

	return filepath.Join(home, ".local", "state", "claude-bash-approve", "telemetry.db"), true
}

func legacyTelemetryDBPath(executable func() (string, error)) (string, bool) {
	exe, err := executable()
	if err != nil || exe == "" {
		return "", false
	}

	return filepath.Join(filepath.Dir(exe), "telemetry.db"), true
}

func sqliteFilesFor(base string) []string {
	candidates := []string{base, base + "-wal", base + "-shm", base + "-journal"}
	files := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}
	return files
}

// openTelemetryDB opens (or creates) telemetry.db next to the running executable.
// Returns nil if anything goes wrong — telemetry must never break the hook.
func openTelemetryDB() *sql.DB {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dbPath := filepath.Join(filepath.Dir(exe), "telemetry.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id       INTEGER PRIMARY KEY,
		ts       TEXT DEFAULT (datetime('now')),
		payload  TEXT,
		command  TEXT,
		decision TEXT,
		reason   TEXT
	)`)
	if err != nil {
		db.Close() //nolint:errcheck // best-effort telemetry
		return nil
	}
	return db
}

// logDecision inserts a telemetry row. Silently ignores all errors.
func logDecision(db *sql.DB, payload, command, decision, reason string) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO decisions (payload, command, decision, reason) VALUES (?, ?, ?, ?)`,
		payload, command, decision, reason)
}
