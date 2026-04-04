# Eliminate Global Pattern State and init()

## Problem

`allCommandPatterns` and `allWrapperPatterns` are mutable package-level `var` slices in `rules.go`. Validators like `isFindSafe` and `isXargsSafe` (from PR mariusvniekerk/claude-bash-approve#6) call `evaluate()`, which needs the pattern slices. But the pattern definitions want to reference those validators via `WithValidator()`. This circular dependency forces the use of `init()` functions that mutate the global slice after initialization:

```
allCommandPatterns (var)
    ← referenced by evaluate()
        ← called by isFindSafe / isXargsSafe
            ← need to be attached to patterns in allCommandPatterns
                → circular: can't reference validator at definition time
```

The `init()` workaround finds patterns by tag and mutates the slice to attach validators post-init. This is fragile (tag string matching), implicit (no compile-time guarantee the attachment happens), and prevents the slices from being immutable.

## Solution

Pass an evaluator function to validators at call time, breaking the circular dependency.

### Type changes in `rules.go`

New type:

```go
type evaluatorFunc func(cmd string) *result
```

Changed validator signature:

```go
// Before
type argsValidator func(args []*syntax.Word) bool

// After
type argsValidator func(args []*syntax.Word, eval evaluatorFunc) bool
```

Global vars become functions returning fresh slices:

```go
// Before
var allCommandPatterns = []pattern{ ... }
var allWrapperPatterns = []pattern{ ... }

// After
func commandPatterns() []pattern { return []pattern{ ... } }
func wrapperPatterns() []pattern { return []pattern{ ... } }
```

With the circular dependency broken, future validators (find, xargs from PR #6) can attach inline at definition time instead of via `init()`:

```go
// Before (validator set in init() to break cycle)
NewPattern(`^xargs\b`, tags("xargs", "shell")),

// After (direct attachment, no init() needed)
NewPattern(`^xargs\b`, tags("xargs", "shell"), WithValidator(isXargsSafe)),
```

### Call site change in `main.go`

In `matchAndBuild`, construct the evaluator closure from the already-in-scope pattern slices:

```go
eval := func(cmd string) *result {
    return evaluate(cmd, wrapperPats, commandPats)
}
if matched.validate != nil && !matched.validate(astArgs, eval) {
    return &result{reason: matched.label(), decision: decisionAsk}
}
```

`buildActivePatterns` calls the new functions instead of referencing globals:

```go
// Before
return allWrapperPatterns, allCommandPatterns

// After
return wrapperPatterns(), commandPatterns()
```

### Validator change in `curl.go`

`isCurlReadOnly` gains an unused `_ evaluatorFunc` parameter to satisfy the new `argsValidator` signature. Curl does not do recursive evaluation, so the parameter is ignored.

### Tests

`evaluateAll` in `main_test.go` calls the new functions:

```go
func evaluateAll(cmd string) *result {
    return evaluate(cmd, wrapperPatterns(), commandPatterns())
}
```

All other test references to `allWrapperPatterns` / `allCommandPatterns` change to `wrapperPatterns()` / `commandPatterns()`. No test logic changes.

## Performance

With `var`, regexes compile once at package init. With functions, `regexp.MustCompile` runs on each call to `commandPatterns()` / `wrapperPatterns()`.

**Production:** `buildActivePatterns` calls each function once per CLI invocation. Recursive validators reuse the same slices through the evaluator closure — no additional compilation.

**Tests:** `evaluateAll` calls both functions per test case (~70 subtests currently). That's ~70 x 120 pattern compilations = ~8,400 `regexp.MustCompile` calls at ~1us each = under 10ms total. Negligible.

## Files changed (this PR)

| File | Changes |
|------|---------|
| `rules.go` | Add `evaluatorFunc` type. Change `argsValidator` signature. Convert `allCommandPatterns`/`allWrapperPatterns` from `var` to functions. |
| `main.go` | Update `buildActivePatterns` to call functions. Construct evaluator closure in `matchAndBuild`. |
| `curl.go` | Update `isCurlReadOnly` signature to accept `_ evaluatorFunc` (unused). |
| `main_test.go` | Replace global references with function calls. |

## Implementation order

This change lands first, on the current codebase. Only `rules.go`, `main.go`, `curl.go`, and `main_test.go` change — there are no find/xargs files yet.

The find/xargs PR (mariusvniekerk/claude-bash-approve#6) then rebases onto this. Its rebase is mechanical:
- Delete `init()` from `find.go` and `xargs.go`
- Change `isFindSafe` and `isXargsSafe` signatures to accept `eval evaluatorFunc`
- Replace `evaluate(cmd, allWrapperPatterns, allCommandPatterns)` with `eval(cmd)`
- Use `WithValidator(isFindSafe)` / `WithValidator(isXargsSafe)` inline in the pattern definitions
