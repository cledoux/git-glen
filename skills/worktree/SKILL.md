---
name: worktree
description: >-
  Manage isolated Git worktrees for branch-based development. Use before writing
  any code to inspect active worktrees, create feature worktrees, verify
  changes, and clean up merged branches.
---

# Git Worktree Workflow for Multi-Agent Coordination

This skill provides standard procedures for discovering repository topology,
creating isolated feature worktrees at `<root>/<branch-name>`, configuring
per-worktree author identities, and tearing down merged worktrees without
working tree or branch collisions.

For deep architectural background on the Pure Bare Repository Root topology,
per-worktree config isolation, and recovery procedures, see
[pure-bare-topology.md](./references/pure-bare-topology.md).

If a repository is not yet configured with `.bare/` + `.git` + `main/`, use the
**`worktree-init`** skill (`../worktree-init/SKILL.md`) to initialize, convert,
or repair it.

______________________________________________________________________

## Core Principles & Invariants

- **Never Edit Code in `main/`**: Always perform feature, bugfix, and
  refactoring work inside an isolated worktree at `<root>/<branch-name>`.
- **Never Run Working-Tree Commands at `<root>/`**: In a Pure Bare topology,
  `<root>/` contains `<root>/.bare/`, `<root>/.git` (`gitdir: ./.bare`),
  `<root>/main/`, and `<root>/<branch-name>/`. While `<root>/.git` allows IDEs
  and metadata commands (`git worktree list`, `git fetch`, `git branch`) to run
  from `<root>/`, `<root>/` has `core.bare = true` and is NOT a working tree.
  Always target `git -C <root>/main` or `git -C <root>/<branch-name>` for
  working-tree commands (`git status`, `git diff`, `git add`, `git commit`,
  `git checkout`).
- **Direct Branch Folder Naming**: Every worktree directory inside `<root>/`
  matches its branch name directly:
  - `main` -> `<root>/main`
  - `feat/finding-ingestion` -> `<root>/feat/finding-ingestion`
  - `feat/101-sqlite-state` -> `<root>/feat/101-sqlite-state`
  - `fix/sigterm-handling` -> `<root>/fix/sigterm-handling`
- **Antigravity Sandbox Rule**: State-changing Git commands (`git worktree add`,
  `git worktree remove`, `git worktree prune`, `git config --worktree`,
  `git commit`, `git branch`, `git fetch`, `git push`) write to `.bare/` and
  MUST be executed with `BypassSandbox: true`. Read-only commands (`git status`,
  `git diff`, `git log`, `git worktree list`) run in standard sandbox mode
  (`BypassSandbox: false`).

______________________________________________________________________

## Fast-Path CLI Commands (`glen` / `git glen`)

Use `glen` (or `git glen`) to manage worktrees deterministically (run with
`BypassSandbox: true` for mutating subcommands):

```bash
# Initialize, convert, migrate, or repair a repository into Pure Bare topology (BypassSandbox: true)
glen init [target-dir-or-url] [-C <dir>]

# List active worktrees and topology paths (sandboxed OK; supports --json)
glen list [-C <workspace-path>] [--json]

# Create an isolated feature worktree at <root>/<branch-name> (BypassSandbox: true)
glen create <branch-or-slug> [base-ref] [--issue <id>] [--name "Agent" --email "user+agent@example.com"] [-C <workspace-path>]

# Tear down a merged worktree, clean up empty parent folders, prune locks, and delete its local branch (BypassSandbox: true)
glen remove <branch-or-slug> [-C <workspace-path>]

# Prune stale worktree metadata and locks (BypassSandbox: true)
glen prune [-C <workspace-path>]
```

______________________________________________________________________

## Step-by-Step Manual Procedure

### 1. Discover Workspace Root & Inspect Active Worktrees

Resolve `<root>` by locating the directory containing `.bare/`, then list active
worktrees (`BypassSandbox: false`):

```bash
git --git-dir=<root>/.bare worktree list
```

### 2. Fetch Latest Remote State & Ensure Bare Defaults

Ensure `<root>/.git` points to `./.bare`, `.bare` has the wildcard fetch refspec
and ergonomic defaults configured, and fetch the latest default branch from
`origin` (or `upstream`) (`BypassSandbox: true`):

```bash
echo "gitdir: ./.bare" > <root>/.git
git --git-dir=<root>/.bare config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
git --git-dir=<root>/.bare config extensions.worktreeConfig true
git --git-dir=<root>/.bare config push.autoSetupRemote true
git --git-dir=<root>/.bare config branch.autoSetupMerge simple
git --git-dir=<root>/.bare config fetch.prune true
git --git-dir=<root>/.bare config worktree.guessRemote true
git --git-dir=<root>/.bare fetch origin
```

### 3. Create an Isolated Worktree

1. Choose a descriptive branch name (`<branch-name>`):
   - Issue-scoped feature: `feat/<issue-id>-<slug>` (e.g.
     `feat/101-sqlite-state`)
   - General feature: `feat/<task-slug>` (e.g. `feat/worktree-plugin`)
   - Bug fix: `fix/<issue-slug>` (e.g. `fix/sigterm-handling`)
   - Docs / Refactor: `docs/<slug>`, `refactor/<slug>`
1. Provision the worktree at `<root>/<branch-name>` from `.bare`
   (`BypassSandbox: true`):
   ```bash
   git --git-dir=<root>/.bare worktree add \
       <root>/<branch-name> \
       -b <branch-name> origin/main
   ```
1. Set `core.bare = false` in the worktree's isolated config
   (`BypassSandbox: true`):
   ```bash
   git -C <root>/<branch-name> config --worktree core.bare false
   ```
1. *(Optional)* If operating under a specific agent persona, isolate Git author
   identity (`BypassSandbox: true`):
   ```bash
   git -C <root>/<branch-name> config --worktree user.name "<Agent Name>"
   git -C <root>/<branch-name> config --worktree user.email "<user+agent@example.com>"
   ```
1. **Validation**: Run `git -C <root>/<branch-name> status`
   (`BypassSandbox: false`) and confirm it reports `On branch <branch-name>`
   with a clean working tree.

### 4. Work Strictly Inside the Worktree

- Target all file edits (`write_to_file`, `replace_file_content`, `view_file`)
  and shell commands (`Cwd`) strictly to `<root>/<branch-name>`.
- Always run formatters (`mdformat` for `.md` files) and test suites with an
  explicit outer timeout:
  ```bash
  timeout 60s make test
  ```

### 5. Commit, Push & Open Pull Request

1. Use the bundled **`commit`** skill (`../commit/SKILL.md`) (or **`shipit`**
   skill `../shipit/SKILL.md` for lightweight fast-path changes) to create
   atomic commits recording both the **Why** and the **What**.
1. Push the branch (`BypassSandbox: true`):
   ```bash
   git -C <root>/<branch-name> push -u origin <branch-name>
   ```
1. Ensure automatic remote branch deletion is enabled on the GitHub repository:
   ```bash
   gh repo edit --delete-branch-on-merge
   ```

### 6. Merge & Post-Merge Cleanup

Keep the worktree alive during code review. Once the pull request is approved
and ready to merge, use the bundled **`submit`** skill (`../submit/SKILL.md`):

1. Merge the PR **without** `--delete-branch` (to prevent local checkout
   collisions with `<root>/main/`):
   ```bash
   gh pr merge <pr-number> --squash
   ```
1. Synchronize `<root>/main/` with the merged upstream changes
   (`BypassSandbox: true`):
   ```bash
   git -C <root>/main pull --ff-only origin main
   ```
1. Remove the merged feature worktree, prune stale metadata, and delete the
   local branch (`BypassSandbox: true`):
   ```bash
   glen remove <branch-name> -C <root>
   # or manually:
   git --git-dir=<root>/.bare worktree remove <root>/<branch-name>
   git --git-dir=<root>/.bare worktree prune --expire now
   git --git-dir=<root>/.bare branch -D <branch-name>
   git --git-dir=<root>/.bare fetch --prune origin
   ```
1. **Validation**: Run `git --git-dir=<root>/.bare worktree list`
   (`BypassSandbox: false`) and verify that `<root>/<branch-name>` is removed
   and `<root>/main` is at the latest merged commit.
