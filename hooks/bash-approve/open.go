package main

import (
	"net/url"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var mediaFileExtensions = map[string]bool{
	".jpeg": true,
	".jpg":  true,
	".pdf":  true,
	".png":  true,
	".svg":  true,
	".webm": true,
}

// isMediaOpenTarget allows open/xdg-open only for literal local paths whose
// extensions look like image or video files. Flags and remote URLs fall back
// to no-opinion so other approval layers can decide.
func isMediaOpenTarget(args []*syntax.Word, _ evalContext) bool {
	if len(args) < 2 {
		return false
	}

	start := 1
	if wordLiteral(args[start]) == "--" {
		start++
	}
	if start >= len(args) {
		return false
	}

	for _, arg := range args[start:] {
		target := wordLiteral(arg)
		if target == "" || strings.HasPrefix(target, "-") {
			return false
		}
		if strings.Contains(target, "://") {
			if !isLocalWebURL(target) {
				return false
			}
			continue
		}
		if !mediaFileExtensions[strings.ToLower(filepath.Ext(target))] {
			return false
		}
	}

	return true
}

func isLocalWebURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	switch u.Hostname() {
	case "localhost", "127.0.0.1":
		return true
	default:
		return false
	}
}
