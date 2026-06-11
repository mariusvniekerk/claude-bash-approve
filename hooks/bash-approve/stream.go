package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var readOnlyStreamReasons = map[string]bool{
	"bq":                    true,
	"gcloud compute read":   true,
	"gcloud config":         true,
	"gcloud container read": true,
	"gcloud logging":        true,
	"gcloud projects":       true,
	"gh read op":            true,
	"gh search":             true,
	"git read op":           true,
	"grep":                  true,
	"jj read op":            true,
	"kubectl read op":       true,
	"read-only":             true,
}

func isReadOnlyStreamStmt(stmt *syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern) bool {
	if stmt == nil {
		return false
	}
	streamCtx := ctx
	streamCtx.xargsHasPipelineInput = false
	streamCtx.xargsInputFromReadOnlyPipe = false
	r := evaluateStmt(stmt, streamCtx, wrapperPats, commandPats)
	if r == nil || r.decision != decisionAllow {
		return false
	}
	return reasonIsReadOnlyStream(r.reason)
}

func reasonIsReadOnlyStream(reason string) bool {
	if reason == "" {
		return false
	}
	for part := range strings.SplitSeq(reason, " | ") {
		if !reasonPartIsReadOnlyStream(part) {
			return false
		}
	}
	return true
}

func reasonPartIsReadOnlyStream(reason string) bool {
	for part := range strings.SplitSeq(reason, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		if part == "time" || part == "timeout" || part == "nice" || part == "env" || part == "env vars" {
			continue
		}
		if !readOnlyStreamReasons[part] {
			return false
		}
	}
	return true
}
