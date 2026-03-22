# claude-bash-approve

Go project in `hooks/bash-approve/`.

## Commands

```bash
cd hooks/bash-approve
go test ./...                # run tests
golangci-lint run ./...      # lint
```

## Adding commands

1. Add pattern to `allCommandPatterns` or `allWrapperPatterns` in `rules.go`
2. Add test cases in `main_test.go`
3. Update `categories.yaml` if introducing a new group
