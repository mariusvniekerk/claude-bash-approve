package main

import (
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func evaluateReadTool(input ReadInput, ctx evalContext) *result {
	if pathInCurrentRepo(ctx.cwd, input.FilePath) {
		return approved("read")
	}
	return &result{reason: "read", decision: ""}
}

func evaluateGrepTool(input GrepInput, ctx evalContext) *result {
	paths := input.Paths
	if input.Path != "" {
		paths = append([]string{input.Path}, paths...)
	}

	if len(paths) == 0 {
		if repoRootForCwd(ctx.cwd) == "" {
			return &result{reason: "grep", decision: ""}
		}
		return approved("grep")
	}

	for _, path := range paths {
		if !pathInCurrentRepo(ctx.cwd, path) {
			return &result{reason: "grep", decision: ""}
		}
	}
	return approved("grep")
}

func isCurrentRepoWorktreeCD(args []*syntax.Word, ctx evalContext) bool {
	if ctx.cwd == "" || len(args) != 2 {
		return false
	}

	target := wordLiteral(args[1])
	if target == "" || target == "-" {
		return false
	}

	targetPath := filepath.Join(ctx.cwd, target)
	if filepath.IsAbs(target) {
		targetPath = target
	}

	resolvedTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	resolvedTarget, err = filepath.EvalSymlinks(resolvedTarget)
	if err != nil {
		return false
	}

	currentCommonDir, err := gitResolvedPath(ctx.cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}

	targetTopLevel, err := gitResolvedPath(resolvedTarget, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	if targetTopLevel != resolvedTarget {
		return false
	}

	targetCommonDir, err := gitResolvedPath(resolvedTarget, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}

	return currentCommonDir == targetCommonDir
}

func repoRootForCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	root, err := gitResolvedPath(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

func pathInCurrentRepo(cwd, target string) bool {
	repoRoot := repoRootForCwd(cwd)
	if repoRoot == "" || target == "" {
		return false
	}

	resolvedTarget, err := resolvePathFromCwd(cwd, target)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(repoRoot, resolvedTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitResolvedPath(dir string, args ...string) (string, error) {
	out, err := gitOutput(dir, args...)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(dir, out)
	}
	return filepath.EvalSymlinks(out)
}

func resolvePathFromCwd(cwd, target string) (string, error) {
	targetPath := filepath.Join(cwd, target)
	if filepath.IsAbs(target) {
		targetPath = target
	}

	resolvedTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(resolvedTarget)
}
