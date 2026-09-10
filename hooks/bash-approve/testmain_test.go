package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain prevents Git fixture commands from inheriting a commit hook's repository and index.
func TestMain(m *testing.M) {
	scratchRoot, err := os.MkdirTemp("", "bash-approve-tests-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test scratch directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") {
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintf(os.Stderr, "clear %s: %v\n", name, err)
				_ = os.RemoveAll(scratchRoot)
				os.Exit(1)
			}
		}
	}

	globalConfig := filepath.Join(scratchRoot, "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "create isolated Git config: %v\n", err)
		_ = os.RemoveAll(scratchRoot)
		os.Exit(1)
	}
	if err := os.Setenv("GIT_CONFIG_GLOBAL", globalConfig); err != nil {
		fmt.Fprintf(os.Stderr, "set GIT_CONFIG_GLOBAL: %v\n", err)
		_ = os.RemoveAll(scratchRoot)
		os.Exit(1)
	}
	if err := os.Setenv("GIT_CONFIG_NOSYSTEM", "1"); err != nil {
		fmt.Fprintf(os.Stderr, "set GIT_CONFIG_NOSYSTEM: %v\n", err)
		_ = os.RemoveAll(scratchRoot)
		os.Exit(1)
	}
	if err := os.Setenv("TMPDIR", scratchRoot); err != nil {
		fmt.Fprintf(os.Stderr, "set TMPDIR: %v\n", err)
		_ = os.RemoveAll(scratchRoot)
		os.Exit(1)
	}

	exitCode := m.Run()
	if err := os.RemoveAll(scratchRoot); err != nil {
		fmt.Fprintf(os.Stderr, "remove test scratch directory: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
