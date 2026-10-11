---
trigger: model_decision
description: "Mandatory invariants and skill redirection for Pure Bare Git repositories and worktrees."
---

# Git Worktree & Pure Bare Topology Discipline

This workspace uses the **Pure Bare Repository Root Topology** (`.bare/` +
`.git` + `main/` + `<branch-name>/`). Glen's hooks block the most damaging
mistakes (edits inside `main/`, `gh pr merge --delete-branch`); the rules below
explain the workflow so you never hit them.

## 1. Root Execution Guard

- `<root>/` contains only `.bare/`, `.git` (`gitdir: ./.bare`), `main/`, and
  `<branch-name>/` worktrees. It has `core.bare = true` and is NOT a working
  tree.
- **NEVER run working-tree Git commands** (`git status`, `git diff`,
  `git add`, `git commit`, `git checkout`) at `<root>/`. Always pass
  `-C <root>/main` or `-C <root>/<branch-name>`.

## 2. Feature Worktree Isolation

- **NEVER edit code inside `main/`** (or whichever worktree tracks the default
  branch). Before modifying code, run `glen create <branch-name>` or use the
  **`worktree`** skill to provision `<root>/<branch-name>`.
- Run all edits, tests, linters, and builds inside `<root>/<branch-name>`.
- Worktree directories mirror branch names (branch `feat/auth` ->
  `<root>/feat/auth`).

## 3. Use Glen & Skills for the Lifecycle

Never perform ad-hoc worktree plumbing:

- **List / inspect**: `glen list` or the **`worktree`** skill.
- **Create**: `glen create <branch-name>` or the **`worktree`** skill.
- **Teardown**: `glen remove <branch-name>` and `glen prune`, or the
  **`submit`** skill.
- **Init / convert / repair**: `glen init` or the **`worktree-init`** skill.
- **Commits & PRs**: the **`commit`**, **`submit`**, and **`shipit`** skills.

## 4. Sandboxed Terminals

Commands that write to `.bare/` or worktree metadata (`glen create`,
`glen remove`, `glen prune`, `glen init`, `git worktree ...`, `git commit`,
`git branch`, `git fetch`, `git push`) fail in terminal sandboxes that mount
`.bare/` read-only. If one fails with a permission or lock error, re-run it with
sandbox escalation. Read-only commands (`git status`, `git diff`, `git log`,
`glen list`) never need it.

## 5. Safe Pull Request Merges

- Enable server-side branch cleanup once per repository:
  `gh repo edit --delete-branch-on-merge`.
- Merge with `gh pr merge <pr> --squash` **without** `--delete-branch` (it
  forces a local `git checkout main`, colliding with `main/`).
- After merging, clean up with `glen remove <branch-name>`.
