package main

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var recursiveRmStandardTempRoots = [...]string{
	"/tmp",
	"/var/tmp",
	"/private/tmp",
	"/private/var/tmp",
}

// resolveRecursiveRmDecision allows recursive removal only when every target
// is an explicit strict descendant of the current repository or a standard
// temporary directory. The pattern's baseline deny remains in force for
// dynamic paths, globs, outside paths, and attempts to remove an allowed root.
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
		if hasUnquotedRmTargetExpansion(arg) {
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

		if hasUnquotedGlob(arg) || !recursiveRmTargetInAllowedScope(ctx.cwd, literal) {
			return nil
		}
		targets++
	}

	if targets == 0 {
		return nil
	}
	return &result{decision: decisionAllow}
}

func hasUnquotedRmTargetExpansion(word *syntax.Word) bool {
	for _, part := range word.Parts {
		switch part.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst:
			return true
		}
	}
	return false
}

func recursiveRmTargetInAllowedScope(cwd, target string) bool {
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

	repoRoot := repoRootForCwd(cwd)
	if repoRoot != "" {
		if targetPath == repoRoot {
			return false
		}
		if recursiveRmTargetStrictlyBelow(repoRoot, targetPath) {
			return true
		}
	}
	for _, root := range recursiveRmStandardTempRoots {
		resolvedRoot, err := resolveRecursiveRmTarget(root)
		if err == nil && recursiveRmTargetStrictlyBelow(resolvedRoot, targetPath) {
			return true
		}
	}
	return false
}

func recursiveRmTargetStrictlyBelow(root, target string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
