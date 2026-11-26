---
name: worktree
description: >-
  Manage isolated Git worktrees for branch-based development. Use before writing
  any code to inspect active worktrees, create feature worktrees, verify
  changes, and clean up merged branches.
---

# Git Worktree Workflow for Multi-Agent Coordination

This skill provides standard procedures for discovering repository layout
(`bare` or `subdir`), creating isolated feature worktrees, configuring
per-worktree author identities, and tearing down merged worktrees without
working tree or branch collisions.

For architectural details on the `bare` and `subdir` layouts, `[glen]` Git
configuration, per-worktree config isolation, and recovery procedures, see
[layouts.md](./references/layouts.md).

If a repository is not yet initialized with `glen` (`glen list` reports
`uninitialized`), use the **`worktree-init`** skill
(`../worktree-init/SKILL.md`) to initialize, convert, or repair it (`glen init`
for `bare`, or `glen init --layout=subdir` for in-place `subdir`).

______________________________________________________________________

## Core Principles & Invariants

- **Never Edit Code on the Default Branch**: Always perform feature, bugfix, and
  refactoring work inside an isolated feature worktree (`<root>/<branch-name>`
  in `bare`, or `<repo>/.worktrees/<branch-name>` in `subdir`).
- **Never Run Working-Tree Commands at `<root>/` in `bare` Layout**: In a `bare`
  workspace, `<root>/` contains `<root>/.bare/`, `<root>/.git`
  (`gitdir: ./.bare`), `<root>/main/`, and `<root>/<branch-name>/`. `<root>/`
  has `core.bare = true` and is NOT a working tree. Always target
  `git -C <root>/main` or `git -C <root>/<branch-name>` for working-tree
  commands (`git status`, `git diff`, `git add`, `git commit`, `git checkout`).
- **Direct Branch Folder Naming**: Worktree directories mirror branch names
  directly under the layout's worktrees root:
  - **`bare` layout (`<root>/<branch-name>`)**: `feat/101-sqlite-state` ->
    `<root>/feat/101-sqlite-state`
  - **`subdir` layout (`<repo>/.worktrees/<branch-name>`)**:
    `feat/101-sqlite-state` -> `<repo>/.worktrees/feat/101-sqlite-state`
- **Sandboxed Terminals**: State-changing Git commands (`glen create`,
  `glen remove`, `glen prune`, `glen init`, `git worktree add`,
  `git worktree remove`, `git config --worktree`, `git commit`, `git branch`,
  `git fetch`, `git push`) write to `.bare/` or `.git/` and fail in terminal
  sandboxes that mount Git metadata read-only; re-run them with sandbox
  escalation. Read-only commands (`git status`, `git diff`, `git log`,
  `glen list`) never need it.

______________________________________________________________________

## Fast-Path CLI Commands (`glen` / `git glen`)

Use `glen` (or `git glen`) to manage worktrees deterministically:

```bash
# Initialize, convert, migrate, or repair a repository (`bare` default, or `subdir`)
# Optional --vscode creates/updates <root>/<repo>.code-workspace (kept in sync by create/remove/prune)
glen init [--layout=bare|subdir] [--vscode] [target-dir-or-url] [-C <dir>]

# List active worktrees and layout status (read-only; supports --json)
glen list [-C <workspace-path>] [--json]

# Create an isolated feature worktree (<root>/<branch> in bare, <repo>/.worktrees/<branch> in subdir)
glen create <branch-or-slug> [base-ref] [--issue <id>] [--name "Agent" --email "user+agent@example.com"] [-C <workspace-path>]

# Tear down a merged worktree, clean up empty parent folders, prune locks, and delete its local branch
glen remove <branch-or-slug> [-C <workspace-path>]

# Prune stale worktree metadata and locks
glen prune [-C <workspace-path>]
```

______________________________________________________________________

## Step-by-Step Workflow

### Discover Workspace Layout & Active Worktrees

Inspect the workspace layout and active checkouts:

```bash
glen list -C <workspace-path>
```

If `Layout` is `uninitialized`, initialize first (`glen init --layout=subdir` to
keep an existing clone in-place, or `glen init` to convert a clean clone to
`bare`).

### Create an Isolated Worktree

1. Choose a descriptive branch name (`<branch-name>`):
   - Issue-scoped feature: `feat/<issue-id>-<slug>` (e.g.
     `feat/101-sqlite-state`)
   - General feature: `feat/<task-slug>` (e.g. `feat/worktree-plugin`)
   - Bug fix: `fix/<issue-slug>` (e.g. `fix/sigterm-handling`)
   - Docs / Refactor: `docs/<slug>`, `refactor/<slug>`
1. Provision the worktree via `glen create` (fetches `origin` automatically when
   configured):
   ```bash
   glen create <branch-name> -C <workspace-path>
   ```
   - **Building on a Non-Default Base Branch**: When instructed to build on top
     of an existing non-`main` branch (e.g. a teammate's branch or an unmerged
     stacked base branch), always create a new feature worktree off that base
     branch (`glen create <new-branch> <base-branch>`) rather than editing or
     committing directly onto the base branch's worktree.
   - *(Optional)* Pass
     `--name "<Agent Name>" --email "<user+agent@example.com>"` to isolate
     per-worktree Git author identity.
1. **Validation**: Run `git -C <worktree-path> status` and confirm it reports
   `On branch <branch-name>` with a clean working tree.

### Work Strictly Inside the Worktree

- Target all file edits and shell commands strictly to the created worktree path
  (`<root>/<branch-name>` in `bare`, or `<repo>/.worktrees/<branch-name>` in
  `subdir`).
- Always run formatters (`mdformat` for `.md` files) and test suites with an
  explicit outer timeout:
  ```bash
  timeout 60s just test
  ```

### Commit, Push & Open Pull Request

1. Use the bundled **`commit`** skill (`../commit/SKILL.md`) (or **`shipit`**
   skill `../shipit/SKILL.md` for lightweight fast-path changes) to create
   atomic commits recording both the **Why** and the **What**.
1. Push the branch:
   ```bash
   git -C <worktree-path> push -u origin <branch-name>
   ```
1. Ensure automatic remote branch deletion is enabled on the GitHub repository:
   ```bash
   gh repo edit --delete-branch-on-merge
   ```
1. When explicitly **stacking** a PR on top of a parent PR, also register or
   update the native GitHub Pull Request Stack via
   `POST /repos/<owner>/<repo>/stacks` (or
   `PATCH /repos/<owner>/<repo>/stacks/<stack_number>`) as documented in
   `../commit/SKILL.md`.

### Merge & Post-Merge Cleanup

Keep the worktree alive during code review. Once the pull request is approved
and ready to merge, use the bundled **`submit`** skill (`../submit/SKILL.md`):

1. Merge the PR **without** `--delete-branch` (to prevent local checkout
   collisions with the primary default-branch checkout):
   ```bash
   gh pr merge <pr-number> --squash
   ```
1. Synchronize the default-branch checkout (`<root>/main` in `bare`, or `<repo>`
   in `subdir`) with the merged upstream changes:
   ```bash
   git -C <default-worktree> pull --ff-only origin main
   ```
1. Remove the merged feature worktree, prune stale metadata, and delete the
   local branch:
   ```bash
   glen remove <branch-name> -C <workspace-path>
   ```
1. **Validation**: Run `glen list -C <workspace-path>` and verify that the
   feature worktree is removed and the default branch is at the latest merged
   commit.
