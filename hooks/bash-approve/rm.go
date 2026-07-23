package main

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// resolveRecursiveRmDecision allows recursive removal only when every target
// is an explicit strict descendant of the current repository. The pattern's
// baseline deny remains in force for dynamic paths, globs, outside paths, and
// attempts to remove the repository root itself.
func resolveRecursiveRmDecision(args []*syntax.Word, ctx evalContext) *result {
	if ctx.cwd == "" || len(args) < 3 {
		return nil
	}

	parsingOptions := true
	targets := 0
	for _, arg := range args[1:] {
		literal := wordLiteralPathWithContext(arg, ctx)
		if literal == "" {
			return nil
		}
		if parsingOptions {
			if literal == "--" {
				parsingOptions = false
				continue
			}
			if strings.HasPrefix(literal, "-") {
				continue
			}
			parsingOptions = false
		}

		if hasUnquotedGlob(arg) || !recursiveRmTargetInCurrentRepo(ctx.cwd, literal) {
			return nil
		}
		targets++
	}

	if targets == 0 {
		return nil
	}
	return &result{decision: decisionAllow}
}

func recursiveRmTargetInCurrentRepo(cwd, target string) bool {
	repoRoot := repoRootForCwd(cwd)
	if repoRoot == "" {
		return false
	}

	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(cwd, targetPath)
	}
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	targetPath, err = resolveRecursiveRmTarget(targetPath)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(repoRoot, targetPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}

// resolveRecursiveRmTarget resolves every existing path component, then
// appends any non-existent suffix. This keeps macOS paths such as /var and
// /private/var comparable and rejects targets reached through escaping
// symlinks.
func resolveRecursiveRmTarget(target string) (string, error) {
	probe := filepath.Clean(target)
	suffix := ""
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(probe), suffix)
		probe = parent
	}
}
