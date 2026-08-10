package main

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/syntax"
)

var guardedHarnessEnvSuffixes = map[string]string{
	"HOME":            "",
	"XDG_CONFIG_HOME": "/config",
	"XDG_DATA_HOME":   "/data",
	"XDG_CACHE_HOME":  "/cache",
	"XDG_STATE_HOME":  "/state",
}

func evaluateGuardedTestHarness(
	stmts []*syntax.Stmt,
	ctx evalContext,
	wrapperPats, commandPats []pattern,
) (*result, bool) {
	if r, ok := evaluateGuardedKnownTempCleanup(stmts, ctx); ok {
		return r, true
	}

	for start := 0; start+3 < len(stmts); start++ {
		tempVar, ok := guardedTempAssignment(stmts[start], ctx)
		if !ok || !guardedTempCase(stmts[start+1], tempVar) ||
			!guardedTempChmod(stmts[start+2], tempVar, ctx) {
			continue
		}

		trapIndex := start + 3
		for trapIndex < len(stmts) && guardedTempMkdir(stmts[trapIndex], tempVar, ctx) {
			trapIndex++
		}
		if trapIndex >= len(stmts) || !guardedTempTrap(stmts[trapIndex], tempVar, ctx) {
			continue
		}

		results := make([]*result, 0, len(stmts)-trapIndex+2)
		localCtx := ctx
		valid := true
		for _, stmt := range stmts[:start] {
			if guardedStmtTouchesTempVar(stmt, tempVar) {
				valid = false
				break
			}
			r := evaluateStmt(stmt, localCtx, wrapperPats, commandPats)
			if r == nil {
				valid = false
				break
			}
			results = append(results, r)
			if r.decision == decisionAllow {
				recordStandaloneAssignments(stmt, &localCtx)
			}
		}
		if !valid {
			continue
		}

		results = append(results, approved("isolated test harness"))
		runnerCount := 0
		for _, stmt := range stmts[trapIndex+1:] {
			if r, matched := guardedTempEnvRunner(stmt, tempVar, localCtx, wrapperPats, commandPats); matched {
				if r == nil {
					valid = false
					break
				}
				results = append(results, r)
				runnerCount++
				continue
			}
			if guardedStmtTouchesTempVar(stmt, tempVar) {
				valid = false
				break
			}

			r := evaluateStmt(stmt, localCtx, wrapperPats, commandPats)
			if r == nil {
				valid = false
				break
			}
			results = append(results, r)
			if r.decision == decisionAllow {
				recordStandaloneAssignments(stmt, &localCtx)
			}
		}
		if valid && runnerCount > 0 {
			return mergeGuardedHarnessResults(results), true
		}
	}
	return nil, false
}

func guardedStmtTouchesTempVar(stmt *syntax.Stmt, tempVar string) bool {
	touches := false
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if touches {
			return false
		}
		switch n := node.(type) {
		case *syntax.DeclClause:
			touches = true
		case *syntax.Assign:
			touches = n.Name != nil && n.Name.Value == tempVar
		case *syntax.WordIter:
			touches = n.Name != nil && n.Name.Value == tempVar
		case *syntax.ParamExp:
			touches = n.Param != nil && n.Param.Value == tempVar
		case *syntax.CallExpr:
			if len(n.Args) == 0 {
				return true
			}
			command := wordLiteral(n.Args[0])
			if command == "command" || command == "eval" || command == "source" || command == "." {
				touches = true
				return false
			}
			if command == "unset" || command == "read" {
				for _, arg := range n.Args[1:] {
					if wordLiteral(arg) == tempVar {
						touches = true
						return false
					}
				}
			}
			if command == "printf" {
				for i := 1; i+1 < len(n.Args); i++ {
					if wordLiteral(n.Args[i]) == "-v" && wordLiteral(n.Args[i+1]) == tempVar {
						touches = true
						return false
					}
				}
			}
		}
		return !touches
	})
	return touches
}

func guardedTempAssignment(stmt *syntax.Stmt, ctx evalContext) (string, bool) {
	if !guardedPlainStmt(stmt) {
		return "", false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 0 || len(call.Assigns) != 1 {
		return "", false
	}
	assign := call.Assigns[0]
	if assign == nil || assign.Append || assign.Naked || assign.Name == nil ||
		assign.Index != nil || assign.Array != nil || assign.Value == nil ||
		len(assign.Value.Parts) != 1 {
		return "", false
	}
	valuePart := assign.Value.Parts[0]
	if quoted, quotedOK := valuePart.(*syntax.DblQuoted); quotedOK && !quoted.Dollar && len(quoted.Parts) == 1 {
		valuePart = quoted.Parts[0]
	}
	subst, ok := valuePart.(*syntax.CmdSubst)
	if !ok || subst.Backquotes || subst.TempFile || subst.ReplyVar || len(subst.Stmts) != 1 {
		return "", false
	}
	inner, ok := guardedPlainCall(subst.Stmts[0])
	if !ok || len(inner.Assigns) != 0 || len(inner.Args) != 4 {
		return "", false
	}
	args, ok := guardedLiteralArgs(inner.Args)
	if !ok || !guardedCommandName(args[0], "mktemp", ctx) || args[1] != "-d" || args[2] != "-t" ||
		!guardedTempTemplate(args[3]) {
		return "", false
	}
	return assign.Name.Value, true
}

func guardedTempTemplate(value string) bool {
	const suffix = ".XXXXXX"
	if !strings.HasSuffix(value, suffix) || filepath.Base(value) != value {
		return false
	}
	prefix := strings.TrimSuffix(value, suffix)
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-", r) {
			return false
		}
	}
	return true
}

func guardedTempCase(stmt *syntax.Stmt, tempVar string) bool {
	if !guardedPlainStmt(stmt) {
		return false
	}
	clause, ok := stmt.Cmd.(*syntax.CaseClause)
	if !ok || clause.Braces || !guardedQuotedVarWord(clause.Word, tempVar, "") ||
		len(clause.Items) != 2 {
		return false
	}

	accepted := clause.Items[0]
	if accepted == nil || accepted.Op.String() != ";;" || len(accepted.Stmts) != 0 ||
		len(accepted.Patterns) != 3 {
		return false
	}
	want := map[string]bool{"/tmp/*": true, "/private/tmp/*": true, "/var/folders/*": true}
	for _, pattern := range accepted.Patterns {
		value, ok := wordDecodedLiteral(pattern)
		if !ok || !want[value] {
			return false
		}
		delete(want, value)
	}
	if len(want) != 0 {
		return false
	}

	fallback := clause.Items[1]
	if fallback == nil || fallback.Op.String() != ";;" || len(fallback.Patterns) != 1 ||
		wordLiteral(fallback.Patterns[0]) != "*" || len(fallback.Stmts) != 1 {
		return false
	}
	exitCall, ok := guardedPlainCall(fallback.Stmts[0])
	if !ok || len(exitCall.Assigns) != 0 || len(exitCall.Args) != 2 ||
		wordLiteral(exitCall.Args[0]) != "exit" {
		return false
	}
	status, err := strconv.Atoi(wordLiteral(exitCall.Args[1]))
	return err == nil && status > 0 && status <= 255
}

func guardedTempChmod(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 3 &&
		guardedCommandWord(call.Args[0], "chmod", ctx) &&
		wordLiteral(call.Args[1]) == "700" &&
		guardedQuotedVarWord(call.Args[2], tempVar, "")
}

func guardedTempTrap(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	if !ok || len(call.Assigns) != 0 || len(call.Args) != 3 ||
		!guardedCommandWord(call.Args[0], "trap", ctx) || wordLiteral(call.Args[2]) != "EXIT" {
		return false
	}
	payload, ok := wordDecodedLiteral(call.Args[1])
	if !ok {
		return false
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(payload), "")
	if err != nil {
		return false
	}
	if len(file.Stmts) == 1 {
		return guardedTempTrapRm(file.Stmts[0], tempVar, ctx)
	}
	if len(file.Stmts) != 3 {
		return false
	}
	statusVar, ok := guardedStatusAssignment(file.Stmts[0])
	return ok && statusVar != tempVar &&
		guardedTempTrapRm(file.Stmts[1], tempVar, ctx) &&
		guardedStatusExit(file.Stmts[2], statusVar)
}

func guardedStatusAssignment(stmt *syntax.Stmt) (string, bool) {
	if !guardedPlainStmt(stmt) {
		return "", false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 0 || len(call.Assigns) != 1 {
		return "", false
	}
	assign := call.Assigns[0]
	if assign == nil || assign.Append || assign.Naked || assign.Name == nil ||
		assign.Index != nil || assign.Array != nil || !guardedParamWord(assign.Value, "?") {
		return "", false
	}
	return assign.Name.Value, true
}

func guardedTempTrapRm(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 4 &&
		guardedCommandWord(call.Args[0], "rm", ctx) &&
		wordLiteral(call.Args[1]) == "-rf" &&
		wordLiteral(call.Args[2]) == "--" &&
		guardedQuotedVarWord(call.Args[3], tempVar, "")
}

func guardedStatusExit(stmt *syntax.Stmt, statusVar string) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 2 &&
		wordLiteral(call.Args[0]) == "exit" &&
		guardedParamWord(call.Args[1], statusVar)
}

func guardedTempEnvRunner(
	stmt *syntax.Stmt,
	tempVar string,
	ctx evalContext,
	wrapperPats, commandPats []pattern,
) (*result, bool) {
	call, ok := guardedPlainCall(stmt)
	if !ok || len(call.Assigns) != 0 || len(call.Args) < 2 ||
		!guardedCommandWord(call.Args[0], "env", ctx) {
		return nil, false
	}

	seen := make(map[string]bool, len(guardedHarnessEnvSuffixes)+4)
	var overrides []*result
	i := 1
	for i < len(call.Args) {
		literal := wordLiteral(call.Args[i])
		switch {
		case literal == "-u" || literal == "--unset":
			if i+1 >= len(call.Args) {
				return nil, false
			}
			name := wordLiteral(call.Args[i+1])
			if !syntax.ValidName(name) {
				return nil, false
			}
			i += 2
			continue
		case strings.HasPrefix(literal, "--unset="):
			if !syntax.ValidName(strings.TrimPrefix(literal, "--unset=")) {
				return nil, false
			}
			i++
			continue
		case literal == "--":
			i++
		}

		if i >= len(call.Args) {
			return nil, false
		}
		name, suffix, tempDerived, assignment, matched := guardedHarnessEnvAssignment(call.Args[i], tempVar, ctx)
		if !matched {
			break
		}
		if seen[name] {
			return nil, false
		}
		seen[name] = true
		if name == "PATH" {
			if !guardedHarnessPathAssignment(call.Args[i], ctx) {
				return nil, false
			}
		} else if want, required := guardedHarnessEnvSuffixes[name]; required {
			if !tempDerived || !guardedHarnessRequiredSuffix(name, suffix, want) {
				return nil, false
			}
		}
		if name != "PATH" {
			if tempDerived && suffix != "" && !guardedTempDescendantSuffix(suffix) {
				return nil, false
			}
			if override := guardedHarnessEnvOverride(assignment, tempDerived, suffix, ctx); override != nil {
				overrides = append(overrides, override)
			}
		}
		i++
	}

	for name := range guardedHarnessEnvSuffixes {
		if !seen[name] {
			return nil, false
		}
	}
	if i >= len(call.Args) {
		return nil, false
	}

	runnerWords := call.Args[i:]
	executable := wordLiteral(runnerWords[0])
	var r *result
	if filepath.IsAbs(executable) && filepath.Base(executable) == "node" {
		if isSafeAbsolutePath(filepath.Dir(executable)+string(filepath.Separator), ctx) != nil ||
			len(runnerWords) < 2 || wordLiteral(runnerWords[1]) != "../node_modules/vite-plus/bin/vp" {
			return nil, false
		}
		command := "vp"
		if len(runnerWords) > 2 {
			command += " " + argsText(runnerWords[2:])
		}
		r = evaluate(command, ctx, wrapperPats, commandPats)
	} else {
		r = evaluateCallExpr(&syntax.CallExpr{Args: runnerWords}, ctx, wrapperPats, commandPats)
	}
	if r == nil {
		return nil, true
	}
	r.decision, r.denyReason = mergeAllDecisions(r.decision, r.denyReason, overrides)
	r.reason = "isolated env+" + r.reason
	return r, true
}

func guardedHarnessEnvAssignment(
	word *syntax.Word,
	tempVar string,
	ctx evalContext,
) (name, suffix string, tempDerived bool, assignment envAssignment, ok bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", "", false, envAssignment{}, false
	}
	if len(word.Parts) == 1 {
		literal, literalOK := word.Parts[0].(*syntax.Lit)
		if !literalOK {
			return "", "", false, envAssignment{}, false
		}
		name, value, found := strings.Cut(literal.Value, "=")
		if !found || !syntax.ValidName(name) {
			return "", "", false, envAssignment{}, false
		}
		return name, value, false, envAssignment{name: name, value: value, staticValue: true}, true
	}
	prefix, prefixOK := word.Parts[0].(*syntax.Lit)
	if !prefixOK || !strings.HasSuffix(prefix.Value, "=") {
		return "", "", false, envAssignment{}, false
	}
	name = strings.TrimSuffix(prefix.Value, "=")
	if !syntax.ValidName(name) {
		return "", "", false, envAssignment{}, false
	}
	assignment.name = name

	if derivedName, derivedSuffix, derivedOK := guardedHarnessAssignment(word, tempVar); derivedOK {
		assignment.value = "$" + tempVar + derivedSuffix
		return derivedName, derivedSuffix, true, assignment, true
	}

	decoded, decodedOK := wordDecodedLiteralWithContext(word, ctx)
	if decodedOK {
		decodedName, value, found := strings.Cut(decoded, "=")
		if found && decodedName == name {
			assignment.value = value
			assignment.staticValue = true
			return name, value, false, assignment, true
		}
	}

	if name == "PATH" && guardedHarnessPathAssignment(word, ctx) {
		return name, "", false, assignment, true
	}
	return "", "", false, envAssignment{}, false
}

func guardedHarnessRequiredSuffix(name, suffix, legacySuffix string) bool {
	if name == "HOME" && suffix == "" {
		return true
	}
	if suffix == legacySuffix {
		return true
	}
	return guardedTempDescendantSuffix(suffix)
}

func guardedHarnessEnvOverride(assignment envAssignment, tempDerived bool, suffix string, ctx evalContext) *result {
	if tempDerived {
		if _, required := guardedHarnessEnvSuffixes[assignment.name]; required {
			return nil
		}
	}
	if tempDerived && assignment.name == "BUN_INSTALL_CACHE_DIR" && suffix == "/bun-cache" {
		return nil
	}
	r := validateEnvAssignments([]envAssignment{assignment}, ctx)
	return r
}

func guardedHarnessPathAssignment(word *syntax.Word, ctx evalContext) bool {
	if word == nil || len(word.Parts) != 2 {
		return false
	}
	prefix, ok := word.Parts[0].(*syntax.Lit)
	if !ok || prefix.Value != "PATH=" {
		return false
	}
	quoted, ok := word.Parts[1].(*syntax.DblQuoted)
	if !ok || quoted.Dollar || len(quoted.Parts) == 0 {
		return false
	}
	if literal, ok := quoted.Parts[0].(*syntax.Lit); ok && len(quoted.Parts) == 1 {
		return guardedHarnessPath(literal.Value, ctx)
	}

	last, ok := quoted.Parts[len(quoted.Parts)-1].(*syntax.ParamExp)
	if !ok || !guardedParam(last, "PATH") {
		return false
	}
	var prefixPath strings.Builder
	for _, part := range quoted.Parts[:len(quoted.Parts)-1] {
		switch p := part.(type) {
		case *syntax.Lit:
			prefixPath.WriteString(p.Value)
		case *syntax.ParamExp:
			if p.Param == nil || !guardedParam(p, p.Param.Value) {
				return false
			}
			value, exists := ctx.shellVars[p.Param.Value]
			if !exists {
				return false
			}
			prefixPath.WriteString(value)
		default:
			return false
		}
	}
	value := prefixPath.String()
	if value == "" {
		return true
	}
	if !strings.HasSuffix(value, ":") {
		return false
	}
	return guardedHarnessPath(strings.TrimSuffix(value, ":"), ctx)
}

func guardedHarnessAssignment(word *syntax.Word, tempVar string) (name, value string, ok bool) {
	if word == nil || len(word.Parts) != 2 {
		return "", "", false
	}
	prefix, ok := word.Parts[0].(*syntax.Lit)
	if !ok || !strings.HasSuffix(prefix.Value, "=") {
		return "", "", false
	}
	name = strings.TrimSuffix(prefix.Value, "=")
	if name == "PATH" {
		quoted, ok := word.Parts[1].(*syntax.DblQuoted)
		if !ok || quoted.Dollar || len(quoted.Parts) != 1 {
			return "", "", false
		}
		literal, ok := quoted.Parts[0].(*syntax.Lit)
		if !ok {
			return "", "", false
		}
		return name, literal.Value, true
	}
	quoted, ok := word.Parts[1].(*syntax.DblQuoted)
	if !ok || quoted.Dollar || len(quoted.Parts) < 1 || len(quoted.Parts) > 2 {
		return "", "", false
	}
	param, ok := quoted.Parts[0].(*syntax.ParamExp)
	if !ok || !guardedParam(param, tempVar) {
		return "", "", false
	}
	if len(quoted.Parts) == 2 {
		literal, ok := quoted.Parts[1].(*syntax.Lit)
		if !ok {
			return "", "", false
		}
		value = literal.Value
	}
	return name, value, true
}

func guardedHarnessPath(value string, ctx evalContext) bool {
	parts := strings.Split(value, ":")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part == "" || !filepath.IsAbs(part) ||
			isSafeAbsolutePath(part+string(filepath.Separator), ctx) != nil {
			return false
		}
	}
	return true
}

func guardedTempMkdir(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	if !ok || len(call.Assigns) != 0 || len(call.Args) < 3 ||
		!guardedCommandWord(call.Args[0], "mkdir", ctx) || wordLiteral(call.Args[1]) != "-p" {
		return false
	}
	for _, arg := range call.Args[2:] {
		_, suffix, ok := guardedTempWord(arg, tempVar)
		if !ok || !guardedTempDescendantSuffix(suffix) {
			return false
		}
	}
	return true
}

func guardedTempWord(word *syntax.Word, tempVar string) (name, suffix string, ok bool) {
	if word == nil || len(word.Parts) != 1 {
		return "", "", false
	}
	quoted, ok := word.Parts[0].(*syntax.DblQuoted)
	if !ok || quoted.Dollar || len(quoted.Parts) < 1 || len(quoted.Parts) > 2 {
		return "", "", false
	}
	param, ok := quoted.Parts[0].(*syntax.ParamExp)
	if !ok || !guardedParam(param, tempVar) {
		return "", "", false
	}
	if len(quoted.Parts) == 2 {
		literal, ok := quoted.Parts[1].(*syntax.Lit)
		if !ok {
			return "", "", false
		}
		suffix = literal.Value
	}
	return tempVar, suffix, true
}

func guardedTempDescendantSuffix(suffix string) bool {
	if !strings.HasPrefix(suffix, "/") || suffix == "/" || filepath.Clean(suffix) != suffix {
		return false
	}
	return !strings.HasPrefix(suffix, "/../") && suffix != "/.."
}

func guardedCommandWord(word *syntax.Word, name string, ctx evalContext) bool {
	return guardedCommandName(wordLiteral(word), name, ctx)
}

func guardedCommandName(command, name string, ctx evalContext) bool {
	if command == name {
		return true
	}
	return filepath.IsAbs(command) && filepath.Base(command) == name &&
		isSafeAbsolutePath(filepath.Dir(command)+string(filepath.Separator), ctx) == nil
}

func mergeGuardedHarnessResults(results []*result) *result {
	decision, denyReason := mergeAllDecisions(decisionAllow, "", results)
	return &result{reason: "isolated test harness", decision: decision, denyReason: denyReason}
}

func evaluateGuardedKnownTempCleanup(stmts []*syntax.Stmt, ctx evalContext) (*result, bool) {
	if len(stmts) != 6 {
		return nil, false
	}
	tempVar, tempPath, ok := guardedStaticTempAssignment(stmts[0], ctx)
	if !ok || !guardedKnownTempCase(stmts[1], tempVar, tempPath) ||
		!guardedVarTest(stmts[2], tempVar, false, ctx) ||
		!guardedRecursiveChmod(stmts[3], tempVar, ctx) ||
		!guardedKnownTempRm(stmts[4], tempVar, ctx) ||
		!guardedVarTest(stmts[5], tempVar, true, ctx) {
		return nil, false
	}
	return approved("guarded temporary cleanup"), true
}

func guardedStaticTempAssignment(stmt *syntax.Stmt, ctx evalContext) (string, string, bool) {
	if !guardedPlainStmt(stmt) {
		return "", "", false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 0 || len(call.Assigns) != 1 {
		return "", "", false
	}
	assign := call.Assigns[0]
	if assign == nil || assign.Name == nil || assign.Value == nil || assign.Append || assign.Naked ||
		assign.Index != nil || assign.Array != nil || len(assign.Value.Parts) != 1 {
		return "", "", false
	}
	value, ok := wordDecodedLiteral(assign.Value)
	if !ok || !filepath.IsAbs(value) || !destructiveTargetInAllowedScope(ctx.cwd, value) {
		return "", "", false
	}
	return assign.Name.Value, value, true
}

func guardedKnownTempCase(stmt *syntax.Stmt, tempVar, tempPath string) bool {
	if !guardedPlainStmt(stmt) {
		return false
	}
	clause, ok := stmt.Cmd.(*syntax.CaseClause)
	if !ok || clause.Braces || !guardedQuotedVarWord(clause.Word, tempVar, "") || len(clause.Items) != 2 {
		return false
	}
	accepted := clause.Items[0]
	if accepted == nil || accepted.Op.String() != ";;" || len(accepted.Stmts) != 0 || len(accepted.Patterns) == 0 {
		return false
	}
	matched := false
	for _, patternWord := range accepted.Patterns {
		pattern, ok := wordDecodedLiteral(patternWord)
		if !ok {
			return false
		}
		if guardedShellPatternMatch(pattern, tempPath) {
			matched = true
		}
	}
	fallback := clause.Items[1]
	if fallback == nil || fallback.Op.String() != ";;" || len(fallback.Patterns) != 1 || wordLiteral(fallback.Patterns[0]) != "*" ||
		len(fallback.Stmts) != 1 {
		return false
	}
	exitCall, ok := guardedPlainCall(fallback.Stmts[0])
	if !ok || len(exitCall.Assigns) != 0 || len(exitCall.Args) != 2 || wordLiteral(exitCall.Args[0]) != "exit" {
		return false
	}
	status, err := strconv.Atoi(wordLiteral(exitCall.Args[1]))
	return matched && err == nil && status > 0 && status <= 255
}

func guardedShellPatternMatch(pattern, value string) bool {
	if strings.ContainsAny(pattern, "[]\\") {
		return false
	}
	expression := regexp.QuoteMeta(pattern)
	expression = strings.ReplaceAll(expression, `\*`, `.*`)
	expression = strings.ReplaceAll(expression, `\?`, `.`)
	matched, err := regexp.MatchString("^"+expression+"$", value)
	return err == nil && matched
}

func guardedVarTest(stmt *syntax.Stmt, tempVar string, absent bool, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 || !guardedCommandWord(call.Args[0], "test", ctx) {
		return false
	}
	if absent {
		return len(call.Args) == 4 && wordLiteral(call.Args[1]) == "!" &&
			wordLiteral(call.Args[2]) == "-e" && guardedQuotedVarWord(call.Args[3], tempVar, "")
	}
	return len(call.Args) == 3 && wordLiteral(call.Args[1]) == "-d" &&
		guardedQuotedVarWord(call.Args[2], tempVar, "")
}

func guardedRecursiveChmod(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 4 &&
		guardedCommandWord(call.Args[0], "chmod", ctx) && wordLiteral(call.Args[1]) == "-R" &&
		wordLiteral(call.Args[2]) == "u+w" && guardedQuotedVarWord(call.Args[3], tempVar, "")
}

func guardedKnownTempRm(stmt *syntax.Stmt, tempVar string, ctx evalContext) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 4 &&
		guardedCommandWord(call.Args[0], "rm", ctx) && wordLiteral(call.Args[1]) == "-rf" &&
		wordLiteral(call.Args[2]) == "--" && guardedQuotedVarWord(call.Args[3], tempVar, "")
}

func guardedPlainStmt(stmt *syntax.Stmt) bool {
	return stmt != nil && !stmt.Negated && !stmt.Background && !stmt.Coprocess &&
		len(stmt.Redirs) == 0 && stmt.Cmd != nil
}

func guardedPlainCall(stmt *syntax.Stmt) (*syntax.CallExpr, bool) {
	if !guardedPlainStmt(stmt) {
		return nil, false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	return call, ok
}

func guardedLiteralArgs(words []*syntax.Word) ([]string, bool) {
	out := make([]string, len(words))
	for i, word := range words {
		value, ok := wordDecodedLiteral(word)
		if !ok {
			return nil, false
		}
		out[i] = value
	}
	return out, true
}

func guardedQuotedVarWord(word *syntax.Word, name, suffix string) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	quoted, ok := word.Parts[0].(*syntax.DblQuoted)
	if !ok || quoted.Dollar || len(quoted.Parts) < 1 || len(quoted.Parts) > 2 {
		return false
	}
	param, ok := quoted.Parts[0].(*syntax.ParamExp)
	if !ok || !guardedParam(param, name) {
		return false
	}
	if suffix == "" {
		return len(quoted.Parts) == 1
	}
	if len(quoted.Parts) != 2 {
		return false
	}
	literal, ok := quoted.Parts[1].(*syntax.Lit)
	return ok && literal.Value == suffix
}

func guardedParamWord(word *syntax.Word, name string) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	param, ok := word.Parts[0].(*syntax.ParamExp)
	return ok && guardedParam(param, name)
}

func guardedParam(param *syntax.ParamExp, name string) bool {
	return param != nil && param.Param != nil && param.Param.Value == name &&
		!param.Excl && !param.Length && !param.Width && param.Index == nil &&
		param.Slice == nil && param.Repl == nil && param.Names == 0 && param.Exp == nil
}
