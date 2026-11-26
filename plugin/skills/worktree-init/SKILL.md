---
name: worktree-init
description: >-
  Initialize, convert, migrate, or repair a Git repository for a worktree-based
  workflow. Use when setting up a new repository, converting an existing clone,
  or repairing worktree pointers and locks.
---

# Repository Initialization & Conversion (`worktree-init`)

This skill initializes, converts, migrates, or repairs a Git repository for
isolated worktree development using either of Glen's layouts:

- **`bare` (default):** `.bare/` + `.git` (`gitdir: ./.bare`) + `main/` +
  `<branch-name>/`
- **`subdir` (`--layout=subdir`):** standard `.git/` clone kept in-place +
  `.worktrees/<branch-name>/` (excluded in `.git/info/exclude`)

______________________________________________________________________

## Safety Invariants

- **Dirty Working Copy Guard (`bare` Conversions Only)**: Converting an existing
  standard `.git/` clone or legacy `repo/` layout into `bare` moves working-tree
  files into `<root>/main/` and therefore requires a completely clean working
  tree (`git status --porcelain` empty) and zero active linked worktrees. If
  dirty files exist, either commit/stash first **or** use
  `glen init --layout=subdir` to initialize in-place without moving any files.
- **Layout Preservation on Re-Init / Repair**: Running `glen init` or
  `glen repair` without `--layout` on an already-initialized workspace preserves
  its recorded `glen.layout` (`bare` or `subdir`) and repairs it in-place.

______________________________________________________________________

## Fast-Path Execution (`glen init`)

Initialization mutates `.bare/` or `.git/` metadata, so in a sandboxed terminal
run it with sandbox escalation:

```bash
# Clone a remote repository (`bare` default, or `--layout=subdir`)
glen init [--layout=bare|subdir] git@github.com:example-org/my-project.git

# Initialize, convert, migrate, or repair a local directory
glen init [target-dir]                   # bare layout (or repairs existing layout)
glen init --layout=subdir [target-dir]   # subdir layout in-place (works on dirty clones)

# Optional: Create or update <root>/<repo>.code-workspace with active worktree folders
glen init [--layout=bare|subdir] --vscode [target-dir-or-url]
```

To prefer `subdir` globally whenever `--layout` is omitted, set:

```bash
git config --global glen.defaultLayout subdir
```

______________________________________________________________________

## Initialization Scenarios

### Remote Git URL (`remote-clone`)

- **`bare` (default):** Clones `--bare` into `<target>/.bare`, writes
  `gitdir: ./.bare` to `<target>/.git`, configures `glen.layout = bare` and
  per-worktree `core.bare`, attaches `<target>/<default-branch>`, and symlinks
  top-level agent configs (`.agents`, `.claude`, `AGENTS.md`, `GEMINI.md`,
  `CLAUDE.md`) at `<target>/`.
- **`subdir` (`--layout=subdir`):** Clones normally into `<target>`, configures
  `glen.layout = subdir` and `extensions.worktreeConfig = true`, creates
  `<target>/.worktrees`, and adds `/.worktrees/` to
  `<target>/.git/info/exclude`.

### Existing Standard `.git/` Clone (`clone-convert` / `subdir-init` / `subdir-repair`)

- **`bare` (`glen init <dir>` on unconfigured clean clone):** Verifies zero
  linked worktrees and clean `git status --porcelain`, moves `.git` to `.bare`,
  writes root `.git` pointer, attaches `<dir>/<branch>`, and syncs root agent
  config symlinks.
- **`subdir` (`glen init --layout=subdir <dir>`, or `glen init <dir>` when
  `glen.layout = subdir` is already set):** Leaves all working files untouched
  (safe on dirty trees), repairs worktree pointers, sets `glen.layout = subdir`
  and ergonomic Git defaults, creates `.worktrees/`, adds `/.worktrees/` to
  `.git/info/exclude`, and prunes stale locks.

### Existing `bare` Workspace Repair (`bare-repair`)

When `<target>/.bare` exists: restores `<target>/.git` (`gitdir: ./.bare`), runs
`git worktree repair`, enforces `glen.layout = bare`,
`extensions.worktreeConfig`, per-worktree `core.bare`, and
`remote.origin.fetch`, prunes expired locks, and refreshes root agent symlinks
and any existing `.code-workspace` file.

### Legacy Sibling Layout Migration (`legacy-migrate`)

When `<target>/repo/.git` exists: verifies `<target>/repo` is clean, migrates
`repo/.git` to `<target>/.bare` (`glen.layout = bare`), attaches
`<target>/main`, prunes stale metadata, and syncs root agent links.

### Greenfield Directory (`greenfield`)

When `<target>` does not exist or is empty: initializes either a `bare`
workspace (`.bare/` + `.git` + `main/` + initial commit) or, with
`--layout=subdir`, a standard clone (`.git/` + `.worktrees/` +
`.git/info/exclude` + initial commit).

______________________________________________________________________

## Verification Steps

After running `glen init` or `glen repair`:

1. Run `glen list -C <target-dir>` and confirm `Layout:` reports `bare` or
   `subdir` (not `uninitialized`).
1. For **`bare`**: confirm `<target-dir>/.git` contains `gitdir: ./.bare`,
   `git --git-dir=<target-dir>/.bare config --get glen.layout` returns `bare`,
   and `git -C <target-dir>/main status` is clean.
1. For **`subdir`**: confirm `git -C <target-dir> config --get glen.layout`
   returns `subdir`, `<target-dir>/.git/info/exclude` contains `/.worktrees/`,
   and `git -C <target-dir> status --porcelain` does not show `.worktrees/`.
