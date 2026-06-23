package main

import (
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
	cs := singleCmdSubstWord(w)
	if cs == nil || len(cs.Stmts) != 1 {
		return false
	}
	stmt := cs.Stmts[0]
	if stmt == nil || stmt.Cmd == nil || len(stmt.Redirs) > 0 {
		return false
	}
	calls := flattenPipeCalls(stmt.Cmd)
	if len(calls) < 3 {
		return false
	}
	if !callProducesColonPrefixedLines(calls[0], ctx) {
		return false
	}
	for _, call := range calls[1 : len(calls)-1] {
		if !isLineNumberPipelineMiddleCall(call, ctx) {
			return false
		}
	}
	return isCutFirstColonFieldCall(calls[len(calls)-1], ctx)
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

func callProducesColonPrefixedLines(call *syntax.CallExpr, ctx evalContext) bool {
	if len(call.Args) == 0 {
		return false
	}
	name, ok := wordDecodedLiteralWithContext(call.Args[0], ctx)
	if !ok {
		return false
	}
	switch name {
	case "grep", "rg":
		return callArgsContainLineNumberFlag(call.Args[1:], ctx)
	case "git":
		if len(call.Args) < 3 {
			return false
		}
		subcmd, ok := wordDecodedLiteralWithContext(call.Args[1], ctx)
		if !ok || subcmd != "grep" {
			return false
		}
		return callArgsContainLineNumberFlag(call.Args[2:], ctx)
	default:
		return false
	}
}

func callArgsContainLineNumberFlag(args []*syntax.Word, ctx evalContext) bool {
	for _, arg := range args {
		lit, ok := wordDecodedLiteralWithContext(arg, ctx)
		if !ok {
			return false
		}
		if lit == "--" {
			return false
		}
		if lit == "-n" || lit == "--line-number" {
			return true
		}
		if strings.HasPrefix(lit, "-") && !strings.HasPrefix(lit, "--") && strings.Contains(lit[1:], "n") {
			return true
		}
		if !strings.HasPrefix(lit, "-") {
			return false
		}
	}
	return false
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

func isCutFirstColonFieldCall(call *syntax.CallExpr, ctx evalContext) bool {
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
		case "-f1", "--fields=1":
			fieldOK = true
		case "-f", "--fields":
			i++
			if i >= len(call.Args) {
				return false
			}
			value, ok := wordDecodedLiteralWithContext(call.Args[i], ctx)
			if !ok || value != "1" {
				return false
			}
			fieldOK = true
		default:
			return false
		}
	}
	return delimOK && fieldOK
}
