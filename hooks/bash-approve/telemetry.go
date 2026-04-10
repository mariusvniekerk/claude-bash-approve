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

var copyFile = copyFileStdlib

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
		_ = os.Rename(file.backup, file.dst)
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
			entry.backup = backupPath
			if err := os.Rename(dst, backupPath); err != nil {
				rollbackCopiedTelemetryFiles(append(copied, entry))
				return err
			}
		}

		copied = append(copied, entry)
		if err := os.Rename(tempPath, dst); err != nil {
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
