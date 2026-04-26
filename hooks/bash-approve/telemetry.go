package main

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

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

var (
	telemetryHomeDir    = os.UserHomeDir
	telemetryExecutable = os.Executable
	copyFile            = copyFileStdlib
	renameFile          = os.Rename
	deleteLegacyFiles   = deleteLegacyTelemetryFiles
)

func copyFileStdlib(src, dst string) (err error) {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return os.Chmod(dst, info.Mode().Perm())
}

func cleanupFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

type copiedTelemetryFile struct {
	dst         string
	backup      string
	temp        string
	hadExisting bool
}

func rollbackCopiedTelemetryFiles(files []copiedTelemetryFile) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		cleanupFiles([]string{file.temp})
		if file.backup == "" {
			if !file.hadExisting {
				cleanupFiles([]string{file.dst})
			}
			continue
		}
		cleanupFiles([]string{file.dst})
		_ = renameFile(file.backup, file.dst)
	}
}

func finalizeCopiedTelemetryFiles(files []copiedTelemetryFile) {
	for _, file := range files {
		cleanupFiles([]string{file.backup, file.temp})
	}
}

func temporaryTelemetryFilePath(path, pattern string) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+pattern)
	if err != nil {
		return "", err
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		cleanupFiles([]string{name})
		return "", err
	}
	cleanupFiles([]string{name})
	return name, nil
}

func copyLegacyTelemetryFiles(legacyPath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	copied := make([]copiedTelemetryFile, 0, 4)
	for _, src := range sqliteFilesFor(legacyPath) {
		dst := destPath + strings.TrimPrefix(src, legacyPath)
		tempPath, err := temporaryTelemetryFilePath(dst, ".tmp-*")
		if err != nil {
			rollbackCopiedTelemetryFiles(copied)
			return err
		}

		entry := copiedTelemetryFile{dst: dst, temp: tempPath}
		if _, err := os.Stat(dst); err == nil {
			entry.hadExisting = true
		} else if !errors.Is(err, os.ErrNotExist) {
			rollbackCopiedTelemetryFiles(append(copied, entry))
			return err
		}

		if err := copyFile(src, tempPath); err != nil {
			rollbackCopiedTelemetryFiles(append(copied, entry))
			return err
		}

		if entry.hadExisting {
			backupPath, backupErr := temporaryTelemetryFilePath(dst, ".bak-*")
			if backupErr != nil {
				rollbackCopiedTelemetryFiles(append(copied, entry))
				return backupErr
			}
			if err := renameFile(dst, backupPath); err != nil {
				cleanupFiles([]string{backupPath})
				rollbackCopiedTelemetryFiles(append(copied, entry))
				return err
			}
			entry.backup = backupPath
		} else if _, err := os.Stat(dst); err == nil {
			cleanupFiles([]string{tempPath})
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			rollbackCopiedTelemetryFiles(append(copied, entry))
			return err
		}

		copied = append(copied, entry)
		if err := renameFile(tempPath, dst); err != nil {
			if errors.Is(err, os.ErrExist) {
				cleanupFiles([]string{tempPath, copied[len(copied)-1].backup})
				copied[len(copied)-1].temp = ""
				copied[len(copied)-1].backup = ""
				return err
			}
			rollbackCopiedTelemetryFiles(copied)
			return err
		}
		copied[len(copied)-1].temp = ""
	}

	finalizeCopiedTelemetryFiles(copied)
	return nil
}

func deleteLegacyTelemetryFiles(legacyPath string) error {
	for _, path := range sqliteFilesFor(legacyPath) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func resetTelemetryTestHooks() {
	telemetryHomeDir = os.UserHomeDir
	telemetryExecutable = os.Executable
	copyFile = copyFileStdlib
	renameFile = os.Rename
	deleteLegacyFiles = deleteLegacyTelemetryFiles
}

func telemetryArtifactPaths(base string) []string {
	return []string{base, base + "-wal", base + "-shm", base + "-journal"}
}

func cleanupTelemetryArtifacts(base string) {
	cleanupFiles(telemetryArtifactPaths(base))
}

func initTelemetrySchema(db *sql.DB) bool {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id       INTEGER PRIMARY KEY,
		ts       TEXT DEFAULT (datetime('now')),
		agent    TEXT DEFAULT 'claude',
		payload  TEXT,
		command  TEXT,
		decision TEXT,
		reason   TEXT
	)`)
	if err != nil {
		return false
	}
	if !ensureTelemetryColumn(db, "agent", "TEXT DEFAULT 'claude'") {
		return false
	}
	_, err = db.Exec(`UPDATE decisions SET agent = 'claude' WHERE agent IS NULL OR agent = ''`)
	return err == nil
}

func ensureTelemetryColumn(db *sql.DB, name, definition string) bool {
	rows, err := db.Query(`PRAGMA table_info(decisions)`)
	if err != nil {
		return false
	}
	defer rows.Close() //nolint:errcheck // best-effort telemetry validation

	for rows.Next() {
		var cid int
		var columnName string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false
		}
		if columnName == name {
			return rows.Err() == nil
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}

	_, err = db.Exec(`ALTER TABLE decisions ADD COLUMN ` + name + ` ` + definition)
	return err == nil
}

func validateTelemetryDBPath(path string) bool {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return false
	}
	defer db.Close() //nolint:errcheck // best-effort telemetry validation

	return initTelemetrySchema(db)
}

func openAndValidateTelemetryDB(path string) (*sql.DB, bool) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, false
	}
	if !initTelemetrySchema(db) {
		db.Close() //nolint:errcheck // best-effort telemetry
		return nil, false
	}
	return db, true
}

func migrateLegacyTelemetryDB(legacyPath, destPath string) (string, bool) {
	sourceFiles := sqliteFilesFor(legacyPath)
	if len(sourceFiles) == 0 {
		cleanupTelemetryArtifacts(destPath)
		return "", false
	}

	if err := copyLegacyTelemetryFiles(legacyPath, destPath); err != nil {
		if errors.Is(err, os.ErrExist) && validateTelemetryDBPath(destPath) {
			return destPath, true
		}
		cleanupTelemetryArtifacts(destPath)
		return "", false
	}

	for _, src := range sourceFiles {
		dst := destPath + strings.TrimPrefix(src, legacyPath)
		if _, err := os.Stat(dst); err != nil {
			cleanupTelemetryArtifacts(destPath)
			return "", false
		}
	}
	if !validateTelemetryDBPath(destPath) {
		cleanupTelemetryArtifacts(destPath)
		return "", false
	}

	_ = deleteLegacyFiles(legacyPath)
	return destPath, true
}

func ensureTelemetryDBReady(destPath string) (string, bool) {
	if _, err := os.Stat(destPath); err == nil {
		if validateTelemetryDBPath(destPath) {
			return destPath, true
		}

		legacyPath, ok := legacyTelemetryDBPath(telemetryExecutable)
		if !ok {
			return "", false
		}
		if _, err := os.Stat(legacyPath); err != nil {
			return "", false
		}

		cleanupTelemetryArtifacts(destPath)
		return migrateLegacyTelemetryDB(legacyPath, destPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		cleanupTelemetryArtifacts(destPath)
		return "", false
	}

	legacyPath, ok := legacyTelemetryDBPath(telemetryExecutable)
	if !ok {
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			cleanupTelemetryArtifacts(destPath)
			return "", false
		}
		return destPath, true
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", false
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			cleanupTelemetryArtifacts(destPath)
			return "", false
		}
		return destPath, true
	}

	return migrateLegacyTelemetryDB(legacyPath, destPath)
}

// openTelemetryDB opens telemetry.db from XDG state, migrating any legacy copy as needed.
// Returns nil if anything goes wrong — telemetry must never break the hook.
func openTelemetryDB() *sql.DB {
	dbPath, ok := telemetryDBPath(telemetryHomeDir)
	if !ok {
		return nil
	}
	readyPath, ok := ensureTelemetryDBReady(dbPath)
	if !ok {
		return nil
	}
	db, ok := openAndValidateTelemetryDB(readyPath)
	if !ok {
		return nil
	}
	return db
}

// logDecision inserts a telemetry row. Silently ignores all errors.
func logDecision(db *sql.DB, agent, payload, command, decision, reason string) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO decisions (agent, payload, command, decision, reason) VALUES (?, ?, ?, ?, ?)`,
		agent, payload, command, decision, reason)
}
