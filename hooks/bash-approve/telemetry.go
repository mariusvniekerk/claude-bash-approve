package main

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

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
