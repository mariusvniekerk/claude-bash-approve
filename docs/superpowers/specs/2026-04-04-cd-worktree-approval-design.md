# CD Worktree Approval Design

## Goal

Restrict automatic approval for `cd` so it only applies when the destination is a linked worktree root for the same Git repository as the current working directory. All other `cd` commands should become no-decision.

## Behavior

- Treat the current hook payload `cwd` as the source repository context.
- Auto-approve `cd <dest>` only when:
  - the command is a plain two-argument `cd` invocation,
  - `<dest>` resolves to an existing directory,
  - the destination resolves to the top-level directory of a Git worktree,
  - the destination worktree shares the same Git common directory as the repository containing `cwd`.
- Return no-decision when:
  - `cwd` is not inside a Git repository,
  - `<dest>` is not a Git worktree root,
  - `<dest>` belongs to another repository,
  - the `cd` argument is dynamic or otherwise cannot be resolved safely.

## Implementation Notes

- Thread `cwd` through evaluation so validators can make repository-aware decisions.
- Keep validator failure behavior configurable per pattern so `curl` can still fall back to `ask` while `cd` falls back to no-decision.
- Use Git path identity (`rev-parse --git-common-dir` plus `--show-toplevel`) instead of broad filesystem heuristics.

## Testing

- Approve `cd` from a repo worktree to a linked worktree in the same repo.
- Approve relative-path navigation to that linked worktree.
- Return no-decision for unrelated directories.
- Return no-decision when `cwd` is not in a Git repository.
