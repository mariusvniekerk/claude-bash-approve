package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// findExecFlags are find flags that accept a command terminated by ; or +.
var findExecFlags = map[string]bool{
	"-exec":    true,
	"-execdir": true,
	"-ok":      true,
	"-okdir":   true,
}

// findDangerousFlags are find actions that modify the filesystem.
var findDangerousFlags = map[string]bool{
	"-delete": true,
}

func init() {
	// Attach the find validator after allCommandPatterns is initialized
	// to break the init cycle (isFindSafe → evaluate → allCommandPatterns).
	for i := range allCommandPatterns {
		if allCommandPatterns[i].tags[0] == "find" {
			allCommandPatterns[i].validate = isFindSafe
			break
		}
	}
}

// isFindSafe validates a find invocation by checking -exec/-execdir commands
// through the normal evaluation pipeline. Returns false (ask) if any embedded
// command is unrecognized or if -delete is present.
// args includes the command name at args[0].
func isFindSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}

	for i := 1; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			continue // non-literal args (globs, expansions) are fine for find paths/patterns
		}

		if findDangerousFlags[lit] {
			return false
		}

		if !findExecFlags[lit] {
			continue
		}

		// Collect the command between -exec and the terminator (; or +).
		// find -exec git diff {} \;
		//            ^^^^^^^^^^ this part
		i++
		var cmdParts []string
		for i < len(args) {
			part := wordLiteral(args[i])
			if part == "" {
				// non-literal in exec command — can't validate statically
				return false
			}
			if part == ";" || part == `\;` || part == "+" {
				break
			}
			cmdParts = append(cmdParts, part)
			i++
		}

		if len(cmdParts) == 0 {
			return false // -exec with no command
		}

		// Evaluate the embedded command through the normal pipeline.
		cmd := strings.Join(cmdParts, " ")
		r := evaluate(cmd, ctx, allWrapperPatterns, allCommandPatterns)
		if r == nil {
			return false // unknown command
		}
		if r.decision != decisionAllow {
			return false
		}
	}

	return true
}
