package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	gitcmd "go.kenn.io/kit/git/cmd"
	"mvdan.cc/sh/v3/syntax"
)

func isGitTagReadOnly(args []*syntax.Word, ctx evalContext) bool {
	decoded := make([]string, len(args))
	for i, arg := range args {
		value, ok := wordDecodedLiteralWithContext(arg, ctx)
		if !ok {
			return false
		}
		decoded[i] = value
	}

	tagIndex := -1
	for i := range len(decoded) {
		if decoded[i] != "git" {
			continue
		}
		candidate := i + 1
		if candidate < len(decoded) && decoded[candidate] == "-C" {
			candidate += 2
		}
		if candidate < len(decoded) && decoded[candidate] == "tag" {
			tagIndex = candidate
			break
		}
	}
	if tagIndex < 0 {
		return false
	}
	if tagIndex == len(decoded)-1 {
		return true
	}

	listMode := false
	for _, arg := range decoded[tagIndex+1:] {
		switch {
		case arg == "-l", arg == "--list", arg == "-i", arg == "--ignore-case",
			arg == "--no-column", arg == "--omit-empty":
			listMode = true
		case isGitTagLinesOption(arg), isGitTagListValueOption(arg):
			listMode = true
		case arg == "--":
			if !listMode {
				return false
			}
		case strings.HasPrefix(arg, "-"):
			return false
		default:
			if !listMode {
				return false
			}
		}
	}
	return listMode
}

func isGitTagLinesOption(arg string) bool {
	if arg == "-n" {
		return true
	}
	if !strings.HasPrefix(arg, "-n") || len(arg) == 2 {
		return false
	}
	for _, digit := range arg[2:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func isGitTagListValueOption(arg string) bool {
	for _, name := range []string{
		"contains",
		"no-contains",
		"points-at",
		"merged",
		"no-merged",
		"sort",
		"format",
		"column",
		"color",
	} {
		option := "--" + name
		if arg == option || strings.HasPrefix(arg, option+"=") {
			return true
		}
	}
	return false
}

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

func evaluateFindTool(input PiInput, ctx evalContext) *result {
	target := input.Path
	if target == "" {
		target = "."
	}
	if pathInCurrentRepoFamily(ctx.cwd, target) {
		return approved("find")
	}
	return &result{reason: "find", decision: ""}
}

func evaluateLsTool(input PiInput, ctx evalContext) *result {
	target := input.Path
	if target == "" {
		target = "."
	}
	if pathInCurrentRepoFamily(ctx.cwd, target) {
		return approved("ls")
	}
	return &result{reason: "ls", decision: ""}
}

func isCurrentRepoWorktreeCD(args []*syntax.Word, ctx evalContext) bool {
	if ctx.cwd == "" || len(args) != 2 {
		return false
	}

	target := wordLiteralPathWithContext(args[1], ctx)
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

	if pathWithinAnySafeCDPrefix(resolvedTarget, ctx.safeCDPrefixes) {
		return true
	}

	if pathIsExistingDirWithinRepo(ctx.cwd, resolvedTarget) {
		return true
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

func pathWithinAnySafeCDPrefix(target string, prefixes []string) bool {
	if target == "" {
		return false
	}

	target, err := filepath.Abs(target)
	if err != nil {
		return false
	}

	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !filepath.IsAbs(prefix) {
			continue
		}

		prefix, err := filepath.Abs(prefix)
		if err != nil {
			continue
		}
		if resolvedPrefix, err := filepath.EvalSymlinks(prefix); err == nil {
			prefix = resolvedPrefix
		}

		if pathWithinDir(prefix, target) {
			return true
		}
	}
	return false
}

func pathIsExistingDirWithinRepo(cwd, target string) bool {
	repoRoot := repoRootForCwd(cwd)
	if repoRoot == "" || target == "" {
		return false
	}

	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return false
	}

	return pathWithinDir(repoRoot, target)
}

func pathWithinDir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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

func pathInCurrentRepoFamily(cwd, target string) bool {
	if pathInCurrentRepo(cwd, target) {
		return true
	}
	if cwd == "" || target == "" {
		return false
	}

	resolvedTarget, err := resolvePathFromCwd(cwd, target)
	if err != nil {
		return false
	}

	currentCommonDir, err := gitResolvedPath(cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}

	probeDir := resolvedTarget
	if info, statErr := os.Stat(resolvedTarget); statErr == nil && !info.IsDir() {
		probeDir = filepath.Dir(resolvedTarget)
	}

	targetTopLevel, err := gitResolvedPath(probeDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	targetCommonDir, err := gitResolvedPath(probeDir, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}
	if currentCommonDir != targetCommonDir {
		return false
	}

	rel, err := filepath.Rel(targetTopLevel, resolvedTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := gitcmd.New().Output(context.Background(), dir, args...)
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
