package main

import (
	"errors"
	"path/filepath"
	"testing"
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
