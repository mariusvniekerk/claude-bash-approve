package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiMode_BashAllow(t *testing.T) {
	repo := initGitRepo(t)
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool:    "bash",
		Command: "git status",
		Cwd:     repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"bash","decision":"allow","reason":"git read op"}`, out)
}

func TestPiMode_ReadInRepoAllow(t *testing.T) {
	repo := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("hello\n"), 0o644))
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool: "read",
		Path: "notes.txt",
		Cwd:  repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"read","decision":"allow","reason":"read"}`, out)
}

func TestPiMode_GrepInRepoAllow(t *testing.T) {
	repo := initGitRepo(t)
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool:    "grep",
		Pattern: "TODO",
		Path:    ".",
		Cwd:     repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"grep","decision":"allow","reason":"grep"}`, out)
}

func TestPiMode_UnsupportedTool(t *testing.T) {
	repo := initGitRepo(t)
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool: "write",
		Cwd:  repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"unsupported-tool","message":"unsupported tool: write"}}`, out)
}

func TestPiMode_InvalidInput(t *testing.T) {
	out := parseAndEvaluatePiModeForTest(t, Config{Enabled: []string{"all"}}, []byte(`{"tool":`))
	assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"invalid-input","message":"invalid pi input"}}`, out)
}

func TestPiMode_CLIInvalidInputReturnsPiError(t *testing.T) {
	out := runApproveBashCLI(t, []string{"--pi"}, []byte(`{"tool":`))
	assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"invalid-input","message":"invalid pi input"}}`, out)
}

func TestPiMode_ConfigPathError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	out := runPiModeWithConfigPathForTest(t, missing, PiInput{Tool: "bash", Command: "git status", Cwd: t.TempDir()})
	assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"config-error","message":"cannot read config `+missing+`"}}`, out)
}

func TestPiMode_CLIParsesConfigFlagBeforeMode(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	payload, err := json.Marshal(PiInput{Tool: "bash", Command: "git status", Cwd: t.TempDir()})
	require.NoError(t, err)
	out := runApproveBashCLI(t, []string{"--config", missing, "--pi"}, payload)
	assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"config-error","message":"cannot read config `+missing+`"}}`, out)
}

func TestPiMode_FindInRepoAllow(t *testing.T) {
	repo := initGitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "subdir"), 0o755))
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool:    "find",
		Path:    "subdir",
		Pattern: "*.go",
		Cwd:     repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"find","decision":"allow","reason":"find"}`, out)
}

func TestPiMode_FindOutsideRepoNoop(t *testing.T) {
	repo := initGitRepo(t)
	outside := t.TempDir()
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool:    "find",
		Path:    outside,
		Pattern: "*.go",
		Cwd:     repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"find","decision":"noop","reason":"find"}`, out)
}

func TestPiMode_FindDefaultPathUsesCwd(t *testing.T) {
	repo := initGitRepo(t)
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool:    "find",
		Pattern: "*.go",
		Cwd:     repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"find","decision":"allow","reason":"find"}`, out)
}

func TestPiMode_LsDefaultCwdAllow(t *testing.T) {
	repo := initGitRepo(t)
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool: "ls",
		Cwd:  repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"ls","decision":"allow","reason":"ls"}`, out)
}

func TestPiMode_LsOutsideRepoNoop(t *testing.T) {
	repo := initGitRepo(t)
	outside := t.TempDir()
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool: "ls",
		Path: outside,
		Cwd:  repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"ls","decision":"noop","reason":"ls"}`, out)
}

func TestPiMode_LsLinkedWorktreeAllow(t *testing.T) {
	repo := initGitRepo(t)
	linkedWorktree := filepath.Join(t.TempDir(), "feature-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature", linkedWorktree, "HEAD")
	out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, PiInput{
		Tool: "ls",
		Path: linkedWorktree,
		Cwd:  repo,
	})
	assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"ls","decision":"allow","reason":"ls"}`, out)
}

func runPiModeForTest(t *testing.T, cfg Config, input PiInput) string {
	t.Helper()
	payload, err := json.Marshal(input)
	require.NoError(t, err)
	return parseAndEvaluatePiModeForTest(t, cfg, payload)
}

func parseAndEvaluatePiModeForTest(t *testing.T, cfg Config, payload []byte) string {
	t.Helper()
	var input PiInput
	if err := json.Unmarshal(payload, &input); err != nil {
		out, err := json.Marshal(buildPiErrorOutput("invalid-input", "invalid pi input"))
		require.NoError(t, err)
		return string(out)
	}
	_, r, err := evaluatePiToolUse(input, cfg)
	if err != nil {
		code := "invalid-input"
		if err.Error() == "unsupported tool: "+input.Tool {
			code = "unsupported-tool"
		}
		out, marshalErr := json.Marshal(buildPiErrorOutput(code, err.Error()))
		require.NoError(t, marshalErr)
		return string(out)
	}
	out, err := json.Marshal(buildPiOutput(input.Tool, r))
	require.NoError(t, err)
	return string(out)
}

func runPiModeWithConfigPathForTest(t *testing.T, configPath string, input PiInput) string {
	t.Helper()
	_, err := loadExplicitConfigFromPath(configPath)
	if err != nil {
		out, marshalErr := json.Marshal(buildPiErrorOutput("config-error", err.Error()))
		require.NoError(t, marshalErr)
		return string(out)
	}
	return runPiModeForTest(t, Config{Enabled: []string{"all"}}, input)
}

func runApproveBashCLI(t *testing.T, args []string, stdin []byte) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = "."
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "stderr: %s", stderr.String())
	return stdout.String()
}
