package main

import (
	"path/filepath"
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
	if len(stmts) != 5 {
		return nil, false
	}
	tempVar, ok := guardedTempAssignment(stmts[0])
	if !ok ||
		!guardedTempCase(stmts[1], tempVar) ||
		!guardedTempChmod(stmts[2], tempVar) ||
		!guardedTempTrap(stmts[3], tempVar) {
		return nil, false
	}
	r, ok := guardedTempEnvRunner(stmts[4], tempVar, ctx, wrapperPats, commandPats)
	if !ok || r == nil {
		return nil, false
	}
	if r.decision != decisionAllow {
		return r, true
	}
	return approved("isolated test harness"), true
}

func guardedTempAssignment(stmt *syntax.Stmt) (string, bool) {
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
	subst, ok := assign.Value.Parts[0].(*syntax.CmdSubst)
	if !ok || subst.Backquotes || subst.TempFile || subst.ReplyVar || len(subst.Stmts) != 1 {
		return "", false
	}
	inner, ok := guardedPlainCall(subst.Stmts[0])
	if !ok || len(inner.Assigns) != 0 || len(inner.Args) != 4 {
		return "", false
	}
	args, ok := guardedLiteralArgs(inner.Args)
	if !ok || args[0] != "mktemp" || args[1] != "-d" || args[2] != "-t" ||
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

func guardedTempChmod(stmt *syntax.Stmt, tempVar string) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 3 &&
		wordLiteral(call.Args[0]) == "chmod" &&
		wordLiteral(call.Args[1]) == "700" &&
		guardedQuotedVarWord(call.Args[2], tempVar, "")
}

func guardedTempTrap(stmt *syntax.Stmt, tempVar string) bool {
	call, ok := guardedPlainCall(stmt)
	if !ok || len(call.Assigns) != 0 || len(call.Args) != 3 ||
		wordLiteral(call.Args[0]) != "trap" || wordLiteral(call.Args[2]) != "EXIT" {
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
		return guardedTempTrapRm(file.Stmts[0], tempVar)
	}
	if len(file.Stmts) != 3 {
		return false
	}
	statusVar, ok := guardedStatusAssignment(file.Stmts[0])
	return ok &&
		guardedTempTrapRm(file.Stmts[1], tempVar) &&
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

func guardedTempTrapRm(stmt *syntax.Stmt, tempVar string) bool {
	call, ok := guardedPlainCall(stmt)
	return ok && len(call.Assigns) == 0 && len(call.Args) == 4 &&
		wordLiteral(call.Args[0]) == "rm" &&
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
	if !ok || len(call.Assigns) != 0 || len(call.Args) < 2 || wordLiteral(call.Args[0]) != "env" {
		return nil, false
	}

	seen := make(map[string]bool, len(guardedHarnessEnvSuffixes)+2)
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
		name, suffix, matched := guardedHarnessAssignment(call.Args[i], tempVar)
		if !matched {
			break
		}
		if seen[name] {
			return nil, false
		}
		seen[name] = true
		if name == "PATH" {
			if !guardedHarnessPath(suffix, ctx) {
				return nil, false
			}
		} else if want, required := guardedHarnessEnvSuffixes[name]; required {
			if suffix != want {
				return nil, false
			}
		} else if name != "BUN_INSTALL_CACHE_DIR" || suffix != "/bun-cache" {
			return nil, false
		}
		i++
	}

	for name := range guardedHarnessEnvSuffixes {
		if !seen[name] {
			return nil, false
		}
	}
	if !seen["PATH"] || i >= len(call.Args) {
		return nil, false
	}

	runnerWords := call.Args[i:]
	command := argsText(runnerWords)
	executable := wordLiteral(runnerWords[0])
	if filepath.IsAbs(executable) && filepath.Base(executable) == "node" {
		if isSafeAbsolutePath(filepath.Dir(executable)+string(filepath.Separator), ctx) != nil ||
			len(runnerWords) < 2 || wordLiteral(runnerWords[1]) != "../node_modules/vite-plus/bin/vp" {
			return nil, false
		}
		command = "vp"
		if len(runnerWords) > 2 {
			command += " " + argsText(runnerWords[2:])
		}
	}
	r := evaluate(command, ctx, wrapperPats, commandPats)
	return r, r != nil
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
