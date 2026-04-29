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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

type ReadInput struct {
	FilePath string `json:"file_path"`
}

type GrepInput struct {
	Pattern string   `json:"pattern"`
	Path    string   `json:"path"`
	Paths   []string `json:"paths"`
}

type PiInput struct {
	Tool    string   `json:"tool"`
	Command string   `json:"command,omitempty"`
	Path    string   `json:"path,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Cwd     string   `json:"cwd"`
}

type PiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PiOutput struct {
	Version  int      `json:"version"`
	Kind     string   `json:"kind"`
	Tool     string   `json:"tool,omitempty"`
	Decision string   `json:"decision,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Error    *PiError `json:"error,omitempty"`
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
	// wrapperPats and commandPats carry the config-filtered pattern
	// lists through nested evaluation (find -exec, xargs, awk system).
	// evaluate() seeds them on every call so they are always populated;
	// nested validators must use ctx.wrapperPats/ctx.commandPats instead
	// of wrapperPatterns()/commandPatterns() to honor disabled categories.
	wrapperPats []pattern
	commandPats []pattern
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

func loadExplicitConfigFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("cannot read config %s", path)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("cannot parse config %s: %w", path, err)
	}
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

// buildActivePatterns filters pattern lists based on the config.
func buildActivePatterns(cfg Config) (wrappers []pattern, commands []pattern) {
	// Fast path: if all enabled and nothing disabled, return all patterns.
	if len(cfg.Disabled) == 0 && len(cfg.Enabled) == 1 && cfg.Enabled[0] == "all" {
		return wrapperPatterns(), commandPatterns()
	}

	enabled := toSet(cfg.Enabled)
	disabled := toSet(cfg.Disabled)
	for _, p := range wrapperPatterns() {
		if isEnabled(p.tags, enabled, disabled) {
			wrappers = append(wrappers, p)
		}
	}
	for _, p := range commandPatterns() {
		if isEnabled(p.tags, enabled, disabled) {
			commands = append(commands, p)
		}
	}
	return
}

func emitPiOutput(out PiOutput) {
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	os.Exit(0)
}

func buildPiErrorOutput(code, message string) PiOutput {
	return PiOutput{
		Version: 1,
		Kind:    "error",
		Error: &PiError{
			Code:    code,
			Message: message,
		},
	}
}

// argsText flattens CallExpr args to a string for regex matching. Each word is
// decoded as the shell would (quote stripping, backslash escapes, ANSI-C
// $'...'); words with dynamic parts (CmdSubst, ParamExp) fall back to the
// printed source so substitution-aware logic still sees them.
func argsText(args []*syntax.Word) string {
	parts := make([]string, 0, len(args))
	for _, w := range args {
		parts = append(parts, wordMatchText(w))
	}
	return strings.Join(parts, " ")
}

// wordMatchText returns the shell-decoded literal of a word, or the printed
// source when the word contains dynamic expansions. Single-element words
// whose decoded form contains shell whitespace also fall back to the
// printed source: argsText joins decoded words with spaces, so passing
// `'git status'` (one argv element with embedded space) through as
// `git status` would let the matcher mistake it for two argv elements
// and approve it as a git read op. Whitespace that originates from a
// top-level CmdSubst is not subject to the fallback, since that
// whitespace is the IFS argv boundary bash would split on.
func wordMatchText(w *syntax.Word) string {
	if decoded, ok := wordDecodedLiteral(w); ok {
		if !hasNonCmdSubstWhitespace(w) {
			return decoded
		}
	}
	var buf bytes.Buffer
	_ = shPrinter.Print(&buf, w)
	return buf.String()
}

// hasNonCmdSubstWhitespace reports whether any part of the word other
// than a top-level CmdSubst decodes to a string containing shell
// whitespace. Such whitespace is part of one argv element and would
// create false word boundaries if joined into the matcher's argv string.
func hasNonCmdSubstWhitespace(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, part := range w.Parts {
		if _, ok := part.(*syntax.CmdSubst); ok {
			continue
		}
		single := &syntax.Word{Parts: []syntax.WordPart{part}}
		if decoded, ok := wordDecodedLiteral(single); ok {
			if strings.ContainsAny(decoded, " \t\n\r\v\f") {
				return true
			}
		}
	}
	return false
}

// wrapperMatch records both the matched pattern and the matched text for a
// single wrapper hit, so wrapper validators can inspect the prefix bash
// would actually evaluate (e.g. the env-vars wrapper validator needs the
// raw `KEY=val ` text to extract names).
type wrapperMatch struct {
	pattern *pattern
	text    string
}

func stripWrappers(cmd string, wrapperPats []pattern) (string, []wrapperMatch) {
	var wrappers []wrapperMatch
	changed := true
	for changed {
		changed = false
		for i := range wrapperPats {
			wp := &wrapperPats[i]
			loc := wp.re.FindStringIndex(cmd)
			if loc != nil && loc[0] == 0 {
				wrappers = append(wrappers, wrapperMatch{pattern: wp, text: cmd[:loc[1]]})
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
// recursively evaluating each inner command. Returns the most-pessimistic
// inner *result by decision priority (deny > ask > no-opinion > allow), or
// nil if any substitution is unrecognized — the caller should reject the
// outer command. A non-nil allow result means every substitution resolved
// to allow; the caller continues with normal matching. Any other non-nil
// decision should propagate as the outer command's decision so a
// substitution can't smuggle a side-effecting command past the outer
// rule's auto-approve, and ask/deny prompts/blocks reach the user with
// the inner command's reason.
func checkSubstitutions(node syntax.Node, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	var worst *result
	rejected := false
	syntax.Walk(node, func(n syntax.Node) bool {
		if rejected {
			return false
		}
		var stmts []*syntax.Stmt
		switch x := n.(type) {
		case *syntax.CmdSubst:
			stmts = x.Stmts
		case *syntax.ProcSubst:
			stmts = x.Stmts
		}
		if stmts == nil {
			return true
		}
		for _, stmt := range stmts {
			r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
			if r == nil {
				rejected = true
				return false
			}
			if worst == nil || decisionRank(r.decision) > decisionRank(worst.decision) {
				worst = r
			}
		}
		return false // don't recurse into children, we handled stmts
	})
	if rejected {
		return nil
	}
	if worst == nil {
		// No substitutions encountered — equivalent to allow.
		return approved("")
	}
	return worst
}

// decisionRank orders decisions worst-first so the most-pessimistic
// inner result wins when multiple substitutions live in one word.
// deny > ask > no-opinion > allow.
func decisionRank(d string) int {
	switch d {
	case decisionDeny:
		return 3
	case decisionAsk:
		return 2
	case decisionAllow:
		return 0
	default:
		return 1
	}
}

// evaluateStmt evaluates a single statement from the AST.
// Returns nil if rejected.
func evaluateStmt(stmt *syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	var redirOverrides []*result
	for _, redir := range stmt.Redirs {
		if redir.Word != nil {
			if r, prop := substitutionPropagate(redir.Word, ctx, wrapperPats, commandPats); prop {
				return r
			}
		}
		if redir.Hdoc != nil {
			if r, prop := substitutionPropagate(redir.Hdoc, ctx, wrapperPats, commandPats); prop {
				return r
			}
		}
		if r := validateRedirect(redir, ctx); r != nil {
			redirOverrides = append(redirOverrides, r)
		}
	}

	if stmt.Cmd == nil {
		return nil
	}

	cmdR := evaluateCommand(stmt.Cmd, ctx, wrapperPats, commandPats)
	if cmdR == nil {
		return nil
	}
	if len(redirOverrides) > 0 {
		cmdR.decision, cmdR.denyReason = mergeAllDecisions(cmdR.decision, cmdR.denyReason, redirOverrides)
	}
	return cmdR
}

// substitutionPropagate adapts checkSubstitutions for callers that follow
// the "reject or propagate or continue" pattern. Returns (r, true) if the
// caller should return r (either nil for unrecognized, or a non-allow
// result to propagate). Returns (nil, false) when every substitution is
// allow and the caller can continue with its normal matching.
func substitutionPropagate(node syntax.Node, ctx evalContext, wrapperPats, commandPats []pattern) (*result, bool) {
	r := checkSubstitutions(node, ctx, wrapperPats, commandPats)
	if r == nil {
		return nil, true
	}
	if r.decision != decisionAllow {
		return r, true
	}
	return nil, false
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
		return evaluateDeclClause(c, ctx, wrapperPats, commandPats)
	case *syntax.ForClause:
		if r, prop := checkForLoop(c.Loop, ctx, wrapperPats, commandPats); prop {
			return r
		}
		return evaluateBlock(c.Do, ctx, wrapperPats, commandPats, "for")
	case *syntax.WhileClause:
		for _, stmt := range c.Cond {
			r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
			if r == nil {
				return nil
			}
			if r.decision != decisionAllow {
				return r
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

// evaluateIfClause checks all branches of an if/elif/else. Cond, then,
// and else decisions all merge with severity priority so a deny/ask in
// any branch surfaces in the result instead of being hidden by an
// allowed then.
func evaluateIfClause(ic *syntax.IfClause, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	for _, stmt := range ic.Cond {
		r := evaluateStmt(stmt, ctx, wrapperPats, commandPats)
		if r == nil {
			return nil
		}
		if r.decision != decisionAllow {
			return r
		}
	}
	thenR := evaluateBlock(ic.Then, ctx, wrapperPats, commandPats, "if")
	if thenR == nil {
		return nil
	}
	if ic.Else != nil {
		elseR := evaluateCommand(ic.Else, ctx, wrapperPats, commandPats)
		if elseR == nil {
			return nil
		}
		thenR.decision, thenR.denyReason = mergeAllDecisions(thenR.decision, thenR.denyReason, []*result{elseR})
	}
	return thenR
}

// evaluateDeclClause validates `export`, `declare`, `local`, `readonly`,
// `typeset` (mvdan does not parse `unset` as a DeclClause). Variable
// names go through validateEnvVarNames so hard-deny vars (LD_PRELOAD,
// BASH_ENV, ...) and ask-exact vars (PATH, NODE_OPTIONS, ...) surface
// their decisions instead of getting blanket approval. Substitutions in
// any assignment value (e.g. `export FOO=$(rm -rf /)`) are checked
// separately via the standard substitutionPropagate path.
func evaluateDeclClause(c *syntax.DeclClause, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	if r, prop := substitutionPropagate(c, ctx, wrapperPats, commandPats); prop {
		return r
	}
	names := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		if a.Name != nil {
			names = append(names, a.Name.Value)
		}
	}
	out := approved("shell vars")
	if r := validateEnvVarNames(names, ctx); r != nil {
		out.decision = r.decision
		out.denyReason = r.denyReason
	}
	return out
}

// checkForLoop validates the iteration/condition of a for loop.
// Returns (r, true) when the outer command should be replaced by r —
// either nil for unrecognized substitutions or a non-allow result to
// propagate. Returns (nil, false) when every substitution is allow.
func checkForLoop(loop syntax.Loop, ctx evalContext, wrapperPats, commandPats []pattern) (*result, bool) {
	switch l := loop.(type) {
	case *syntax.WordIter:
		for _, word := range l.Items {
			if r, prop := substitutionPropagate(word, ctx, wrapperPats, commandPats); prop {
				return r, true
			}
		}
	case *syntax.CStyleLoop:
		for _, expr := range []syntax.ArithmExpr{l.Init, l.Cond, l.Post} {
			if expr != nil {
				if r, prop := substitutionPropagate(expr, ctx, wrapperPats, commandPats); prop {
					return r, true
				}
			}
		}
	}
	return nil, false
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
	// Standalone variable assignment: FOO=bar (no command). Apply only
	// the dangerous-name guard (hard-deny / ask-exact / LD_-DYLD_) so
	// `LD_PRELOAD=/x; cat foo` still surfaces the deny, but ordinary
	// project locals like `hm_src=/path` auto-approve instead of asking
	// on every unknown name.
	if len(call.Args) == 0 && len(call.Assigns) > 0 {
		if r, prop := substitutionPropagate(call, ctx, wrapperPats, commandPats); prop {
			return r
		}
		names := make([]string, 0, len(call.Assigns))
		for _, a := range call.Assigns {
			if a.Name != nil {
				names = append(names, a.Name.Value)
			}
		}
		out := approved("var assignment")
		if r := validateStandaloneAssignNames(names); r != nil {
			out.decision = r.decision
			out.denyReason = r.denyReason
		}
		return out
	}

	if len(call.Args) == 0 {
		return nil
	}

	// Resolve command name. Allow $(which X) / $(command -v X) / $(...)/path/to/cmd.
	cmdName := wordLiteral(call.Args[0])
	if cmdName == "" {
		resolved, knownSafe := resolveWhichSubst(call.Args[0])
		if resolved == "" {
			// Dynamic command name we can't match. A deny/ask inside
			// the substitution should still propagate so the user sees
			// the inner command's prompt or block.
			if r, prop := substitutionPropagate(call, ctx, wrapperPats, commandPats); prop {
				return r
			}
			return nil
		}
		if knownSafe {
			// $(which X) / $(command -v X) — inherently safe, only check remaining args
			for _, arg := range call.Args[1:] {
				if r, prop := substitutionPropagate(arg, ctx, wrapperPats, commandPats); prop {
					return r
				}
			}
		} else {
			// $(...)/path/to/cmd — must verify the inner substitution is safe too
			for _, arg := range call.Args {
				if r, prop := substitutionPropagate(arg, ctx, wrapperPats, commandPats); prop {
					return r
				}
			}
		}
		return matchAndBuild(resolved, call.Args[1:], call.Assigns, call.Args, ctx, wrapperPats, commandPats)
	}

	// Normal path: check all substitutions
	if r, prop := substitutionPropagate(call, ctx, wrapperPats, commandPats); prop {
		return r
	}

	return matchAndBuild(argsText(call.Args), nil, call.Assigns, call.Args, ctx, wrapperPats, commandPats)
}

// matchAndBuild strips wrappers, matches the command, and builds the result.
// cmdText is the primary command text. extraArgs are appended if non-nil.
// astArgs are the raw AST arguments passed to pattern validators.
//
// Multiple decision sources can override the matched pattern's baseline:
//   - the per-pattern argsValidator (false downgrades to validateFallback)
//   - each wrapper's validateWrapper (returns *result to override)
//   - validateEnvVarNames over call.Assigns (handles command-leading
//     `LD_PRELOAD=/x cat foo` even when the env-vars wrapper regex did
//     not match because another wrapper sat in front)
//
// All overrides merge with the matched decision via mergeAllDecisions
// (deny > ask > "" > allow).
func matchAndBuild(cmdText string, extraArgs []*syntax.Word, assigns []*syntax.Assign, astArgs []*syntax.Word, ctx evalContext, wrapperPats, commandPats []pattern) *result {
	if len(extraArgs) > 0 {
		cmdText += " " + argsText(extraArgs)
	}
	coreCmd, wrappers := stripWrappers(cmdText, wrapperPats)
	matched := matchCommand(coreCmd, commandPats)
	if matched == nil {
		return nil
	}
	var overrides []*result
	if matched.validate != nil && !matched.validate(astArgs, ctx) {
		overrides = append(overrides, &result{decision: matched.validateFallback})
	}
	for _, wm := range wrappers {
		if wm.pattern.validateWrapper == nil {
			continue
		}
		if r := wm.pattern.validateWrapper(wm.text, ctx); r != nil {
			overrides = append(overrides, r)
		}
	}
	if len(assigns) > 0 {
		names := make([]string, 0, len(assigns))
		for _, a := range assigns {
			if a.Name != nil {
				names = append(names, a.Name.Value)
			}
		}
		if r := validateEnvVarNames(names, ctx); r != nil {
			overrides = append(overrides, r)
		}
	}
	wrapperLabels := make([]string, 0, len(wrappers))
	for _, wm := range wrappers {
		wrapperLabels = append(wrapperLabels, wm.pattern.label())
	}
	if len(assigns) > 0 && !slices.Contains(wrapperLabels, tagEnvVars) {
		wrapperLabels = append([]string{tagEnvVars}, wrapperLabels...)
	}
	reason := matched.label()
	if len(wrapperLabels) > 0 {
		reason = strings.Join(wrapperLabels, "+") + "+" + reason
	}
	finalDecision, finalDenyReason := mergeAllDecisions(matched.decision, matched.denyReason, overrides)
	return &result{reason: reason, decision: finalDecision, denyReason: finalDenyReason}
}

// mergeAllDecisions combines the matched pattern's baseline decision with
// any number of validator overrides using priority deny > ask > "" >
// allow. Strict `>` means the matched pattern's denyReason wins on ties.
func mergeAllDecisions(matchedDecision, matchedDenyReason string, overrides []*result) (string, string) {
	worstDecision := matchedDecision
	worstDenyReason := matchedDenyReason
	for _, o := range overrides {
		if decisionRank(o.decision) > decisionRank(worstDecision) {
			worstDecision = o.decision
			worstDenyReason = o.denyReason
		}
	}
	return worstDecision, worstDenyReason
}

// wordLiteral returns the shell-decoded literal value of a word, or "" if the
// word contains dynamic expansions (parameter expansion, command substitution,
// arithmetic). It strips unquoted backslash escapes, decodes ANSI-C $'...'
// sequences, and unwraps single and double quotes — matching what the shell
// would produce in argv.
func wordLiteral(w *syntax.Word) string {
	s, ok := wordDecodedLiteral(w)
	if !ok {
		return ""
	}
	return s
}

// wordLiteralPath is wordLiteral plus a tilde guard: bash expands a
// leading unquoted `~` / `~user` to $HOME / a user's homedir at runtime,
// so the validator must ask instead of comparing the literal text against
// the repo. Use this for redirect / tee / similar path validators.
func wordLiteralPath(w *syntax.Word) string {
	if w != nil && len(w.Parts) > 0 {
		if lit, ok := w.Parts[0].(*syntax.Lit); ok && hasUnquotedTildePrefix(lit.Value) {
			return ""
		}
	}
	return wordLiteral(w)
}

// hasUnquotedTildePrefix reports whether s starts with `~` (no
// backslash-escape) — `~`, `~/...`, `~user`, `~user/...`, `~+`, and
// `~-` all expand at runtime to $HOME / oldpwd / cwd / a user's homedir,
// none of which the validator can statically prove.
func hasUnquotedTildePrefix(s string) bool {
	return strings.HasPrefix(s, "~")
}

// hasUnquotedGlob reports whether the word contains an unescaped glob
// metacharacter (`*`, `?`, `[`) in an unquoted Lit part. Bash expands
// these before the redirect, so an unquoted `out*` could resolve to a
// different file (potentially a symlink leaving the repo) than the
// literal text suggests. Quoted `'*'` / `"*"` and backslash-escaped
// metacharacters (`out\*`) stay literal and do not trigger this guard.
func hasUnquotedGlob(w *syntax.Word) bool {
	for _, part := range w.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			if litHasUnescapedGlob(lit.Value) {
				return true
			}
		}
	}
	return false
}

// litHasUnescapedGlob walks s and returns true on the first glob
// metacharacter not preceded by a backslash. mvdan keeps the backslash
// in Lit.Value for unquoted backslash escapes, so the scan is sufficient.
func litHasUnescapedGlob(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			continue
		}
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}

// safeRedirectDevices are device targets where a write redirect is
// harmless — these are pseudo-devices, not real files.
var safeRedirectDevices = map[string]bool{
	"/dev/null":   true,
	"/dev/stdout": true,
	"/dev/stderr": true,
	"/dev/tty":    true,
}

// safeWritePrefixes are absolute path prefixes where a write redirect
// auto-approves. System scratch directories where stray writes don't
// hurt anything important. The lstat check below catches pre-existing
// symlinks that would escape these prefixes.
var safeWritePrefixes = []string{
	"/tmp/",
	"/var/tmp/",
}

// isSafeWriteTarget reports whether target is under a safe write
// prefix. A pre-existing symlink at the target path that resolves
// outside any safe prefix is refused (mitigates `/tmp/X → /etc/passwd`
// planted-symlink attacks). Non-existent targets pass: there's no
// symlink to resolve and the parent is by definition safe.
//
// This does not close the same-invocation TOCTOU window where an
// earlier approved stmt creates the symlink between hook validation
// and shell execution; that gap exists for in-repo writes too and
// would have to be closed by validating `ln -s` separately.
func isSafeWriteTarget(target string) bool {
	if !filepath.IsAbs(target) {
		return false
	}
	cleaned := filepath.Clean(target)
	withSlash := cleaned + "/"
	matched := false
	for _, p := range safeWritePrefixes {
		if strings.HasPrefix(withSlash, p) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return true
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return true
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return false
	}
	resolvedSlash := filepath.Clean(resolved) + "/"
	for _, p := range safeWritePrefixes {
		if strings.HasPrefix(resolvedSlash, p) {
			return true
		}
	}
	return false
}

// validateRedirect inspects a redirect operator/target and asks when a
// write/append redirect would clobber a file outside the current repo.
// Read redirects (`<`, heredoc, here-string) and fd-duplications (`<&N`,
// `>&N`, `>&-`) do not write a file path and pass through. The bare
// `>&FILE` form is handled specially: bash treats it as a write redirect
// (equivalent to `&>FILE`) when the target is not a number or `-`.
// Non-literal write targets (variable, substitution) and unquoted glob
// patterns drop to ask defensively because we cannot statically prove
// where the bytes land.
func validateRedirect(redir *syntax.Redirect, ctx evalContext) *result {
	if !isWriteRedirect(redir.Op) && !isDplOutWriteToFile(redir) {
		return nil
	}
	if redir.Word == nil {
		return nil
	}
	if hasUnquotedGlob(redir.Word) {
		return &result{decision: decisionAsk}
	}
	target := wordLiteralPath(redir.Word)
	if target == "" {
		return &result{decision: decisionAsk}
	}
	if safeRedirectDevices[target] {
		return nil
	}
	if isSafeWriteTarget(target) {
		return nil
	}
	if !teeTargetInRepo(ctx.cwd, target) {
		return &result{decision: decisionAsk}
	}
	return nil
}

// isDplOutWriteToFile reports whether `>&` (DplOut) is being used as the
// bare write-to-file form (equivalent to `&>file`) rather than fd
// duplication. `>&N` (number) and `>&-` (close) are fd-dup; anything
// else is a filename.
func isDplOutWriteToFile(redir *syntax.Redirect) bool {
	if redir.Op != syntax.DplOut || redir.Word == nil {
		return false
	}
	target := wordLiteral(redir.Word)
	if target == "" {
		// Dynamic word: defensively treat as a write redirect so the
		// caller can ask.
		return true
	}
	if target == "-" {
		return false
	}
	for i := 0; i < len(target); i++ {
		if target[i] < '0' || target[i] > '9' {
			return true
		}
	}
	return false
}

// isWriteRedirect reports whether the operator opens or modifies a file.
// `<>` (RdrInOut) is included because it opens for read+write.
func isWriteRedirect(op syntax.RedirOperator) bool {
	switch op {
	case syntax.RdrOut, syntax.AppOut, syntax.ClbOut,
		syntax.RdrAll, syntax.AppAll, syntax.RdrInOut:
		return true
	}
	return false
}

// wordDecodedLiteral returns the shell-decoded literal of a word. ok is false
// when the word contains parts that can't be decoded statically (unknown
// command substitutions, ParamExp, ArithmExp), in which case s is the empty
// string. A small set of pure-string command substitutions (echo) is decoded
// statically so common bypass attempts via $(echo -rf) get caught.
func wordDecodedLiteral(w *syntax.Word) (s string, ok bool) {
	if w == nil {
		return "", true
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(stripUnquotedBackslashes(p.Value))
		case *syntax.SglQuoted:
			if p.Dollar {
				decoded, err := expand.Literal(nil, &syntax.Word{Parts: []syntax.WordPart{p}})
				if err != nil {
					return "", false
				}
				sb.WriteString(decoded)
			} else {
				sb.WriteString(p.Value)
			}
		case *syntax.DblQuoted:
			if !dblQuotedDecodable(p) {
				return "", false
			}
			decoded, err := expand.Literal(literalCfg, &syntax.Word{Parts: []syntax.WordPart{p}})
			if err != nil {
				return "", false
			}
			sb.WriteString(decoded)
		case *syntax.CmdSubst:
			val, ok := tryEvalCmdSubst(p)
			if !ok {
				return "", false
			}
			sb.WriteString(val)
		default:
			return "", false
		}
	}
	return sb.String(), true
}

// errUnhandledCmdSubst signals that a command substitution can't be evaluated
// statically. Used inside literalCfg.CmdSubst so expand.Literal aborts the
// expansion of the enclosing word.
var errUnhandledCmdSubst = errors.New("unhandled cmd subst")

// literalCfg is the expand.Config used inside double-quoted contexts. It
// delegates command substitutions to tryEvalCmdSubst so $(echo ...) inside
// quotes decodes the same way as outside. Built in init() to break the
// initialization cycle between the closure and tryEvalCmdSubst.
var literalCfg *expand.Config

func init() {
	literalCfg = &expand.Config{
		CmdSubst: func(w io.Writer, cs *syntax.CmdSubst) error {
			val, ok := tryEvalCmdSubst(cs)
			if !ok {
				return errUnhandledCmdSubst
			}
			_, err := w.Write([]byte(val))
			return err
		},
	}
}

// tryEvalCmdSubst statically evaluates a command substitution that consists
// of a single, redirect-free call to a known pure-string command. Returns
// ("", false) for anything else (fail-closed: unknown substitutions never
// produce statically-decoded text; the caller falls back to the printed
// source so substitutions stay opaque to regex matching).
//
// Output is returned as a single string with args space-joined. We do not
// model bash IFS word splitting separately because the matching pipeline
// regex-matches against a space-joined argv anyway — the deny patterns are
// prefix matchers (e.g. `^rm\s+-r`), so a word boundary in the substitution
// output is indistinguishable from a literal space at the regex level.
func tryEvalCmdSubst(cs *syntax.CmdSubst) (string, bool) {
	if cs == nil || len(cs.Stmts) != 1 {
		return "", false
	}
	stmt := cs.Stmts[0]
	if len(stmt.Redirs) > 0 || stmt.Cmd == nil {
		return "", false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return "", false
	}
	name, ok := wordDecodedLiteral(call.Args[0])
	if !ok {
		return "", false
	}
	switch name {
	case "echo":
		return evalEchoArgs(call.Args[1:])
	}
	return "", false
}

// evalEchoArgs returns echo's stdout as bash would emit it, trimmed of the
// trailing newline that command substitution would strip anyway. Leading
// flag-only args matching -[neE]+ are consumed; -e enables backslash escape
// interpretation, -E disables it (default), with the latest setting winning.
// Once a non-flag arg appears, remaining args (including dash-prefixed ones)
// are kept literally, with bash's echo backslash escapes decoded when -e is
// active.
func evalEchoArgs(args []*syntax.Word) (string, bool) {
	skipFlags := true
	decodeEscapes := false
	parts := make([]string, 0, len(args))
	for _, a := range args {
		s, ok := wordDecodedLiteral(a)
		if !ok {
			return "", false
		}
		if skipFlags && isEchoFlag(s) {
			for i := 1; i < len(s); i++ {
				switch s[i] {
				case 'e':
					decodeEscapes = true
				case 'E':
					decodeEscapes = false
				}
			}
			continue
		}
		skipFlags = false
		if decodeEscapes {
			decoded, suppress := decodeEchoBackslash(s)
			parts = append(parts, decoded)
			if suppress {
				break
			}
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " "), true
}

// isEchoFlag reports whether s is a leading echo option (-n, -e, -E, or any
// concatenation thereof). bash's builtin echo treats anything else literally.
func isEchoFlag(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case 'n', 'e', 'E':
		default:
			return false
		}
	}
	return true
}

// decodeEchoBackslash applies bash `echo -e` backslash escape rules to s.
// Recognized: \\, \a, \b, \c (suppress remaining), \e, \E, \f, \n, \r, \t,
// \v, \0NNN (zero plus 0-3 octal digits), \xHH (1-2 hex digits). Unicode
// escapes (\u, \U) and unrecognized escapes pass through unchanged — they
// can't produce ASCII flag characters that would change matching. The
// suppress return is true when \c was encountered; the caller must stop
// emitting further arguments, since bash's echo \c truncates the entire
// output stream, not just the current argument.
func decodeEchoBackslash(s string) (out string, suppress bool) {
	if !strings.Contains(s, "\\") {
		return s, false
	}
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			i++
			continue
		}
		switch s[i+1] {
		case '\\':
			sb.WriteByte('\\')
			i += 2
		case 'a':
			sb.WriteByte(0x07)
			i += 2
		case 'b':
			sb.WriteByte(0x08)
			i += 2
		case 'c':
			return sb.String(), true
		case 'e', 'E':
			sb.WriteByte(0x1b)
			i += 2
		case 'f':
			sb.WriteByte(0x0c)
			i += 2
		case 'n':
			sb.WriteByte(0x0a)
			i += 2
		case 'r':
			sb.WriteByte(0x0d)
			i += 2
		case 't':
			sb.WriteByte(0x09)
			i += 2
		case 'v':
			sb.WriteByte(0x0b)
			i += 2
		case '0':
			v, n := 0, 0
			for n < 3 && i+2+n < len(s) && s[i+2+n] >= '0' && s[i+2+n] <= '7' {
				v = v*8 + int(s[i+2+n]-'0')
				n++
			}
			sb.WriteByte(byte(v))
			i += 2 + n
		case 'x':
			v, n := 0, 0
			for n < 2 && i+2+n < len(s) {
				d := hexDigit(s[i+2+n])
				if d < 0 {
					break
				}
				v = v*16 + d
				n++
			}
			if n == 0 {
				sb.WriteByte('\\')
				sb.WriteByte('x')
				i += 2
			} else {
				sb.WriteByte(byte(v))
				i += 2 + n
			}
		default:
			sb.WriteByte('\\')
			sb.WriteByte(s[i+1])
			i += 2
		}
	}
	return sb.String(), false
}

func hexDigit(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	}
	return -1
}

// dblQuotedDecodable reports whether all parts inside a *syntax.DblQuoted
// node can be resolved statically: Lit (text with backslash escapes) and
// CmdSubst (handled via literalCfg's CmdSubst handler). ParamExp and
// ArithmExp would be evaluated against an empty environment by
// expand.Literal, silently producing matches based on default values
// instead of the real runtime — so we fall back to the printed source.
func dblQuotedDecodable(d *syntax.DblQuoted) bool {
	for _, part := range d.Parts {
		switch part.(type) {
		case *syntax.Lit, *syntax.CmdSubst:
			// resolvable
		default:
			return false
		}
	}
	return true
}

// stripUnquotedBackslashes drops backslash escapes from an unquoted shell
// literal: "\X" becomes "X". The mvdan/sh parser already removes line
// continuations, so we don't need to handle "\<newline>" here.
func stripUnquotedBackslashes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		sb.WriteByte(s[i])
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

	ctx.wrapperPats = wrapperPats
	ctx.commandPats = commandPats
	return mergeStmtResults(file.Stmts, ctx, wrapperPats, commandPats)
}

func evaluatePiToolUse(input PiInput, cfg Config) (string, *result, error) {
	ctx := evalContext{cwd: input.Cwd}
	if input.Cwd == "" {
		return "", nil, fmt.Errorf("cwd is required")
	}

	switch input.Tool {
	case "bash":
		if input.Command == "" {
			return "", nil, fmt.Errorf("command is required")
		}
		return input.Command, Evaluate(input.Command, cfg, ctx), nil
	case "read":
		if input.Path == "" {
			return "", nil, fmt.Errorf("path is required")
		}
		return input.Path, evaluateReadTool(ReadInput{FilePath: input.Path}, ctx), nil
	case "grep":
		grepInput := GrepInput{Pattern: input.Pattern, Path: input.Path, Paths: input.Paths}
		return grepCommandLabel(grepInput), evaluateGrepTool(grepInput, ctx), nil
	case "find":
		return input.Path, evaluateFindTool(input, ctx), nil
	case "ls":
		return input.Path, evaluateLsTool(input, ctx), nil
	default:
		return "", nil, fmt.Errorf("unsupported tool: %s", input.Tool)
	}
}

func buildPiOutput(tool string, r *result) PiOutput {
	if r == nil {
		return PiOutput{Version: 1, Kind: "decision", Tool: tool, Decision: "noop"}
	}
	reason := r.reason
	if r.decision == decisionDeny && r.denyReason != "" {
		reason = r.denyReason
	}
	decision := r.decision
	if decision == "" {
		decision = "noop"
	}
	return PiOutput{Version: 1, Kind: "decision", Tool: tool, Decision: decision, Reason: reason}
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

type cliMode string

const (
	modeHook     cliMode = "hook"
	modeOpenCode cliMode = "opencode"
	modePi       cliMode = "pi"
	modeCodex    cliMode = "codex"
)

type cliOptions struct {
	mode       cliMode
	configPath string
}

func telemetryAgentForMode(mode cliMode) string {
	switch mode {
	case modePi:
		return "pi"
	case modeOpenCode:
		return "opencode"
	case modeCodex:
		return "codex"
	default:
		return "claude"
	}
}

func parseCLIOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("approve-bash", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var piMode bool
	var openCodeMode bool
	var codexMode bool
	var configPath string
	fs.BoolVar(&piMode, "pi", false, "emit pi runtime JSON output")
	fs.BoolVar(&openCodeMode, "opencode", false, "emit OpenCode JSON output")
	fs.BoolVar(&codexMode, "codex", false, "emit Codex JSON output")
	fs.StringVar(&configPath, "config", "", "path to categories config")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	mode := modeHook
	selectedModes := 0
	if piMode {
		selectedModes++
		mode = modePi
	}
	if openCodeMode {
		selectedModes++
		mode = modeOpenCode
	}
	if codexMode {
		selectedModes++
		mode = modeCodex
	}
	if selectedModes > 1 {
		return cliOptions{}, fmt.Errorf("cannot combine multiple output modes")
	}
	return cliOptions{mode: mode, configPath: configPath}, nil
}

func main() {
	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}

	opts, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		os.Exit(0)
	}

	db := openTelemetryDB()
	if db != nil {
		defer db.Close() //nolint:errcheck // best-effort telemetry
	}

	payload := string(rawInput)
	agent := telemetryAgentForMode(opts.mode)

	var cfg Config
	if opts.configPath != "" {
		cfg, err = loadExplicitConfigFromPath(opts.configPath)
	} else {
		cfg, err = loadConfig()
	}
	if err != nil {
		logDecision(db, agent, payload, "", decisionDeny, err.Error())
		if opts.mode == modePi {
			emitPiOutput(buildPiErrorOutput("config-error", err.Error()))
		}
		if opts.mode == modeOpenCode {
			emitOpenCodeOutput(OpenCodeOutput{Decision: decisionDeny, Reason: err.Error()})
		}
		if opts.mode == modeCodex {
			emitCodexOutput(buildCodexOutput(&result{decision: decisionDeny, denyReason: err.Error()}))
		}
		emitDecision(decisionDeny, err.Error())
	}

	if opts.mode == modePi {
		var data PiInput
		if err := json.Unmarshal(rawInput, &data); err != nil {
			logDecision(db, agent, payload, "", "error", "invalid pi input")
			emitPiOutput(buildPiErrorOutput("invalid-input", "invalid pi input"))
		}

		cmd, r, err := evaluatePiToolUse(data, cfg)
		if err != nil {
			code := "invalid-input"
			if strings.HasPrefix(err.Error(), "unsupported tool:") {
				code = "unsupported-tool"
			}
			logDecision(db, agent, payload, cmd, "error", err.Error())
			emitPiOutput(buildPiErrorOutput(code, err.Error()))
		}
		out := buildPiOutput(data.Tool, r)
		logDecision(db, agent, payload, cmd, out.Decision, out.Reason)
		emitPiOutput(out)
	}

	if opts.mode == modeOpenCode {
		var data OpenCodeInput
		if err := json.Unmarshal(rawInput, &data); err != nil {
			logDecision(db, agent, payload, "", "noop", "")
			emitOpenCodeOutput(OpenCodeOutput{Decision: "noop"})
		}

		cmd, r := evaluateOpenCodeToolUse(data, cfg)
		out := buildOpenCodeOutput(r)
		logDecision(db, agent, payload, cmd, out.Decision, out.Reason)
		emitOpenCodeOutput(out)
	}

	if opts.mode == modeCodex {
		var data CodexInput
		if err := json.Unmarshal(rawInput, &data); err != nil {
			logDecision(db, agent, payload, "", "noop", "")
			emitCodexOutput(CodexOutput{Continue: true})
		}

		cmd, r := evaluateCodexToolUse(data, cfg)
		out := buildCodexOutput(r)
		decision := "noop"
		reason := ""
		if out.HookSpecificOutput != nil && out.HookSpecificOutput.Decision != nil {
			decision = out.HookSpecificOutput.Decision.Behavior
			reason = out.HookSpecificOutput.Decision.Reason
		}
		logDecision(db, agent, payload, cmd, decision, reason)
		emitCodexOutput(out)
	}

	var data HookInput
	if err := json.Unmarshal(rawInput, &data); err != nil {
		os.Exit(0)
	}

	cmd, r := evaluateToolUse(data, cfg)
	if r == nil {
		logDecision(db, agent, payload, cmd, "no-opinion", "")
		os.Exit(0)
	}
	// No-opinion: recognized command but no decision — exit silently, next hook handles it.
	if r.decision == "" {
		logDecision(db, agent, payload, cmd, "no-opinion", r.reason)
		os.Exit(0)
	}
	if r.decision == decisionAsk {
		logDecision(db, agent, payload, cmd, "ask", r.reason)
		emitDecision(decisionAsk, r.reason)
	}

	reason := r.reason
	if r.decision == decisionDeny && r.denyReason != "" {
		reason = r.denyReason
	}
	logDecision(db, agent, payload, cmd, r.decision, reason)
	emitDecision(r.decision, reason)
}
