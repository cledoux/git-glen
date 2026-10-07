---
name: worktree-init
description: >-
  Initialize, convert, migrate, or repair a Git repository for a worktree-based
  workflow. Use when setting up a new repository, converting an existing clone,
  or repairing worktree pointers and locks.
---

# Repository Initialization & Conversion (`worktree-init`)

This skill establishes, converts, migrates, or repairs a repository into a
symmetric **Pure Bare Repository Root Topology** (`.bare/` + `.git` + `main/` +
`<branch-name>/`) across 5 supported target scenarios.

______________________________________________________________________

## Safety Invariant: Zero-Tolerance Dirty Working Copy Guard

Before converting a standard `.git/` clone or migrating a legacy `repo/`
directory in-place, you MUST verify that the working tree is completely clean
(`git status --porcelain` produces empty output).

- **If any uncommitted changes, staged changes, or untracked files exist**:
  **STOP IMMEDIATELY** and do NOT modify or restructure the directory. Instruct
  the user to commit (`git add -A && git commit -m "WIP"`) or stash
  (`git stash --include-untracked`) their changes first.
- Never use destructive `--force` flags to discard uncommitted user work during
  a topology conversion.

______________________________________________________________________

## Fast-Path Execution (`glen init`)

Run `glen init` (or `git glen init`) to execute the repository-level
initialization, conversion, migration, or repair logic. Because initialization
mutates `.bare/` and worktree metadata, always run with `BypassSandbox: true`:

```bash
# Scenario A: Clone a remote repository into Pure Bare topology
glen init git@github.com:example-org/my-project.git
# (or specify a target parent directory with -C <dir>)
glen init -C <parent-dir> git@github.com:example-org/my-project.git

# Scenario B / C / D / E: Convert, repair, migrate, or init a local directory
glen init [target-dir]
```

______________________________________________________________________

## The 5 Initialization Scenarios

### Scenario A: Remote Git URL (`remote-clone`)

When given a remote URL (`git@...`, `https://...`, `ssh://...`):

1. Clone bare database and write root `.git` pointer:
   ```bash
   git clone --bare <repo-url> <target-dir>/.bare
   echo "gitdir: ./.bare" > <target-dir>/.git
   ```
1. Configure mandatory fetch refspec, per-worktree config isolation, and default
   bare ergonomics:
   ```bash
   git --git-dir=<target-dir>/.bare config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
   git --git-dir=<target-dir>/.bare config extensions.worktreeConfig true
   git --git-dir=<target-dir>/.bare config push.autoSetupRemote true
   git --git-dir=<target-dir>/.bare config branch.autoSetupMerge simple
   git --git-dir=<target-dir>/.bare config fetch.prune true
   git --git-dir=<target-dir>/.bare config worktree.guessRemote true
   ```
1. Dynamically probe the remote default branch (`main`, `master`, `trunk`) via
   `git --git-dir=<target-dir>/.bare symbolic-ref refs/remotes/origin/HEAD`.
1. Attach primary `<target-dir>/main` worktree and configure upstream tracking:
   ```bash
   git --git-dir=<target-dir>/.bare worktree add <target-dir>/main origin/<default-branch>
   git -C <target-dir>/main config --worktree core.bare false
   git -C <target-dir>/main branch --set-upstream-to=origin/<default-branch> <default-branch>
   ```

### Scenario B: Standard `.git` Clone In-Place Conversion (`clone-convert`)

When `<target-dir>/.git` is a standard clone directory:

1. **Mandatory Cleanliness Check**: Run
   `git -C <target-dir> status --porcelain`. Abort if non-empty.
1. Resolve active branch (`git -C <target-dir> rev-parse --abbrev-ref HEAD`).
1. Move `<target-dir>/.git` to `<target-dir>/.bare` and configure bare settings:
   ```bash
   mv <target-dir>/.git <target-dir>/.bare
   git --git-dir=<target-dir>/.bare config core.bare true
   git --git-dir=<target-dir>/.bare config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
   git --git-dir=<target-dir>/.bare config extensions.worktreeConfig true
   git --git-dir=<target-dir>/.bare config push.autoSetupRemote true
   git --git-dir=<target-dir>/.bare config branch.autoSetupMerge simple
   git --git-dir=<target-dir>/.bare config fetch.prune true
   git --git-dir=<target-dir>/.bare config worktree.guessRemote true
   ```
1. Remove clean root working files (preserving `.bare`), write the root `.git`
   pointer (`echo "gitdir: ./.bare" > <target-dir>/.git`), and attach
   `<target-dir>/main`:
   ```bash
   echo "gitdir: ./.bare" > <target-dir>/.git
   git --git-dir=<target-dir>/.bare worktree add <target-dir>/main <branch>
   git -C <target-dir>/main config --worktree core.bare false
   ```

### Scenario C: Existing Pure Bare Repair (`bare-repair`)

When `<target-dir>/.bare` already exists:

1. Ensure `<target-dir>/.git` contains `gitdir: ./.bare`.
1. Heal relative or relocated worktree pointers:
   ```bash
   git -C <target-dir>/main worktree repair
   git --git-dir=<target-dir>/.bare worktree repair
   ```
1. Enforce `remote.origin.fetch = +refs/heads/*:refs/remotes/origin/*`,
   `extensions.worktreeConfig = true`, `push.autoSetupRemote = true`,
   `branch.autoSetupMerge = simple`, `fetch.prune = true`, and
   `worktree.guessRemote = true`.
1. Prune expired worktree locks and dangling metadata:
   ```bash
   git --git-dir=<target-dir>/.bare worktree prune --expire now
   ```

### Scenario D: Legacy Sibling Layout Migration (`legacy-migrate`)

When `<target-dir>/repo/.git` exists:

1. **Mandatory Cleanliness Check**: Run
   `git -C <target-dir>/repo status --porcelain`. Abort if non-empty.
1. Resolve active branch in `<target-dir>/repo`.
1. Move `<target-dir>/repo/.git` to `<target-dir>/.bare`, set
   `core.bare = true`, configure bare defaults, remove `<target-dir>/repo`, and
   write `echo "gitdir: ./.bare" > <target-dir>/.git`.
1. Attach `<target-dir>/main`, set `core.bare = false` on `main`, and run
   `worktree prune --expire now`.

### Scenario E: Greenfield Empty Directory (`greenfield`)

When `<target-dir>` does not exist or is empty:

1. Initialize bare database, write `<target-dir>/.git`, and configure bare
   defaults:
   ```bash
   mkdir -p <target-dir>
   git init --bare -b main <target-dir>/.bare
   echo "gitdir: ./.bare" > <target-dir>/.git
   git --git-dir=<target-dir>/.bare config extensions.worktreeConfig true
   ```
1. Provision `<target-dir>/main`, set `core.bare = false`, and create an initial
   `README.md` commit.

______________________________________________________________________

## Verification Steps

After running initialization or conversion, verify the workspace
(`BypassSandbox: false`):

1. `cat <target-dir>/.git` returns `gitdir: ./.bare`.
1. `git --git-dir=<target-dir>/.bare config --get-all remote.origin.fetch`
   includes `+refs/heads/*:refs/remotes/origin/*`.
1. `git --git-dir=<target-dir>/.bare config --get extensions.worktreeConfig`
   returns `true`.
1. `git -C <target-dir> worktree list` shows `<target-dir>/.bare` and
   `<target-dir>/main`.
1. `git -C <target-dir>/main status` exits `0` with a clean working tree.
