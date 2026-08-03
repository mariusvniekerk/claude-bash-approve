package main

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var chmodModePattern = regexp.MustCompile(`^(?:[0-7]{3,4}|[ugoa]*[+=-][rwxXstugo]*(?:,[ugoa]*[+=-][rwxXstugo]*)*)$`)

func resolveChmodDecision(args []*syntax.Word, ctx evalContext) *result {
	if ctx.cwd == "" || len(args) < 3 {
		return nil
	}

	modeIndex := 1
	for modeIndex < len(args) {
		literal := wordLiteral(args[modeIndex])
		if literal == "" {
			return nil
		}
		if literal == "--" {
			modeIndex++
			break
		}
		if isChmodMode(literal) {
			break
		}
		if !isChmodNoArgOption(literal) {
			return nil
		}
		modeIndex++
	}
	if modeIndex >= len(args) || !isChmodMode(wordLiteral(args[modeIndex])) {
		return nil
	}

	targetIndex := modeIndex + 1
	if targetIndex < len(args) && wordLiteral(args[targetIndex]) == "--" {
		targetIndex++
	}
	if targetIndex >= len(args) {
		return nil
	}
	for _, target := range args[targetIndex:] {
		literal := wordLiteralPathWithContext(target, ctx)
		if literal == "" || hasUnquotedPathExpansion(target) || hasUnquotedGlob(target) ||
			!destructiveTargetInAllowedScope(ctx.cwd, literal) {
			return nil
		}
	}
	return &result{decision: decisionAllow}
}

func isChmodMode(value string) bool {
	return chmodModePattern.MatchString(value)
}

func isChmodNoArgOption(value string) bool {
	if strings.HasPrefix(value, "--") {
		switch value {
		case "--recursive", "--changes", "--quiet", "--silent", "--verbose", "--preserve-root", "--no-preserve-root":
			return true
		default:
			return false
		}
	}
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	for _, option := range value[1:] {
		if !strings.ContainsRune("Rfhvc", option) {
			return false
		}
	}
	return true
}
