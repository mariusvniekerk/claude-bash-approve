# Telemetry Storage Migration Implementation Plan

> **For agentic workers:** REQUIRED: Use `/skill:orchestrator-implements` (in-session, orchestrator implements), `/skill:subagent-driven-development` (in-session, subagents implement), or `/skill:executing-plans` (parallel session) to implement this plan. Steps use checkbox syntax for tracking.

**Goal:** Move telemetry storage to an XDG-style user state directory, migrate the legacy executable-adjacent SQLite database once, and document the new behavior.

**Architecture:** Extract telemetry path resolution and migration into small helpers in `hooks/bash-approve/telemetry.go`, with focused tests in a dedicated telemetry test file. `openTelemetryDB()` becomes a coordinator that resolves the new path, conditionally migrates the legacy DB, opens the destination database, initializes schema, and still degrades safely on any failure.

**Tech Stack:** Go, standard library filesystem/path/env APIs, `database/sql`, `modernc.org/sqlite`

---

### Task 1: Add failing tests for XDG path resolution

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/telemetry.go`
- Create: `hooks/bash-approve/telemetry_test.go`
- Test: `hooks/bash-approve/telemetry_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd hooks/bash-approve && go test ./... -run 'TestTelemetryDBPath'`
Expected: FAIL because `telemetryDBPath` does not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd hooks/bash-approve && go test ./... -run 'TestTelemetryDBPath'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go
git commit -m "test: cover telemetry db path resolution"
```

### Task 2: Add failing tests for legacy path resolution and migration helpers

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/telemetry.go`
- Modify: `hooks/bash-approve/telemetry_test.go`
- Test: `hooks/bash-approve/telemetry_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd hooks/bash-approve && go test ./... -run 'TestLegacyTelemetryDBPath|TestSQLiteSidecars'`
Expected: FAIL because helper functions do not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd hooks/bash-approve && go test ./... -run 'TestLegacyTelemetryDBPath|TestSQLiteSidecars'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go
git commit -m "test: add telemetry migration helper coverage"
```

### Task 3: Add failing tests for one-time migration behavior

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/telemetry.go`
- Modify: `hooks/bash-approve/telemetry_test.go`
- Test: `hooks/bash-approve/telemetry_test.go`

- [ ] **Step 1: Write the failing test**

```go
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

func TestDeleteLegacyTelemetryFilesRemovesLegacyFilesAfterSuccess(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := deleteLegacyTelemetryFiles(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy still exists, err = %v", err)
	}
}

func TestCopyLegacyTelemetryFilesCleansPartialDestinationOnFailure(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "telemetry.db")
	dest := filepath.Join(root, "state", "telemetry.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	copyFile = func(src, dst string) error {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
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
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dest still exists, err = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd hooks/bash-approve && go test ./... -run 'Test(CopyLegacyTelemetryFiles|DeleteLegacyTelemetryFiles)'`
Expected: FAIL because `copyLegacyTelemetryFiles`, `deleteLegacyTelemetryFiles`, and file-copy indirection do not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
var copyFile = copyFileStdlib

func copyFileStdlib(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func cleanupFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func copyLegacyTelemetryFiles(legacyPath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	copied := make([]string, 0, 4)
	for _, src := range sqliteFilesFor(legacyPath) {
		dst := destPath + strings.TrimPrefix(src, legacyPath)
		if err := copyFile(src, dst); err != nil {
			cleanupFiles(append(copied, dst))
			return err
		}
		copied = append(copied, dst)
	}
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd hooks/bash-approve && go test ./... -run 'Test(CopyLegacyTelemetryFiles|DeleteLegacyTelemetryFiles)'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go
git commit -m "feat: add telemetry db migration helpers"
```

### Task 4: Add failing tests for openTelemetryDB coordination behavior

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/telemetry.go`
- Modify: `hooks/bash-approve/telemetry_test.go`
- Test: `hooks/bash-approve/telemetry_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
	defer db.Close()
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
	defer legacyDB.Close()
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
	defer opened.Close()

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
	defer legacyDB.Close()
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
	defer opened.Close()

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
	defer legacyDB.Close()
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
	defer opened.Close()

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
	defer opened.Close()
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
	defer opened.Close()
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
	defer legacyDB.Close()
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
	copyFile = func(src, dst string) error {
		if dst == destPath {
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			winner, err := sql.Open("sqlite", destPath)
			if err != nil {
				return err
			}
			defer winner.Close()
			if _, err := winner.Exec(`CREATE TABLE decisions (id INTEGER PRIMARY KEY, command TEXT)`); err != nil {
				return err
			}
			if _, err := winner.Exec(`INSERT INTO decisions(command) VALUES ('won-race')`); err != nil {
				return err
			}
			return os.ErrExist
		}
		return copyFileStdlib(src, dst)
	}
	defer func() { copyFile = copyFileStdlib }()

	opened := openTelemetryDB()
	if opened == nil {
		t.Fatal("expected db")
	}
	defer opened.Close()

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
	defer legacyDB.Close()
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
	defer opened.Close()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd hooks/bash-approve && go test ./... -run 'TestOpenTelemetryDB'`
Expected: FAIL because `openTelemetryDB()` still uses the executable-adjacent path and has no path-resolution or migration coordination

- [ ] **Step 3: Write minimal implementation**

```go
var (
	telemetryHomeDir    = os.UserHomeDir
	telemetryExecutable = os.Executable
)

func resetTelemetryTestHooks() {
	telemetryHomeDir = os.UserHomeDir
	telemetryExecutable = os.Executable
	copyFile = copyFileStdlib
	deleteLegacyFiles = deleteLegacyTelemetryFiles
}

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
```

Add concrete helpers with this behavior:

```go
var deleteLegacyFiles = deleteLegacyTelemetryFiles
```
- package-level indirection used by tests that need delete failure injection

```go
func ensureTelemetryDBReady(destPath string) (string, bool)
```
- if `destPath` already exists and `validateTelemetryDBPath(destPath)` succeeds, return `destPath, true`
- if `destPath` exists but is invalid, resolve the legacy path; when legacy exists, remove invalid destination artifacts and retry migration; when legacy is absent, return `"", false`
- if `destPath` does not exist, resolve the legacy path; when legacy is missing or unavailable, create `filepath.Dir(destPath)` and return `destPath, true` so a fresh DB can be created; when legacy exists, copy files, confirm that every source sidecar present at copy time also exists at destination, validate destination, then call `deleteLegacyFiles(legacyPath)` best-effort and return `destPath, true`
- if copying returns `os.ErrExist` and `validateTelemetryDBPath(destPath)` succeeds, treat that as another process winning the race and return `destPath, true`
- on any setup or migration failure, remove partial destination artifacts and return `"", false`

```go
func validateTelemetryDBPath(path string) bool
```
- open the SQLite database at `path`
- run schema initialization to prove the DB is usable
- close the probe DB before returning
- return `true` on success and `false` on error

```go
func openAndValidateTelemetryDB(path string) (*sql.DB, bool)
```
- open the SQLite database at `path`
- run schema initialization
- on failure, close the DB and return `nil, false`
- this is the runtime open path used only after `ensureTelemetryDBReady`

```go
func initTelemetrySchema(db *sql.DB) bool
```
- run the existing `CREATE TABLE IF NOT EXISTS decisions (...)` statement
- return `true` on success and `false` on error
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd hooks/bash-approve && go test ./... -run 'TestOpenTelemetryDB'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go
git commit -m "feat: migrate telemetry db to xdg state"
```

### Task 5: Update docs and skill references for the new telemetry location

**TDD scenario:** Trivial change — use judgment

**Files:**
- Modify: `README.md`
- Modify: `.claude/skills/bash-approve-telemetry/SKILL.md`

- [ ] **Step 1: Update the README telemetry section**

```md
## Telemetry

Every decision is logged to a local SQLite database in the user state directory.

- If `XDG_STATE_HOME` is set to an absolute path, the DB lives at `$XDG_STATE_HOME/claude-bash-approve/telemetry.db`
- Otherwise, it defaults to `~/.local/state/claude-bash-approve/telemetry.db`
- On first run after upgrade, an existing legacy DB at `~/.claude/hooks/bash-approve/telemetry.db` is migrated automatically when possible

Example:

```bash
state_home="${XDG_STATE_HOME}"
if [ -z "$state_home" ] || [ "${state_home#/}" = "$state_home" ]; then
  state_home="$HOME/.local/state"
fi
sqlite3 "$state_home/claude-bash-approve/telemetry.db" \
  "SELECT ts, decision, command, reason FROM decisions ORDER BY ts DESC LIMIT 20"
```
```

- [ ] **Step 2: Update the skill documentation**

```md
## Database Location

Primary location:
- `$XDG_STATE_HOME/claude-bash-approve/telemetry.db` when `XDG_STATE_HOME` is set to an absolute path
- otherwise `~/.local/state/claude-bash-approve/telemetry.db`

Legacy compatibility:
- older installs may be migrated from the common legacy path `~/.claude/hooks/bash-approve/telemetry.db`
- if cleanup of the legacy files fails after migration, the new path is still authoritative

Behavior notes:
- remove references saying the DB is created next to the compiled binary
- remove references saying the path is resolved via `os.Executable()`
- update the `Common Mistakes` section so it no longer claims the database lives next to the binary
```

Also update the quick-reference query examples to use the new path expression.

- [ ] **Step 3: Run relevant tests**

Run: `cd hooks/bash-approve && go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add README.md .claude/skills/bash-approve-telemetry/SKILL.md
git commit -m "docs: update telemetry database location"
```

### Task 6: Run full verification and finalize

**TDD scenario:** Modifying tested code — run existing tests first

**Files:**
- Modify: `hooks/bash-approve/telemetry.go`
- Modify: `hooks/bash-approve/telemetry_test.go`
- Modify: `README.md`
- Modify: `.claude/skills/bash-approve-telemetry/SKILL.md`

- [ ] **Step 1: Run the full test suite**

Run: `cd hooks/bash-approve && go test ./...`
Expected: PASS

- [ ] **Step 2: Run lint**

Run: `cd hooks/bash-approve && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 3: Review the final diff**

Run: `git diff -- hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go README.md .claude/skills/bash-approve-telemetry/SKILL.md`
Expected: only telemetry path/migration logic, tests, and docs changes

- [ ] **Step 4: Commit final polish if needed**

```bash
git add hooks/bash-approve/telemetry.go hooks/bash-approve/telemetry_test.go README.md .claude/skills/bash-approve-telemetry/SKILL.md
git commit -m "chore: finalize telemetry storage migration"
```

If there is nothing left to commit after verification, skip this step.
