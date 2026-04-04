// Claude Code PreToolUse Hook: Compositional Bash Command Approval
//
// Auto-approves Bash commands that are safe combinations of:
//
//	WRAPPERS (timeout, env vars, .venv/bin/) + CORE COMMANDS (git, pytest, etc.)
//
// Chained commands (&&, ||, ;, |) are split via AST and ALL segments must be safe.
// Command substitutions ($(...)) are recursively evaluated — safe inner commands are allowed.
//
// DEBUG:
//
//	echo '{"tool_name":"Bash","tool_input":{"command":"timeout 30 pytest"}}' | go run main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
	"mvdan.cc/sh/v3/syntax"
)

type HookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Cwd       string          `json:"cwd"`
}

type BashInput struct {
	Command         string `json:"command"`
	Description     string `json:"description"`
	Timeout         int    `json:"timeout"`
	RunInBackground bool   `json:"run_in_background"`
}

type ReadInput struct {
	FilePath string `json:"file_path"`
}

type GrepInput struct {
	Pattern string   `json:"pattern"`
	Path    string   `json:"path"`
	Paths   []string `json:"paths"`
}

type HookOutput struct {
	HookSpecificOutput HookDecision `json:"hookSpecificOutput"`
}

type HookDecision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Decision constants for hook permission decisions.
const (
	decisionAllow = "allow"
	decisionDeny  = "deny"
	decisionAsk   = "ask" // prompts the user to confirm execution
)

// Well-known tag names used in matching logic.
const tagEnvVars = "env vars"

// result represents an evaluation outcome. A nil *result means unknown command (no opinion).
type result struct {
	reason     string
	decision   string
	denyReason string // human-readable explanation shown to Claude on deny
}

type evalContext struct {
	cwd string
}

func approved(reason string) *result { return &result{reason: reason, decision: decisionAllow} }

// shParser and shPrinter are reusable mvdan/sh instances.
var shParser = syntax.NewParser(syntax.Variant(syntax.LangBash))
var shPrinter = syntax.NewPrinter()

// Config represents the categories.yaml file.
type Config struct {
	Enabled  []string `yaml:"enabled"`
	Disabled []string `yaml:"disabled"`
}

// loadConfig reads categories.yaml from the same directory as the executable.
// Returns (config, nil) on success or if the file doesn't exist (defaults to all enabled).
// Returns an error if the file exists but can't be read or parsed.
func loadConfig() (Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return Config{Enabled: []string{"all"}}, nil
	}
	return loadConfigFromPath(filepath.Join(filepath.Dir(exe), "categories.yaml"))
}

// loadConfigFromPath reads a specific categories.yaml file.
// Returns (config, nil) if the file doesn't exist (defaults to all enabled).
// Returns an error if the file exists but can't be parsed.
func loadConfigFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{Enabled: []string{"all"}}, nil
		}
		return Config{}, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("cannot parse config %s: %w", path, err)
	}
	// If enabled is not set, default to all enabled.
	if cfg.Enabled == nil {
		cfg.Enabled = []string{"all"}
	}
	return cfg, nil
}

// isEnabled checks whether a pattern is enabled given its tags and pre-built sets.
// Tags are checked most-specific-first (tags[0], tags[1], ..., then "all").
// A disabled tag at any level overrides an enabled tag at a broader level.
func isEnabled(tags []string, enabled, disabled map[string]bool) bool {
	for _, tag := range tags {
		if disabled[tag] {
			return false
		}
		if enabled[tag] {
			return true
		}
	}
	if disabled["all"] {
		return false
	}
	return enabled["all"]
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// buildActivePatterns filters the global pattern lists based on the config.
func buildActivePatterns(cfg Config) (wrappers []pattern, commands []pattern) {
	// Fast path: if all enabled and nothing disabled, return globals directly.
	if len(cfg.Disabled) == 0 && len(cfg.Enabled) == 1 && cfg.Enabled[0] == "all" {
		return allWrapperPatterns, allCommandPatterns
	}

	enabled := toSet(cfg.Enabled)
	disabled := toSet(cfg.Disabled)
	for _, p := range allWrapperPatterns {
		if isEnabled(p.tags, enabled, disabled) {
			wrappers = append(wrappers, p)
		}
	}
	for _, p := range allCommandPatterns {
		if isEnabled(p.tags, enabled, disabled) {
			commands = append(commands, p)
		}
	}
	return
}

func emitDecision(decision, reason string) {
	out := HookOutput{
		HookSpecificOutput: HookDecision{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
		},
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	os.Exit(0)
}

// argsText prints CallExpr args to a flat string for regex matching.
func argsText(args []*syntax.Word) string {
	parts := make([]string, 0, len(args))
	for _, w := range args {
		var buf bytes.Buffer
		_ = shPrinter.Print(&buf, w)
		parts = append(parts, buf.String())
	}
	return strings.Join(parts, " ")
}

func stripWrappers(cmd string, wrapperPats []pattern) (string, []string) {
	var wrappers []string
	changed := true
	for changed {
		changed = false
		for _, wp := range wrapperPats {
			loc := wp.re.FindStringIndex(cmd)
			if loc != nil && loc[0] == 0 {
				wrappers = append(wrappers, wp.label())
				cmd = cmd[loc[1]:]
				changed = true
				break
			}
		}
	}
	return strings.TrimSpace(cmd), wrappers
}

func matchCommand(cmd string, commandPats []pattern) *pattern {
	for i := range commandPats {
		if commandPats[i].re.MatchString(cmd) {
			return &commandPats[i]
		}
	}
	return nil
}

// checkSubstitutions walks an AST node for CmdSubst and ProcSubst,
// recursively evaluating each inner command. Returns false if any inner
// command is unsafe.
func checkSubstitutions(node syntax.Node, ctx evalContext, wrapperPats, commandPats []pattern) bool {
	safe := true
	syntax.Walk(node, func(n syntax.Node) bool {
		if !safe {
			return false
		}
		var stmts []*syntax.Stmt
		switch x := n.(type) {
		case *syntax.CmdSubst:
			stmts = x.Stmts
		case *syntax.ProcSubst:
			stmts = x.Stmts
		}
		if stmts != nil {
			for _, stmt := range stmts {
				r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
				if r == nil || r.decision == decisionDeny {
					safe = false
					return false
				}
			}
			return false // don't recurse into children, we handled stmts
		}
		return true
	})
	return safe
}

// evaluateStmt evaluates a single statement from the AST.
// Returns nil if rejected.
func evaluateStmt(stmt *syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	for _, redir := range stmt.Redirs {
		if redir.Word != nil {
			if !checkSubstitutions(redir.Word, ctx, wrapperPats, commandPats) {
				return nil
			}
		}
		if redir.Hdoc != nil {
			if !checkSubstitutions(redir.Hdoc, ctx, wrapperPats, commandPats) {
				return nil
			}
		}
	}

	if stmt.Cmd == nil {
		return nil
	}

	return evaluateCommand(stmt.Cmd, ctx, wrapperPats, commandPats)
}

// evaluateCommand dispatches to the appropriate handler based on AST node type.
func evaluateCommand(cmd syntax.Command, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	switch c := cmd.(type) {
	case *syntax.BinaryCmd:
		return evaluateBinaryCmd(c, ctx, wrapperPats, commandPats)
	case *syntax.CallExpr:
		return evaluateCallExpr(c, ctx, wrapperPats, commandPats)
	case *syntax.DeclClause:
		// export, declare, local, readonly, typeset
		return approved("shell vars")
	case *syntax.ForClause:
		if !checkForLoop(c.Loop, ctx, wrapperPats, commandPats) {
			return nil
		}
		return evaluateBlock(c.Do, ctx, wrapperPats, commandPats, "for")
	case *syntax.WhileClause:
		for _, stmt := range c.Cond {
			r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
			if r == nil {
				return nil
			}
		}
		return evaluateBlock(c.Do, ctx, wrapperPats, commandPats, "while")
	case *syntax.IfClause:
		return evaluateIfClause(c, ctx, wrapperPats, commandPats)
	case *syntax.Subshell:
		return evaluateBlock(c.Stmts, ctx, wrapperPats, commandPats, "subshell")
	default:
		// Reject func declarations, etc.
		return nil
	}
}

// mergeStmtResults evaluates all statements and merges their results.
// Returns nil if any statement is rejected.
func mergeStmtResults(stmts []*syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	var reasons []string
	var firstDenyReason string
	askDecision := false
	denyDecision := false
	noOpinion := false
	for _, stmt := range stmts {
		r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
		if r == nil {
			return nil
		}
		reasons = append(reasons, r.reason)
		switch r.decision {
		case decisionDeny:
			denyDecision = true
			if firstDenyReason == "" && r.denyReason != "" {
				firstDenyReason = r.denyReason
			}
		case decisionAsk:
			askDecision = true
		case decisionAllow:
			// no change
		default:
			noOpinion = true
		}
	}
	out := approved(strings.Join(reasons, " | "))
	// Priority: deny > ask > no-opinion > allow
	if denyDecision {
		out.decision = decisionDeny
		out.denyReason = firstDenyReason
	} else if askDecision {
		out.decision = decisionAsk
	} else if noOpinion {
		out.decision = ""
	}
	return out
}

// evaluateBlock checks that all statements in a block (for body, while body, subshell) are safe.
func evaluateBlock(stmts []*syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern, label string) *result {
	r := mergeStmtResults(stmts, ctx, wrapperPats, commandPats)
	if r == nil {
		return nil
	}
	r.reason = label + "{" + r.reason + "}"
	return r
}

// evaluateIfClause checks all branches of an if/elif/else.
func evaluateIfClause(ic *syntax.IfClause, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	// Check condition statements
	for _, stmt := range ic.Cond {
		r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
		if r == nil {
			return nil
		}
	}
	// Check then body
	r := evaluateBlock(ic.Then, ctx, wrapperPats, commandPats, "if")
	if r == nil {
		return nil
	}
	// Check else/elif
	if ic.Else != nil {
		elseR := evaluateCommand(ic.Else, ctx, wrapperPats, commandPats)
		if elseR == nil {
			return nil
		}
	}
	return r
}

// checkForLoop validates the iteration/condition of a for loop.
// Returns false if any substitution inside the loop header is unsafe.
func checkForLoop(loop syntax.Loop, ctx evalContext, wrapperPats, commandPats []pattern) bool {
	switch l := loop.(type) {
	case *syntax.WordIter:
		for _, word := range l.Items {
			if !checkSubstitutions(word, ctx, wrapperPats, commandPats) {
				return false
			}
		}
	case *syntax.CStyleLoop:
		for _, expr := range []syntax.ArithmExpr{l.Init, l.Cond, l.Post} {
			if expr != nil {
				if !checkSubstitutions(expr, ctx, wrapperPats, commandPats) {
					return false
				}
			}
		}
	}
	return true
}

// evaluateBinaryCmd handles &&, ||, | chains.
func evaluateBinaryCmd(bc *syntax.BinaryCmd, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	left := evaluateStmt(bc.X, ctx, wrapperPats, commandPats)
	if left == nil {
		return nil
	}
	right := evaluateStmt(bc.Y, ctx, wrapperPats, commandPats)
	if right == nil {
		return nil
	}
	r := approved(left.reason + " | " + right.reason)
	// Priority: deny > ask > no-opinion > allow
	if left.decision == decisionDeny || right.decision == decisionDeny {
		r.decision = decisionDeny
		if left.denyReason != "" {
			r.denyReason = left.denyReason
		} else {
			r.denyReason = right.denyReason
		}
	} else if left.decision == decisionAsk || right.decision == decisionAsk {
		r.decision = decisionAsk
	} else if left.decision == "" || right.decision == "" {
		r.decision = ""
	}
	return r
}

// evaluateCallExpr handles simple commands (the most common case).
func evaluateCallExpr(call *syntax.CallExpr, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	// Standalone variable assignment: FOO=bar (no command)
	if len(call.Args) == 0 && len(call.Assigns) > 0 {
		if !checkSubstitutions(call, ctx, wrapperPats, commandPats) {
			return nil
		}
		return approved("var assignment")
	}

	if len(call.Args) == 0 {
		return nil
	}

	// Resolve command name. Allow $(which X) / $(command -v X) / $(...)/path/to/cmd.
	cmdName := wordLiteral(call.Args[0])
	if cmdName == "" {
		resolved, knownSafe := resolveWhichSubst(call.Args[0])
		if resolved == "" {
			return nil
		}
		if knownSafe {
			// $(which X) / $(command -v X) — inherently safe, only check remaining args
			for _, arg := range call.Args[1:] {
				if !checkSubstitutions(arg, ctx, wrapperPats, commandPats) {
					return nil
				}
			}
		} else {
			// $(...)/path/to/cmd — must verify the inner substitution is safe too
			for _, arg := range call.Args {
				if !checkSubstitutions(arg, ctx, wrapperPats, commandPats) {
					return nil
				}
			}
		}
		return matchAndBuild(resolved, call.Args[1:], call.Assigns, call.Args, ctx, wrapperPats, commandPats)
	}

	// Normal path: check all substitutions
	if !checkSubstitutions(call, ctx, wrapperPats, commandPats) {
		return nil
	}

	return matchAndBuild(argsText(call.Args), nil, call.Assigns, call.Args, ctx, wrapperPats, commandPats)
}

// matchAndBuild strips wrappers, matches the command, and builds the result.
// cmdText is the primary command text. extraArgs are appended if non-nil.
// astArgs are the raw AST arguments passed to pattern validators.
func matchAndBuild(cmdText string, extraArgs []*syntax.Word, assigns []*syntax.Assign, astArgs []*syntax.Word, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	if len(extraArgs) > 0 {
		cmdText += " " + argsText(extraArgs)
	}
	coreCmd, wrappers := stripWrappers(cmdText, wrapperPats)
	matched := matchCommand(coreCmd, commandPats)
	if matched == nil {
		return nil
	}
	// Run post-match validator if present; false downgrades to "ask".
	if matched.validate != nil && !matched.validate(astArgs, ctx) {
		return &result{reason: matched.label(), decision: matched.validateFallback}
	}
	if len(assigns) > 0 && !slices.Contains(wrappers, tagEnvVars) {
		wrappers = append([]string{tagEnvVars}, wrappers...)
	}
	reason := matched.label()
	if len(wrappers) > 0 {
		reason = strings.Join(wrappers, "+") + "+" + reason
	}
	return &result{reason: reason, decision: matched.decision, denyReason: matched.denyReason}
}

// wordLiteral returns the literal string value of a word if it contains no
// expansions (parameter expansion, command substitution, etc.). Returns ""
// if the word is dynamic.
func wordLiteral(w *syntax.Word) string {
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, dp := range p.Parts {
				if lit, ok := dp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				} else {
					return ""
				}
			}
		default:
			return ""
		}
	}
	return sb.String()
}

// resolveWhichSubst checks if a word resolves to a known command via:
//   - $(which <cmd>) or $(command -v <cmd>) — returned as knownSafe=true
//   - $(...)/path/to/<cmd> — any substitution followed by a path suffix
//     (e.g. $(go env GOROOT)/bin/go, $(brew --prefix)/bin/foo)
//
// For the path suffix case, the caller must verify inner substitutions are safe.
// Returns (resolved command name, knownSafe), or ("", false) if not recognized.
func resolveWhichSubst(w *syntax.Word) (string, bool) {
	// Case 1: $(which X) or $(command -v X) — single part, inherently safe
	if len(w.Parts) == 1 {
		resolved := resolveWhichOrCommandV(w.Parts[0])
		return resolved, true
	}

	// Case 2: $(...)/path/to/cmd — CmdSubst followed by a literal path suffix.
	if len(w.Parts) == 2 {
		_, ok := w.Parts[0].(*syntax.CmdSubst)
		if !ok {
			return "", false
		}
		lit, ok := w.Parts[1].(*syntax.Lit)
		if !ok {
			return "", false
		}
		// Extract the final path component as the command name (e.g. "/bin/go" → "go")
		suffix := lit.Value
		lastSlash := strings.LastIndex(suffix, "/")
		if lastSlash < 0 || lastSlash == len(suffix)-1 {
			return "", false
		}
		return suffix[lastSlash+1:], false
	}

	return "", false
}

// resolveWhichOrCommandV handles $(which X) and $(command -v X).
func resolveWhichOrCommandV(part syntax.WordPart) string {
	cs, ok := part.(*syntax.CmdSubst)
	if !ok || len(cs.Stmts) != 1 {
		return ""
	}
	call, ok := cs.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 {
		return ""
	}
	cmd := wordLiteral(call.Args[0])
	switch {
	case cmd == "which" && len(call.Args) == 2:
		return wordLiteral(call.Args[1])
	case cmd == "command" && len(call.Args) == 3 && wordLiteral(call.Args[1]) == "-v":
		return wordLiteral(call.Args[2])
	default:
		return ""
	}
}

// Evaluate checks whether a command should be approved given a config.
// Returns nil if rejected, or a *result with the approval reason.
func Evaluate(cmd string, cfg Config, ctx evalContext) *result {
	wrappers, commands := buildActivePatterns(cfg)
	return evaluate(cmd, ctx, wrappers, commands)
}

// evaluate checks whether a command should be approved against pre-built pattern lists.
func evaluate(cmd string, ctx evalContext, wrapperPats []pattern, commandPats []pattern) *result {
	if cmd == "" {
		return nil
	}

	file, err := shParser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}

	if len(file.Stmts) == 0 {
		return nil
	}

	return mergeStmtResults(file.Stmts, ctx, wrapperPats, commandPats)
}

func evaluateToolUse(input HookInput, cfg Config) (string, *result) {
	ctx := evalContext{cwd: input.Cwd}

	switch input.ToolName {
	case "Bash":
		var bashInput BashInput
		if err := json.Unmarshal(input.ToolInput, &bashInput); err != nil {
			return "", nil
		}
		return bashInput.Command, Evaluate(bashInput.Command, cfg, ctx)
	case "Read":
		var readInput ReadInput
		if err := json.Unmarshal(input.ToolInput, &readInput); err != nil {
			return "", nil
		}
		return readInput.FilePath, evaluateReadTool(readInput, ctx)
	case "Grep":
		var grepInput GrepInput
		if err := json.Unmarshal(input.ToolInput, &grepInput); err != nil {
			return "", nil
		}
		return grepCommandLabel(grepInput), evaluateGrepTool(grepInput, ctx)
	default:
		return "", nil
	}
}

func grepCommandLabel(input GrepInput) string {
	if input.Path != "" {
		return input.Path
	}
	if len(input.Paths) > 0 {
		return strings.Join(input.Paths, ",")
	}
	return input.Pattern
}

func main() {
	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}

	var data HookInput
	if err := json.Unmarshal(rawInput, &data); err != nil {
		os.Exit(0)
	}

	db := openTelemetryDB()
	if db != nil {
		defer db.Close() //nolint:errcheck // best-effort telemetry
	}

	payload := string(rawInput)

	cfg, err := loadConfig()
	if err != nil {
		logDecision(db, payload, "", decisionDeny, err.Error())
		emitDecision(decisionDeny, err.Error())
	}

	cmd, r := evaluateToolUse(data, cfg)
	if r == nil {
		logDecision(db, payload, cmd, "no-opinion", "")
		os.Exit(0)
	}
	// No-opinion: recognized command but no decision — exit silently, next hook handles it.
	if r.decision == "" {
		logDecision(db, payload, cmd, "no-opinion", r.reason)
		os.Exit(0)
	}
	if r.decision == decisionAsk {
		logDecision(db, payload, cmd, "ask", r.reason)
		emitDecision(decisionAsk, r.reason)
	}

	reason := r.reason
	if r.decision == decisionDeny && r.denyReason != "" {
		reason = r.denyReason
	}
	logDecision(db, payload, cmd, r.decision, reason)
	emitDecision(r.decision, reason)
}
