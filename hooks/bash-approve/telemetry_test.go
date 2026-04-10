package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
