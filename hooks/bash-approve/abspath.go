package main

import (
	"os"
	"path/filepath"
	"strings"
)

// staticAbsPathPrefixes are absolute path prefixes that auto-approve regardless
// of the user. $HOME-relative prefixes are also allowed and are joined with
// HOME at validation time (see homeRelativeAbsPathSuffixes).
var staticAbsPathPrefixes = []string{
	"/usr/",
	"/bin/",
	"/sbin/",
	"/opt/homebrew/bin/",
	"/opt/homebrew/sbin/",
	"/opt/local/bin/",
	"/opt/local/sbin/",
	"/nix/store/",
	"/run/current-system/",
	"/etc/profiles/per-user/",
}

// homeRelativeAbsPathSuffixes are subpaths of $HOME that auto-approve.
var homeRelativeAbsPathSuffixes = []string{
	".cargo/bin/",
	".rustup/",
	".local/bin/",
	"go/bin/",
	".bun/bin/",
	".nvm/",
	".fnm/",
	".npm/",
	".pyenv/",
	".nix-profile/",
	".local/share/mise/installs/",
	".local/share/mise/shims/",
}

// isSafeAbsolutePath returns nil if matchedPrefix starts with one of the
// allowed prefixes (static system / $HOME-relative). Otherwise returns
// &result{decision: ask}.
//
// Repo-root prefixes are intentionally NOT allowlisted: a binary at
// `/repo/bin/git` whose basename matches an allow pattern would otherwise
// auto-approve while the shell executes whatever the repo contains. The
// repo can hold attacker- or model-controlled code that the validator
// has no way to inspect, so absolute repo paths must ask.
//
// The matched prefix is normalized via filepath.Clean before comparison so
// that `..` segments cannot escape an allowed prefix. e.g. without
// normalization, `/usr/../../tmp/evil` would match the `/usr/` prefix even
// though the shell resolves it to `/tmp/evil`.
func isSafeAbsolutePath(matchedPrefix string, _ evalContext) *result {
	cleaned := filepath.Clean(matchedPrefix) + "/"
	for _, p := range staticAbsPathPrefixes {
		if strings.HasPrefix(cleaned, p) {
			return nil
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		for _, suffix := range homeRelativeAbsPathSuffixes {
			full := filepath.Join(home, suffix)
			if !strings.HasSuffix(full, "/") {
				full += "/"
			}
			if strings.HasPrefix(cleaned, full) {
				return nil
			}
		}
	}
	return &result{decision: decisionAsk}
}
