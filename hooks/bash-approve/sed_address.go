package main

import (
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func isSedAddressLiteral(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isLineNumberPipelineAssignment(w *syntax.Word, ctx evalContext) bool {
	calls := singleCmdSubstPipeCalls(w)
	if len(calls) < 2 {
		return false
	}
	lineField, ok := lineNumberFieldForCall(calls[0], ctx)
	if !ok {
		return false
	}
	for _, call := range calls[1 : len(calls)-1] {
		if !isLineNumberPipelineMiddleCall(call, ctx) {
			return false
		}
	}
	return isCutColonFieldCall(calls[len(calls)-1], ctx, lineField)
}

func isLineRecordPipelineAssignment(w *syntax.Word, ctx evalContext) (int, bool) {
	calls := singleCmdSubstPipeCalls(w)
	if len(calls) == 0 {
		return 0, false
	}
	lineField, ok := lineNumberFieldForCall(calls[0], ctx)
	if !ok {
		return 0, false
	}
	for _, call := range calls[1:] {
		if !isLineRecordPipelineMiddleCall(call, ctx) {
			return 0, false
		}
	}
	return lineField, true
}

func isLineRecordCutSedAddressAssignment(w *syntax.Word, ctx evalContext) bool {
	calls := singleCmdSubstPipeCalls(w)
	if len(calls) < 2 {
		return false
	}
	varName, ok := lineRecordVarFromEchoCall(calls[0])
	if !ok {
		return false
	}
	lineField, ok := ctx.lineRecordVars[varName]
	if !ok {
		return false
	}
	for _, call := range calls[1 : len(calls)-1] {
		if !isLineRecordPipelineMiddleCall(call, ctx) {
			return false
		}
	}
	return isCutColonFieldCall(calls[len(calls)-1], ctx, lineField)
}

func singleCmdSubstPipeCalls(w *syntax.Word) []*syntax.CallExpr {
	cs := singleCmdSubstWord(w)
	if cs == nil || len(cs.Stmts) != 1 {
		return nil
	}
	stmt := cs.Stmts[0]
	if stmt == nil || stmt.Cmd == nil || len(stmt.Redirs) > 0 {
		return nil
	}
	return flattenPipeCalls(stmt.Cmd)
}

func singleCmdSubstWord(w *syntax.Word) *syntax.CmdSubst {
	if w == nil || len(w.Parts) != 1 {
		return nil
	}
	switch p := w.Parts[0].(type) {
	case *syntax.CmdSubst:
		return p
	case *syntax.DblQuoted:
		if len(p.Parts) != 1 {
			return nil
		}
		cs, _ := p.Parts[0].(*syntax.CmdSubst)
		return cs
	default:
		return nil
	}
}

func flattenPipeCalls(cmd syntax.Command) []*syntax.CallExpr {
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		return []*syntax.CallExpr{c}
	case *syntax.BinaryCmd:
		if c.Op != syntax.Pipe {
			return nil
		}
		if c.X == nil || c.Y == nil || c.X.Cmd == nil || c.Y.Cmd == nil {
			return nil
		}
		left := flattenPipeCalls(c.X.Cmd)
		right := flattenPipeCalls(c.Y.Cmd)
		if left == nil || right == nil {
			return nil
		}
		return append(left, right...)
	default:
		return nil
	}
}

func lineNumberFieldForCall(call *syntax.CallExpr, ctx evalContext) (int, bool) {
	if len(call.Args) == 0 {
		return 0, false
	}
	name, ok := wordDecodedLiteralWithContext(call.Args[0], ctx)
	if !ok {
		return 0, false
	}
	switch name {
	case "grep":
		return grepLineNumberField(call.Args[1:], ctx)
	case "rg":
		return rgLineNumberField(call.Args[1:], ctx)
	case "git":
		if len(call.Args) < 3 {
			return 0, false
		}
		subcmd, ok := wordDecodedLiteralWithContext(call.Args[1], ctx)
		if !ok || subcmd != "grep" {
			return 0, false
		}
		return gitGrepLineNumberField(call.Args[2:], ctx)
	default:
		return 0, false
	}
}

func grepLineNumberField(args []*syntax.Word, ctx evalContext) (int, bool) {
	state, ok := parseLineNumberSearchArgs(args, ctx, false, "h", "cLlqZz")
	if !ok || !state.hasLineNumber {
		return 0, false
	}
	if state.noFilename {
		return 1, true
	}
	if state.withFilename || state.pathCount > 1 || state.pathHasGlob {
		return 2, true
	}
	return 1, true
}

func rgLineNumberField(args []*syntax.Word, ctx evalContext) (int, bool) {
	state, ok := parseLineNumberSearchArgs(args, ctx, false, "I", "chlLNqz")
	if !ok || !state.hasLineNumber {
		return 0, false
	}
	if state.noFilename {
		return 1, true
	}
	if state.withFilename || state.pathCount > 1 || state.pathHasGlob {
		return 2, true
	}
	return 1, true
}

func gitGrepLineNumberField(args []*syntax.Word, ctx evalContext) (int, bool) {
	state, ok := parseLineNumberSearchArgs(args, ctx, true, "h", "cLlqZz")
	if !ok || !state.hasLineNumber {
		return 0, false
	}
	if state.noFilename {
		return 1, true
	}
	return 2, true
}

type lineNumberSearchState struct {
	hasLineNumber bool
	withFilename  bool
	noFilename    bool
	pathCount     int
	pathHasGlob   bool
}

func parseLineNumberSearchArgs(args []*syntax.Word, ctx evalContext, patternFromOptionDefault bool, shortNoFilename, shortReject string) (lineNumberSearchState, bool) {
	lits := make([]string, 0, len(args))
	for _, arg := range args {
		lit, ok := wordDecodedLiteralWithContext(arg, ctx)
		if !ok {
			return lineNumberSearchState{}, false
		}
		lits = append(lits, lit)
	}

	state := lineNumberSearchState{}
	patternFromOption := patternFromOptionDefault
	var operands []string
	for i := 0; i < len(lits); i++ {
		lit := lits[i]
		if lit == "--" {
			operands = append(operands, lits[i+1:]...)
			break
		}
		if strings.HasPrefix(lit, "--") && len(lit) > 2 {
			ok := updateLongLineNumberSearchFlag(lit, &state, &patternFromOption)
			if !ok {
				return lineNumberSearchState{}, false
			}
			if lineNumberLongFlagConsumesNext(lit) {
				i++
				if i >= len(lits) {
					return lineNumberSearchState{}, false
				}
			}
			continue
		}
		if strings.HasPrefix(lit, "-") && lit != "-" {
			consumedNext, ok := updateShortLineNumberSearchFlags(lit, &state, &patternFromOption, shortNoFilename, shortReject)
			if !ok {
				return lineNumberSearchState{}, false
			}
			if consumedNext {
				i++
				if i >= len(lits) {
					return lineNumberSearchState{}, false
				}
			}
			continue
		}
		operands = append(operands, lit)
	}
	paths := operands
	if !patternFromOption {
		if len(operands) == 0 {
			return lineNumberSearchState{}, false
		}
		paths = operands[1:]
	}
	state.pathCount = len(paths)
	for _, path := range paths {
		if strings.ContainsAny(path, "*?[") {
			state.pathHasGlob = true
			break
		}
	}
	return state, true
}

func updateLongLineNumberSearchFlag(lit string, state *lineNumberSearchState, patternFromOption *bool) bool {
	name := strings.TrimPrefix(lit, "--")
	if flag, _, ok := strings.Cut(name, "="); ok {
		name = flag
	}
	switch name {
	case "line-number":
		state.hasLineNumber = true
	case "recursive", "dereference-recursive", "with-filename":
		state.withFilename = true
	case "no-filename":
		state.noFilename = true
	case "regexp", "file":
		*patternFromOption = true
	case "count", "files-with-matches", "files-without-match", "quiet", "null", "null-data", "no-line-number", "help", "version":
		return false
	}
	return true
}

func lineNumberLongFlagConsumesNext(lit string) bool {
	name := strings.TrimPrefix(lit, "--")
	if strings.Contains(name, "=") {
		return false
	}
	return name == "regexp" || name == "file"
}

func updateShortLineNumberSearchFlags(lit string, state *lineNumberSearchState, patternFromOption *bool, shortNoFilename, shortReject string) (bool, bool) {
	for i := 1; i < len(lit); i++ {
		c := lit[i]
		if strings.ContainsRune(shortReject, rune(c)) {
			return false, false
		}
		if strings.ContainsRune(shortNoFilename, rune(c)) {
			state.noFilename = true
			continue
		}
		switch c {
		case 'n':
			state.hasLineNumber = true
		case 'r', 'R', 'H':
			state.withFilename = true
		case 'e', 'f':
			*patternFromOption = true
			return i+1 == len(lit), true
		case 'm', 'A', 'B', 'C':
			return i+1 == len(lit), true
		}
	}
	return false, true
}

func isLineNumberPipelineMiddleCall(call *syntax.CallExpr, ctx evalContext) bool {
	if len(call.Args) == 0 {
		return false
	}
	name, ok := wordDecodedLiteralWithContext(call.Args[0], ctx)
	if !ok {
		return false
	}
	if name != "head" {
		return false
	}
	return isHeadAtMostOneCall(call.Args[1:], ctx)
}

func isLineRecordPipelineMiddleCall(call *syntax.CallExpr, ctx evalContext) bool {
	if isLineNumberPipelineMiddleCall(call, ctx) {
		return true
	}
	return isLinePreservingGrepFilterCall(call, ctx)
}

func isLinePreservingGrepFilterCall(call *syntax.CallExpr, ctx evalContext) bool {
	if len(call.Args) == 0 {
		return false
	}
	name, ok := wordDecodedLiteralWithContext(call.Args[0], ctx)
	if !ok || name != "grep" {
		return false
	}
	patternOperands := 0
	for i := 1; i < len(call.Args); i++ {
		lit, ok := wordDecodedLiteralWithContext(call.Args[i], ctx)
		if !ok {
			return false
		}
		if lit == "--" {
			patternOperands += len(call.Args) - i - 1
			break
		}
		if strings.HasPrefix(lit, "--") && len(lit) > 2 {
			name := strings.TrimPrefix(lit, "--")
			if flag, _, ok := strings.Cut(name, "="); ok {
				name = flag
			}
			switch name {
			case "invert-match", "ignore-case", "extended-regexp", "fixed-strings", "perl-regexp",
				"basic-regexp", "word-regexp", "line-regexp", "no-messages":
				continue
			case "regexp":
				if !strings.Contains(lit, "=") {
					i++
					if i >= len(call.Args) {
						return false
					}
				}
				patternOperands++
				continue
			default:
				return false
			}
		}
		if strings.HasPrefix(lit, "-") && lit != "-" {
			for j := 1; j < len(lit); j++ {
				switch lit[j] {
				case 'v', 'i', 'E', 'F', 'P', 'G', 'w', 'x', 's':
					continue
				case 'e':
					if j+1 != len(lit) {
						return false
					}
					i++
					if i >= len(call.Args) {
						return false
					}
					patternOperands++
					continue
				default:
					return false
				}
			}
			continue
		}
		patternOperands++
	}
	return patternOperands == 1
}

func isHeadAtMostOneCall(args []*syntax.Word, ctx evalContext) bool {
	if len(args) == 0 {
		return false
	}
	if len(args) == 1 {
		lit, ok := wordDecodedLiteralWithContext(args[0], ctx)
		return ok && (lit == "-1" || lit == "-n1")
	}
	if len(args) == 2 {
		flag, ok := wordDecodedLiteralWithContext(args[0], ctx)
		if !ok || flag != "-n" {
			return false
		}
		count, ok := wordDecodedLiteralWithContext(args[1], ctx)
		return ok && count == "1"
	}
	return false
}

func lineRecordVarFromEchoCall(call *syntax.CallExpr) (string, bool) {
	if len(call.Args) != 2 {
		return "", false
	}
	name, ok := wordDecodedLiteral(call.Args[0])
	if !ok || name != "echo" {
		return "", false
	}
	return plainParamWordName(call.Args[1])
}

func plainParamWordName(w *syntax.Word) (string, bool) {
	if w == nil || len(w.Parts) != 1 {
		return "", false
	}
	switch p := w.Parts[0].(type) {
	case *syntax.ParamExp:
		return plainParamExpName(p)
	case *syntax.DblQuoted:
		if len(p.Parts) != 1 {
			return "", false
		}
		param, ok := p.Parts[0].(*syntax.ParamExp)
		if !ok {
			return "", false
		}
		return plainParamExpName(param)
	default:
		return "", false
	}
}

func plainParamExpName(p *syntax.ParamExp) (string, bool) {
	if p == nil || p.Param == nil {
		return "", false
	}
	if p.Excl || p.Length || p.Width || p.Index != nil || p.Slice != nil ||
		p.Repl != nil || p.Names != 0 || p.Exp != nil {
		return "", false
	}
	return p.Param.Value, true
}

func isCutColonFieldCall(call *syntax.CallExpr, ctx evalContext, field int) bool {
	if len(call.Args) < 2 {
		return false
	}
	name, ok := wordDecodedLiteralWithContext(call.Args[0], ctx)
	if !ok || name != "cut" {
		return false
	}
	delimOK := false
	fieldOK := false
	for i := 1; i < len(call.Args); i++ {
		lit, ok := wordDecodedLiteralWithContext(call.Args[i], ctx)
		if !ok {
			return false
		}
		switch lit {
		case "-d:", "--delimiter=:":
			delimOK = true
		case "-d", "--delimiter":
			i++
			if i >= len(call.Args) {
				return false
			}
			value, ok := wordDecodedLiteralWithContext(call.Args[i], ctx)
			if !ok || value != ":" {
				return false
			}
			delimOK = true
		case "-f" + strconv.Itoa(field), "--fields=" + strconv.Itoa(field):
			fieldOK = true
		case "-f", "--fields":
			i++
			if i >= len(call.Args) {
				return false
			}
			value, ok := wordDecodedLiteralWithContext(call.Args[i], ctx)
			if !ok || value != strconv.Itoa(field) {
				return false
			}
			fieldOK = true
		default:
			return false
		}
	}
	return delimOK && fieldOK
}
