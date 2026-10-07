---
trigger: model_decision
description: "Mandatory invariants, execution guards, and skill redirection for Pure Bare Git repositories and worktrees."
---

# Git Worktree & Pure Bare Topology Discipline

When operating in any repository utilizing the **Pure Bare Repository Root
Topology** (`.bare/` + `.git` + `main/` + `<branch-name>/`), you MUST adhere to
the following invariants and rules:

## 1. Root Execution Guard

- **Bare Root Structure**: The workspace root `<root>/` contains only `.bare/`,
  `.git` (`gitdir: ./.bare`), `main/`, and `<branch-name>/`.
- `<root>/` has `core.bare = true` and is NOT a working tree.
- **NEVER execute working-tree Git commands** (`git status`, `git diff`,
  `git add`, `git commit`, `git checkout`) directly at `<root>/` (fails with
  `fatal: this operation must be run in a work tree`). Always pass
  `-C <root>/main` or `-C <root>/<branch-name>`.

## 2. Mandatory Feature Worktree Isolation & Skill Delegation

- **NEVER edit code directly inside `main/`**: Before writing or modifying any
  code for features, bugfixes, or refactoring, you MUST activate the
  **`worktree`** skill (`/worktree`) or execute `glen create <branch-name>` to
  provision an isolated worktree at `<root>/<branch-name>`.
- All code modifications, tests, linters, and builds MUST execute strictly
  inside `<root>/<branch-name>`.
- Worktree directories directly mirror branch names (e.g. branch `feat/auth` ->
  `<root>/feat/auth`).

## 3. Mandatory Glen & Skill Redirection for Worktree Lifecycle

- **NEVER perform manual, ad-hoc worktree plumbing**: You MUST use the
  **`worktree`** skill or the `glen` CLI for all worktree lifecycle operations:
  - **Inspecting & Listing**: Run `glen list` or use the `worktree` skill.
  - **Creation**: Run `glen create <branch-name>` or use the `worktree` skill.
  - **Teardown & Cleanup**: Run `glen remove <branch-name>` and `glen prune` or
    use the `submit` skill.
  - **Repository Init / Conversion / Repair**: Use the **`worktree-init`** skill
    or `glen init`.
  - **Commits & PRs**: Use the **`commit`** and **`submit`** skills.

## 4. Antigravity Terminal Sandbox Policy

- The terminal sandbox mounts `.bare/` and `.git/` read-only and blocks
  `reftable` transaction locks.
- **Read-Only Git Commands**: Run `git status`, `git diff`, `git log`, and
  `glen list` in standard sandbox mode (`BypassSandbox: false`).
- **State-Changing Git Commands**: Always set `BypassSandbox: true` when
  executing commands that mutate `.bare/` or worktree metadata (`glen create`,
  `glen remove`, `glen prune`, `glen init`, `git worktree add`,
  `git worktree remove`, `git worktree prune`, `git worktree repair`,
  `git config --worktree`, `git commit`, `git branch`, `git fetch`, `git push`).

## 5. Safe Pull Request Merges

- Ensure the remote repository has `delete_branch_on_merge` enabled
  (`gh repo edit --delete-branch-on-merge`).
- When merging via GitHub CLI, execute `gh pr merge <pr> --squash` **WITHOUT**
  `--delete-branch` (passing `--delete-branch` forces a local
  `git checkout main` inside the worktree, colliding with `main/`).
- After merging, clean up via `glen remove <branch-name>`.
