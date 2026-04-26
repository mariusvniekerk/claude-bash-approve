package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

func closeTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}

func assertTelemetryAgentForCommand(t *testing.T, stateRoot, command, want string) {
	t.Helper()
	path := filepath.Join(stateRoot, "claude-bash-approve", "telemetry.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, db) }()

	var agent string
	if err := db.QueryRow(`SELECT agent FROM decisions WHERE command = ? ORDER BY id DESC LIMIT 1`, command).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, want, agent)
}

func TestTelemetryDBPathUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")

	path, ok := telemetryDBPath(func() (string, error) { return "/tmp/home", nil })
	if !ok {
		t.Fatal("expected telemetry path")
	}

	want := filepath.Join("/tmp/xdg-state", "claude-bash-approve", "telemetry.db")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestTelemetryDBPathFallsBackToLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	path, ok := telemetryDBPath(func() (string, error) { return "/tmp/home", nil })
	if !ok {
		t.Fatal("expected fallback telemetry path")
	}

	want := filepath.Join("/tmp/home", ".local", "state", "claude-bash-approve", "telemetry.db")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestTelemetryDBPathIgnoresRelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative")

	path, ok := telemetryDBPath(func() (string, error) { return "/tmp/home", nil })
	if !ok {
		t.Fatal("expected fallback telemetry path")
	}

	want := filepath.Join("/tmp/home", ".local", "state", "claude-bash-approve", "telemetry.db")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestTelemetryDBPathReturnsFalseWhenHomeUnavailable(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	_, ok := telemetryDBPath(func() (string, error) { return "", errors.New("boom") })
	if ok {
		t.Fatal("expected missing telemetry path")
	}
}

func TestLegacyTelemetryDBPathUsesExecutableDir(t *testing.T) {
	path, ok := legacyTelemetryDBPath(func() (string, error) { return "/tmp/bin/approve-bash", nil })
	if !ok {
		t.Fatal("expected legacy path")
	}
	want := filepath.Join("/tmp/bin", "telemetry.db")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestLegacyTelemetryDBPathReturnsFalseWhenExecutableUnavailable(t *testing.T) {
	_, ok := legacyTelemetryDBPath(func() (string, error) { return "", errors.New("boom") })
	if ok {
		t.Fatal("expected no legacy path")
	}
}

func TestInitTelemetrySchemaBackfillsAgentOnExistingDecisionRows(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, db) }()
	if _, err := db.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO decisions(command) VALUES ('git status')`); err != nil {
		t.Fatal(err)
	}

	if !initTelemetrySchema(db) {
		t.Fatal("expected schema initialization")
	}

	var agent string
	if err := db.QueryRow(`SELECT agent FROM decisions WHERE command = 'git status'`).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "claude", agent)
}

func TestLogDecisionPersistsAgent(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, db) }()
	if !initTelemetrySchema(db) {
		t.Fatal("expected schema initialization")
	}

	logDecision(db, "pi", `{"tool":"bash"}`, "git status", decisionAllow, "git read op")

	var agent string
	if err := db.QueryRow(`SELECT agent FROM decisions WHERE command = 'git status'`).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "pi", agent)
}

func TestSQLiteSidecarsIncludesPresentFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "telemetry.db")
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.WriteFile(base+suffix, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := sqliteFilesFor(base)
	want := []string{base, base + "-wal", base + "-shm", base + "-journal"}
	assert.Equal(t, want, got)
}

func TestCopyLegacyTelemetryFilesCopiesDatabaseAndSidecars(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.WriteFile(legacy+suffix, []byte("legacy"+suffix), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyLegacyTelemetryFiles(legacy, dest); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		got, err := os.ReadFile(dest + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "legacy"+suffix {
			t.Fatalf("dest%s = %q", suffix, string(got))
		}
	}
}

func TestCopyLegacyTelemetryFilesPreservesSourcePermissions(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}

	oldUmask := syscall.Umask(0o022)
	defer syscall.Umask(oldUmask)

	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyLegacyTelemetryFiles(legacy, dest); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("dest mode = %#o, want %#o", info.Mode().Perm(), 0o600)
	}
}

func TestDeleteLegacyTelemetryFilesRemovesLegacyFilesAfterSuccess(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.WriteFile(legacy+suffix, []byte("legacy"+suffix), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := deleteLegacyTelemetryFiles(legacy); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Stat(legacy + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy%s still exists, err = %v", suffix, err)
		}
	}
}

func TestCopyLegacyTelemetryFilesCleansPartialDestinationOnFailure(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.WriteFile(legacy+suffix, []byte("legacy"+suffix), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	calls := 0
	copyFile = func(src, dst string) error {
		calls++
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if calls < 3 {
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o600)
		}
		if err := os.WriteFile(dst, []byte("partial"), 0o600); err != nil {
			return err
		}
		return errors.New("boom")
	}
	defer func() { copyFile = copyFileStdlib }()

	if err := copyLegacyTelemetryFiles(legacy, dest); err == nil {
		t.Fatal("expected copy error")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(dest + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dest%s still exists after rollback, err = %v", suffix, err)
		}
	}
}

func TestCopyLegacyTelemetryFilesRestoresExistingDestinationOnFailure(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal"} {
		if err := os.WriteFile(legacy+suffix, []byte("legacy"+suffix), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest+suffix, []byte("existing"+suffix), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	calls := 0
	copyFile = func(src, dst string) error {
		calls++
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if calls == 1 {
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o600)
		}
		if err := os.WriteFile(dst, []byte("partial"), 0o600); err != nil {
			return err
		}
		return errors.New("boom")
	}
	defer func() { copyFile = copyFileStdlib }()

	if err := copyLegacyTelemetryFiles(legacy, dest); err == nil {
		t.Fatal("expected copy error")
	}
	for _, suffix := range []string{"", "-wal"} {
		got, err := os.ReadFile(dest + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "existing"+suffix {
			t.Fatalf("dest%s = %q, want existing contents restored", suffix, string(got))
		}
	}
}

func TestCopyLegacyTelemetryFilesPreservesExistingDestinationWhenBackupRenameFails(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	renameFile = func(oldPath, newPath string) error {
		if strings.Contains(filepath.Base(newPath), ".bak-") {
			return errors.New("boom")
		}
		return os.Rename(oldPath, newPath)
	}
	defer func() { renameFile = os.Rename }()

	if err := copyLegacyTelemetryFiles(legacy, dest); err == nil {
		t.Fatal("expected copy error")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing" {
		t.Fatalf("dest = %q, want existing contents preserved", string(got))
	}
}

func TestCopyLegacyTelemetryFilesKeepsWinnerWhenFinalRenameHitsErrExistAfterBackup(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	renameFile = func(oldPath, newPath string) error {
		if newPath == dest && strings.Contains(filepath.Base(oldPath), filepath.Base(dest)+".tmp-") {
			if err := os.WriteFile(dest, []byte("winner"), 0o600); err != nil {
				return err
			}
			return os.ErrExist
		}
		return os.Rename(oldPath, newPath)
	}
	defer func() { renameFile = os.Rename }()

	if err := copyLegacyTelemetryFiles(legacy, dest); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "winner" {
		t.Fatalf("dest = %q, want winner preserved", string(got))
	}
	backups, err := filepath.Glob(dest + ".bak-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("unexpected backup files remain: %v", backups)
	}
}

func TestOpenTelemetryDBUsesExistingUsableDestinationEvenWhenLegacyExists(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	legacyRoot := filepath.Join(root, "legacy")
	destPath := filepath.Join(stateRoot, "claude-bash-approve", "telemetry.db")
	legacyPath := filepath.Join(legacyRoot, "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", destPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, db) }()
	if _, err := db.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO decisions(command) VALUES ('existing')`); err != nil {
		t.Fatal(err)
	}
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, legacyDB) }()
	if _, err := legacyDB.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decisions(command) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer func() { closeTestDB(t, opened) }()

	var command string
	if err := opened.QueryRow(`SELECT command FROM decisions LIMIT 1`).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "existing" {
		t.Fatalf("command = %q, want existing", command)
	}
}

func TestOpenTelemetryDBMigratesLegacyDatabaseBeforeOpening(t *testing.T) {
	root := t.TempDir()
	legacyRoot := filepath.Join(root, "legacy")
	stateRoot := filepath.Join(root, "state")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyRoot, "telemetry.db")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, legacyDB) }()
	if _, err := legacyDB.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decisions(command) VALUES ('git status')`); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	t.Setenv("XDG_STATE_HOME", stateRoot)
	defer resetTelemetryTestHooks()

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer func() { closeTestDB(t, opened) }()

	var count int
	if err := opened.QueryRow(`SELECT count(*) FROM decisions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestOpenTelemetryDBRetriesMigrationWhenDestinationInvalidAndLegacyExists(t *testing.T) {
	root := t.TempDir()
	legacyRoot := filepath.Join(root, "legacy")
	stateRoot := filepath.Join(root, "state")
	destDir := filepath.Join(stateRoot, "claude-bash-approve")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "telemetry.db"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyRoot, "telemetry.db")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, legacyDB) }()
	if _, err := legacyDB.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decisions(command) VALUES ('recovered')`); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer func() { closeTestDB(t, opened) }()

	var command string
	if err := opened.QueryRow(`SELECT command FROM decisions LIMIT 1`).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "recovered" {
		t.Fatalf("command = %q, want recovered", command)
	}
}

func TestOpenTelemetryDBCreatesFreshDatabaseWhenLegacyPathUnavailable(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return "", errors.New("no exe") }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected fresh db")
	}
	defer func() { closeTestDB(t, opened) }()
}

func TestOpenTelemetryDBCreatesFreshDatabaseWhenLegacyPathResolvesButFileIsMissing(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	legacyRoot := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected fresh db")
	}
	defer func() { closeTestDB(t, opened) }()
}

func TestOpenTelemetryDBDisablesTelemetryWhenLegacyPathStatFails(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	legacyRoot := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacyRoot, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(legacyRoot, 0o755) //nolint:errcheck // restore tempdir access for cleanup
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	if db := openTelemetryDB(); db != nil {
		defer func() { closeTestDB(t, db) }()
		t.Fatal("expected nil db")
	}
}

func TestOpenTelemetryDBDisablesTelemetryWhenDestinationExistsButIsInvalidAndLegacyMissing(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	destDir := filepath.Join(stateRoot, "claude-bash-approve")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "telemetry.db"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return "", errors.New("no exe") }
	t.Setenv("XDG_STATE_HOME", stateRoot)
	defer resetTelemetryTestHooks()

	if db := openTelemetryDB(); db != nil {
		t.Fatal("expected nil db")
	}
}

func TestOpenTelemetryDBDisablesTelemetryWhenDestinationInvalidAndLegacyPathResolvesButFileMissing(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	legacyRoot := filepath.Join(root, "legacy")
	destDir := filepath.Join(stateRoot, "claude-bash-approve")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "telemetry.db"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	t.Setenv("XDG_STATE_HOME", stateRoot)
	defer resetTelemetryTestHooks()

	if db := openTelemetryDB(); db != nil {
		t.Fatal("expected nil db")
	}
}

func TestOpenTelemetryDBTreatsDestinationCreatedDuringMigrationAsSuccess(t *testing.T) {
	root := t.TempDir()
	legacyRoot := filepath.Join(root, "legacy")
	stateRoot := filepath.Join(root, "state")
	legacyPath := filepath.Join(legacyRoot, "telemetry.db")
	destPath := filepath.Join(stateRoot, "claude-bash-approve", "telemetry.db")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, legacyDB) }()
	if _, err := legacyDB.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decisions(command) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	renameFile = func(oldPath, newPath string) error {
		if newPath == destPath && strings.Contains(filepath.Base(oldPath), filepath.Base(destPath)+".tmp-") {
			winner, err := sql.Open("sqlite", destPath)
			if err != nil {
				return err
			}
			defer func() { closeTestDB(t, winner) }()
			if _, err := winner.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
				return err
			}
			if _, err := winner.Exec(`INSERT INTO decisions(command) VALUES ('won-race')`); err != nil {
				return err
			}
			return os.ErrExist
		}
		return os.Rename(oldPath, newPath)
	}
	defer func() { renameFile = os.Rename }()

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer func() { closeTestDB(t, opened) }()

	var command string
	if err := opened.QueryRow(`SELECT command FROM decisions LIMIT 1`).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "won-race" {
		t.Fatalf("command = %q, want won-race", command)
	}
}

func TestOpenTelemetryDBContinuesWhenLegacyDeleteFailsAfterSuccessfulMigration(t *testing.T) {
	root := t.TempDir()
	legacyRoot := filepath.Join(root, "legacy")
	stateRoot := filepath.Join(root, "state")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyRoot, "telemetry.db")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeTestDB(t, legacyDB) }()
	if _, err := legacyDB.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decisions(command) VALUES ('keep-new')`); err != nil {
		t.Fatal(err)
	}
	telemetryHomeDir = func() (string, error) { return root, nil }
	telemetryExecutable = func() (string, error) { return filepath.Join(legacyRoot, "approve-bash"), nil }
	deleteLegacyFiles = func(string) error { return errors.New("cannot delete") }
	defer resetTelemetryTestHooks()
	t.Setenv("XDG_STATE_HOME", stateRoot)

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer func() { closeTestDB(t, opened) }()

	var command string
	if err := opened.QueryRow(`SELECT command FROM decisions LIMIT 1`).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "keep-new" {
		t.Fatalf("command = %q, want keep-new", command)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("expected legacy db to remain after delete failure, err = %v", err)
	}
}
