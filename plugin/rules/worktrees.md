---
trigger: model_decision
description: "Mandatory invariants and skill redirection for glen-managed Git repositories and worktrees (bare and subdir layouts)."
---

# Git Worktree Isolation Discipline

Glen manages isolated Git worktrees across two repository layouts configured in
`$GIT_COMMON_DIR/config` (`[glen] layout = bare | subdir`):

- **`bare` layout (default):** `<root>/.bare/` + `<root>/.git`
  (`gitdir: ./.bare`) + `<root>/main/` + `<root>/<branch-name>/`.
- **`subdir` layout:** standard `<repo>/.git/` clone in-place +
  `<repo>/.worktrees/<branch-name>/` (excluded in `.git/info/exclude`).

Glen's hooks block direct edits on the default branch inside initialized
workspaces and block `gh pr merge --delete-branch`. Follow the rules below so
you never hit them.

## Bare Root Execution Guard (`bare` Layout)

- In a `bare` workspace, `<root>/` contains `.bare/`, `.git`
  (`gitdir: ./.bare`), `main/`, and `<branch-name>/` worktrees. `<root>/` has
  `core.bare = true` and is NOT a working tree.
- **NEVER run working-tree Git commands** (`git status`, `git diff`, `git add`,
  `git commit`, `git checkout`) at `<root>/`. Always pass `-C <root>/main` or
  `-C <root>/<branch-name>`.

## Feature Worktree Isolation (`bare` & `subdir` Layouts)

- **NEVER edit code on the default branch** (`<root>/main` in `bare`, or
  `<repo>` when checked out on `main` in `subdir`). Before modifying code, run
  `glen create <branch-name>` or use the **`worktree`** skill to provision an
  isolated feature worktree (`<root>/<branch-name>` in `bare`, or
  `<repo>/.worktrees/<branch-name>` in `subdir`).
- Run all edits, tests, linters, and builds inside the feature worktree.
- If a standard Git clone is not yet initialized with `glen`, run
  `glen init --layout=subdir` (in-place, preserves dirty working trees) or
  `glen init` (converts a clean clone to `bare`) before calling `glen create`.

## Use Glen & Skills for the Lifecycle

Never perform ad-hoc worktree plumbing:

- **List / inspect**: `glen list` or the **`worktree`** skill.
- **Create**: `glen create <branch-name>` or the **`worktree`** skill.
- **Teardown**: `glen remove <branch-name>` and `glen prune`, or the
  **`submit`** skill.
- **Init / convert / repair**: `glen init [--layout=bare|subdir]` or the
  **`worktree-init`** skill.
- **Commits & PRs**: the **`commit`**, **`submit`**, and **`shipit`** skills.

## Sandboxed Terminals

Commands that write to `.bare/`, `.git/`, or worktree metadata (`glen create`,
`glen remove`, `glen prune`, `glen init`, `git worktree ...`, `git commit`,
`git branch`, `git fetch`, `git push`) fail in terminal sandboxes that mount Git
metadata directories read-only. If one fails with a permission or lock error,
re-run it with sandbox escalation. Read-only commands (`git status`, `git diff`,
`git log`, `glen list`) never need it.

## Safe Pull Request Merges

- Enable server-side branch cleanup once per repository:
  `gh repo edit --delete-branch-on-merge`.
- Merge with `gh pr merge <pr> --squash` **without** `--delete-branch` (passing
  `--delete-branch` forces a local `git checkout main`, which collides when
  `main` is checked out in the primary worktree).
- After merging, clean up with `glen remove <branch-name>`.

## Parallel Subagent & Swarm Worktree Lifecycle

When `glen.autoIsolate` is active (on by default in `glen`-initialized
workspaces, or globally in all Git repositories when
`git config --global glen.autoIsolate true` is set), `glen hook` automatically
upgrades mutating `invoke_subagent` and `swarm` (`action: "add"`) calls with
omitted or `"inherit"` workspace settings to `workspace: "share"` (while leaving
read-only researchers, reviewers, auditors, and verifiers untouched):

- **Start waves from a clean checkout**: Worktree creation replays uncommitted
  staged, unstaged, and untracked edits from the coordinator's checkout into
  each newly forked worker worktree. Commit or stash coordinator edits before
  spawning a parallel wave so workers start cleanly.
- **Commit inside each worker**: Each mutating worker MUST commit its changes on
  its own isolated worktree branch before reporting completion.
- **Integrate before killing workers**: Killing a `workspace: "share"` worker
  deletes both its worktree directory and its local Git branch
  (`git branch -D`). Always merge, cherry-pick, or push a worker's branch
  **before** killing that worker.
