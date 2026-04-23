package main

import (
	"regexp"

	"mvdan.cc/sh/v3/syntax"
)

// pattern defines a command or wrapper that can be matched.
// tags controls both the label (first tag) and config matching (all tags).
// decision is the hook permission decision: "allow" (default) or "" (no opinion, ask user).
// denyReason, if set, is shown to Claude when the command is denied, explaining why.
// argsValidator is called after a regex match to refine the decision using
// the parsed AST arguments (including command name at [0]). Return true to
// keep the matched decision, false to downgrade to no opinion.
type argsValidator func(args []*syntax.Word, ctx evalContext) bool

type pattern struct {
	re               *regexp.Regexp
	tags             []string
	decision         string
	denyReason       string
	validate         argsValidator
	validateFallback string
}

// label returns the first tag, used in approval reason strings.
func (p pattern) label() string { return p.tags[0] }

type patternOption func(*pattern)

// WithDecision overrides the default "allow" decision (e.g. "ask", "deny").
func WithDecision(decision string) patternOption {
	return func(p *pattern) {
		p.decision = decision
	}
}

// WithDenyReason sets a human-readable reason shown to Claude when a command is denied.
func WithDenyReason(reason string) patternOption {
	return func(p *pattern) {
		p.denyReason = reason
	}
}

// WithValidator attaches an argument validator that runs after regex match.
func WithValidator(v argsValidator) patternOption {
	return func(p *pattern) {
		p.validate = v
	}
}

// WithValidatorFallback changes the decision used when a validator rejects.
func WithValidatorFallback(decision string) patternOption {
	return func(p *pattern) {
		p.validateFallback = decision
	}
}

func tags(t ...string) []string { return t }

func NewPattern(re string, t []string, opts ...patternOption) pattern {
	p := pattern{
		re:               regexp.MustCompile(re),
		tags:             t,
		decision:         decisionAllow,
		validateFallback: decisionAsk,
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func wrapperPatterns() []pattern {
	return []pattern{
		NewPattern(`^timeout\s+\d+\s+`, tags("timeout", "wrapper")),
		NewPattern(`^nice\s+(-n\s*\d+\s+)?`, tags("nice", "wrapper")),
		NewPattern(`^env\s+`, tags("env", "wrapper")),
		NewPattern(`^([A-Z_][A-Z0-9_]*=[^\s]*\s+)+`, tags("env vars", "wrapper")),
		NewPattern(`^(\.\./)*\.?venv/bin/`, tags(".venv", "wrapper")),
		NewPattern(`^/[^\s]+/\.?venv/bin/`, tags(".venv", "wrapper")),
		NewPattern(`^bundle\s+exec\s+`, tags("bundle exec", "wrapper")),
		NewPattern(`^rtk\s+proxy\s+`, tags("rtk proxy", "wrapper")),
		NewPattern(`^command\s+`, tags("command", "wrapper")),
		NewPattern(`^(\.\./)*node_modules/\.bin/`, tags("node_modules/.bin", "wrapper")),
		NewPattern(`^/[^\s]+/`, tags("absolute path", "wrapper")),
	}
}

func commandPatterns() []pattern {
	return []pattern{
		// git
		NewPattern(`^git\s+(-C\s+\S+\s+)?(diff|log|status|show|branch|stash\s+list|bisect|worktree\s+list|fetch|ls-files|ls-remote|rev-parse|describe|blame|grep|check-ignore|shortlog|name-rev|cat-file)\b`, tags("git read op", "git")),
		// git destructive ops — blocked by default (matches before write ops)
		NewPattern(`^git\s+(-C\s+\S+\s+)?stash\b`, tags("git stash", "git destructive", "git"), WithDecision("deny"),
			WithDenyReason("BLOCKED: git stash is banned. It destroys work and causes merge conflicts. Make targeted edits instead.")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?revert\b`, tags("git revert", "git destructive", "git"), WithDecision("deny"),
			WithDenyReason("BLOCKED: git revert is banned. Use targeted edits to undo changes.")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?reset\s+--hard\b`, tags("git reset --hard", "git destructive", "git"), WithDecision("deny"),
			WithDenyReason("BLOCKED: git reset --hard is banned. It destroys uncommitted work.")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?checkout\s+(--\s+)?\.$`, tags("git checkout .", "git destructive", "git"), WithDecision("deny"),
			WithDenyReason("BLOCKED: blanket git checkout is banned. Restore specific files only.")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?clean\s+-[a-zA-Z]*f`, tags("git clean -f", "git destructive", "git"), WithDecision("deny"),
			WithDenyReason("BLOCKED: git clean -f is banned. Remove specific files only.")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?(add|checkout|cherry-pick|commit|merge|pull|rebase|switch|remote|config|rerere|worktree)\b`, tags("git write op", "git")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?tag\b`, tags("git tag", "git"), WithDecision("ask")),
		NewPattern(`^git\s+(-C\s+\S+\s+)?push\b`, tags("git push", "git"), WithDecision("")),

		// jj (Jujutsu)
		NewPattern(`^jj\s+(log|diff|show|status|st|file\s+(list|show)|op\s+log|config\s+(list|get)|bookmark\s+list|branch\s+list)\b`, tags("jj read op", "jj")),
		NewPattern(`^jj\s+(new|commit|describe|desc|squash|edit|abandon|restore|rebase|split|bookmark\s+(create|delete|forget|move|rename|set|track|untrack)|branch\s+(create|delete|forget|move|rename|set|track|untrack)|git\s+(fetch|import|export))\b`, tags("jj write op", "jj")),
		NewPattern(`^jj\s+git\s+push\b`, tags("jj git push", "jj"), WithDecision("")),

		// beads (bd)
		NewPattern(`^bd\b`, tags("bd")),

		// python
		NewPattern(`^pytest\b`, tags("pytest", "python")),
		NewPattern(`^python3?\b`, tags("python")),
		NewPattern(`^ruff\b`, tags("ruff", "python")),
		NewPattern(`^uv\s+(pip|run|sync|venv|add|remove|lock|tool)\b`, tags("uv", "python")),
		NewPattern(`^uvx\b`, tags("uvx", "python")),

		// node
		NewPattern(`^npm\s+(install|run|test|build|ci)\b`, tags("npm", "node")),
		NewPattern(`^npx\b`, tags("npx", "node")),
		NewPattern(`^node\s+-[ep]\b`, tags("node -e", "node")),
		NewPattern(`^bun\s+(install|run|test|build|add|remove)\b`, tags("bun", "node")),
		NewPattern(`^bunx\b`, tags("bunx", "node")),
		NewPattern(`^vitest\b`, tags("vitest", "node")),

		// rust
		NewPattern(`^cargo\s+(build|test|run|check|clippy|fmt|clean)\b`, tags("cargo", "rust")),
		NewPattern(`^maturin\s+(develop|build)\b`, tags("maturin", "rust")),

		// make / mise
		NewPattern(`^make\b`, tags("make")),
		NewPattern(`^mise\s+(run|exec|install|use|env|--version|which|search|activate|list|ls|doctor|trust|reshim|settings|lock)\b`, tags("mise")),
		NewPattern(`^mise\s+approve\b`, tags("mise")),

		// shell
		NewPattern(`^rm\s+(-[a-zA-Z]*r[a-zA-Z]*|--recursive)\b`, tags("rm -r", "shell destructive", "shell"), WithDecision("deny"),
			WithDenyReason("BLOCKED: rm -r is banned. Remove specific files only, not entire directory trees.")),
		NewPattern(`^(ls|cat|head|tail|wc|grep|rg|file|which|pwd|du|df|sort|uniq|cut|tr|awk|sed|xxd|od|hexdump|sqlite3|tee|diff|stat|realpath|basename|dirname|readlink|md5sum|sha256sum|shasum|lsof|ps|pgrep|jq|yq|id|whoami|hostname|uname|date|env|seq)\b`, tags("read-only", "shell")),
		NewPattern(`^curl\b`, tags("curl", "shell"), WithValidator(isCurlReadOnly)),
		NewPattern(`^xargs\b`, tags("xargs", "shell"), WithValidator(isXargsSafe)),
		NewPattern(`^find\b`, tags("find", "shell"), WithValidator(isFindSafe)),
		NewPattern(`^touch\b`, tags("touch", "shell")),
		NewPattern(`^mkdir\b`, tags("mkdir", "shell")),
		NewPattern(`^cp\s+-[a-zA-Z]*n`, tags("cp -n", "shell")),
		NewPattern(`^ln\s+-[a-eg-zA-Z]*s[a-eg-zA-Z]*\s`, tags("ln -s", "shell")),
		NewPattern(`^(open|xdg-open)\b`, tags("open media", "shell"), WithValidator(isMediaOpenTarget), WithValidatorFallback("")),
		NewPattern(`^(true|false|exit(\s+\d+)?|wait)$`, tags("shell builtin", "shell")),
		NewPattern(`^test\b`, tags("shell builtin", "shell")),
		NewPattern(`^unset\b`, tags("shell vars", "shell")),
		NewPattern(`^(pkill|kill)\b`, tags("process mgmt", "shell")),
		NewPattern(`^eval\b`, tags("eval", "shell")),
		NewPattern(`^(echo|printf)\b`, tags("echo", "shell")),
		NewPattern(`^cd\s`, tags("cd", "shell"), WithValidator(isCurrentRepoWorktreeCD), WithValidatorFallback("")),
		NewPattern(`^(source|\.) `, tags("source", "shell")),
		NewPattern(`^sleep\s`, tags("sleep", "shell")),
		NewPattern(`^[A-Z_][A-Z0-9_]*=\S*$`, tags("var assignment", "shell")),

		// linters / static analysis
		NewPattern(`^shellcheck\b`, tags("shellcheck")),

		// grpc
		NewPattern(`^grpcurl\b`, tags("grpcurl")),

		// brew
		NewPattern(`^brew\s+(install|list|info|search|tap|untap|update|upgrade|outdated|deps|doctor|config|--version)\b`, tags("brew")),

		// media
		NewPattern(`^ffmpeg\b`, tags("ffmpeg")),
		NewPattern(`^ffprobe\b`, tags("ffprobe")),

		// gh (GitHub CLI)
		NewPattern(`^gh\s+(pr|issue|run|release|repo)\s+(view|list|diff|checks|status|comment)\b`, tags("gh read op", "gh")),
		NewPattern(`^gh\s+image\b`, tags("gh image", "gh")),
		NewPattern(`^gh\s+search\s+(code|repos|issues|prs|commits)\b`, tags("gh search", "gh")),
		NewPattern(`^gh\s+repo\s+clone\b`, tags("gh repo clone", "gh")),
		NewPattern(`^gh\s+pr\s+create\b`, tags("gh pr create", "gh"), WithDecision("")),
		NewPattern(`^gh\s+(pr\s+merge|pr\s+close|pr\s+reopen|pr\s+review|pr\s+edit)\b`, tags("gh write op", "gh")),
		NewPattern(`^gh\s+api\b`, tags("gh api", "gh")),

		// go
		NewPattern(`^go\s+mod\s+vendor\b`, tags("go mod vendor", "go"), WithDecision("deny"),
			WithDenyReason("BLOCKED: go mod vendor is banned. It copies all dependencies into the repo.")),
		NewPattern(`^go\s+mod\s+init\b`, tags("go mod init", "go"), WithDecision("")),
		NewPattern(`^go\s+(build|test|vet|list|get|mod\s+(tidy|download)|run|fmt|generate|install|clean|doc|env|version|tool)\b`, tags("go")),
		NewPattern(`^gofmt\b`, tags("gofmt", "go")),
		NewPattern(`^golangci-lint\b`, tags("golangci-lint", "go")),
		NewPattern(`^ginkgo\b`, tags("ginkgo", "go")),

		// gcloud (Google Cloud CLI)
		NewPattern(`^gcloud\s+logging\s+read\b`, tags("gcloud logging", "gcloud")),
		NewPattern(`^gcloud\s+compute\s+\S+\s+(list|describe)\b`, tags("gcloud compute read", "gcloud")),
		NewPattern(`^gcloud\s+container\s+\S+\s+(list|describe|get-credentials)\b`, tags("gcloud container read", "gcloud")),
		NewPattern(`^gcloud\s+auth\s+print-access-token\b`, tags("gcloud auth", "gcloud")),
		NewPattern(`^gcloud\s+config\s+(list|get-value)\b`, tags("gcloud config", "gcloud")),
		NewPattern(`^gcloud\s+projects\s+(list|describe)\b`, tags("gcloud projects", "gcloud")),

		// kubectl (global flags like --context, -n can appear before or after the subcommand)
		NewPattern(`^kubectl\s+.*\b(get|describe|logs|top|version|config|cluster-info|api-resources|api-versions|explain|auth)\b`, tags("kubectl read op", "kubectl")),
		NewPattern(`^kubectl\s+.*\b(apply|create|delete|patch|edit|scale|label|annotate|taint|cordon|uncordon|drain)\b`, tags("kubectl write op", "kubectl")),
		NewPattern(`^kubectl\s+.*\brollout\s+(status|history)\b`, tags("kubectl read op", "kubectl")),
		NewPattern(`^kubectl\s+.*\brollout\s+(restart|undo|pause|resume)\b`, tags("kubectl write op", "kubectl")),
		NewPattern(`^kubectl\s+.*\bport-forward\b`, tags("kubectl port-forward", "kubectl")),
		NewPattern(`^kubectl\s+.*\bexec\b`, tags("kubectl exec", "kubectl")),
		NewPattern(`^kubectl\s+.*\bcp\b`, tags("kubectl cp", "kubectl")),

		// bq (BigQuery)
		NewPattern(`^bq\s+(ls|show|query|head|get-iam-policy|version|help)\b`, tags("bq")),

		// aws
		NewPattern(`^aws\s+(configure|sts|s3|s3api|ecr|ecs|ec2|iam|lambda|cloudwatch|logs|ssm|secretsmanager|dynamodb|sqs|sns|cloudformation|rds|elasticache)\b`, tags("aws")),

		// acli (Atlassian CLI)
		NewPattern(`^acli\b`, tags("acli")),

		// roborev
		NewPattern(`^roborev\s+show\b`, tags("roborev show", "roborev")),
		NewPattern(`^roborev\s+tui\b`, tags("roborev tui", "roborev"), WithDecision("deny"),
			WithDenyReason("BLOCKED: roborev tui is an interactive TUI that cannot run in a non-interactive shell.")),
		NewPattern(`^roborev\s+(list|summary|wait|stream)\b`, tags("roborev read op", "roborev")),
		NewPattern(`^roborev\b`, tags("roborev")),

		// pre-commit (prek)
		NewPattern(`^(prek|pre-commit)\b`, tags("prek")),

		// process-compose
		NewPattern(`^process-compose\s+(process\s+(list|logs|restart|stop|start|info)|project\s+state|logs|port|--help|help)\b`, tags("process-compose")),

		// docker
		NewPattern(`^docker\s+compose\s+(-[^\s]+\s+\S+\s+)*(up|down|run|build|ps|logs|stop|start|restart|pull|config|exec|create|rm|kill|pause|unpause|top|events|port|images|ls)\b`, tags("docker compose", "docker")),
		NewPattern(`^docker\s+(exec|run|build|logs|ps|inspect|images|pull|stop|start|restart|rm|cp|stats|top|port|network|volume|info|context|--help|help|version)\b`, tags("docker")),
		NewPattern(`^docker\s+compose\s+(-[^\s]+\s+\S+\s+)*version\b`, tags("docker compose", "docker")),
		NewPattern(`^docker-compose\b`, tags("docker-compose", "docker")),

		// ruby
		NewPattern(`^rspec\b`, tags("rspec", "ruby")),
		NewPattern(`^rake\b`, tags("rake", "ruby")),
		NewPattern(`^ruby\b`, tags("ruby")),
		NewPattern(`^rails\s+(test|console|runner|db:migrate|db:rollback|db:seed|routes)\b`, tags("rails", "ruby")),
		NewPattern(`^bundle\s+(install|update|exec|check|list|show|outdated|lock)\b`, tags("bundle", "ruby")),
		NewPattern(`^gem\s+(list|search|info|build)\b`, tags("gem", "ruby")),
		NewPattern(`^rubocop\b`, tags("rubocop", "ruby")),
		NewPattern(`^solargraph\b`, tags("solargraph", "ruby")),
		NewPattern(`^standardrb\b`, tags("standardrb", "ruby")),
	}
}
