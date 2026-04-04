package main

import (
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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
