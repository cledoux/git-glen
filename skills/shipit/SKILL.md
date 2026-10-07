---
name: shipit
description: >-
  Fast-path atomic commit and direct push workflow, bypassing PR and branch
  overhead for single commits, documentation, ADRs, or lightweight changes.
---

# Fast-Path Ship Workflow

Execute a direct, low-overhead ship workflow for self-contained single commits,
documentation, ADRs, and lightweight changes. Bypasses the overhead of pull
requests while maintaining strict quality, atomic commit structure, and
repository safety.

______________________________________________________________________

## Workflow Steps

### Step 1: VCS Discovery & Working Copy Inspection

1. Discover repository topology and VCS markers (`.bare` + `.git` + `main` Pure
   Bare topology, standard `.git` clone, `.jj`, or `.hg`).
1. Run working tree status checks (`BypassSandbox: false`):
   - `git -C <worktree-path> status -uall` and
     `git -C <worktree-path> diff --stat`
1. Identify all modified, untracked, and deleted files.

______________________________________________________________________

### Step 2: Safety & Loose Threads Audit

Before staging or committing:

1. **Foreign Work Guard**:
   - If there are modifications or untracked files not belonging to the current
     task, **STOP** and ask the user before touching them.
1. **Loose Threads Audit**:
   - Verify the task is complete with zero dangling TODOs, broken configs, or
     half-finished code.
   - If loose threads remain, finish them or ask the user before proceeding.

______________________________________________________________________

### Step 3: Quality Audits & Standard Checks

Ensure all pre-commit hygiene checks pass:

1. **Formatters & Linters**:
   - Run relevant formatters (e.g., `mdformat <file.md>`, `goimports`, `gofmt`,
     etc.).
   - Run project linters (e.g., `make lint`, `go vet ./...`).
1. **Automated Tests**:
   - Run relevant test suites with a defensive timeout (e.g.,
     `timeout 60s make test`) and verify all tests pass.
1. **Repository Hygiene**:
   - Ensure scratch files, build artifacts, test dumps, or credentials are not
     staged.

______________________________________________________________________

### Step 4: Construct Atomic Commit

Construct a structured commit adhering to the Why/What standard:

```text
<Concise one-line summary describing the change>

<Paragraph or bullets explaining WHY this change was made, the context, and motivation.>

<Paragraph or bullets detailing WHAT was modified, added, or removed.>
```

______________________________________________________________________

### Step 5: Fast-Path Merge & Direct Push

All state-changing Git commands (`add`, `commit`, `pull`, `merge`, `push`,
`worktree remove`, `branch -d`) MUST run with `BypassSandbox: true`:

1. **If on `main` (`<root>/main` or standard clone)**:
   - Stage target files: `git -C <main-path> add ...`
   - Commit directly: `git -C <main-path> commit -m "..."`
   - Push to remote: `git -C <main-path> push origin main`
1. **If in an isolated feature worktree (`<root>/<branch-name>`)**:
   - Stage and commit the changes inside `<root>/<branch-name>`.
   - Fast-forward `<root>/main` from remote and merge the feature branch:
     ```bash
     git -C <root>/main pull --ff-only origin main
     git -C <root>/main merge --ff-only <branch-name>
     git -C <root>/main push origin main
     ```
   - Clean up the feature worktree and local branch:
     ```bash
     glen remove <branch-name> -C <root>
     ```
1. **If on a feature branch in a standard single-worktree clone**:
   - Commit the changes on the branch.
   - Switch to `main`: `git checkout main`
   - Pull/verify upstream: `git pull --ff-only origin main`
   - Merge feature branch with fast-forward:
     `git merge --ff-only <feature-branch>`
   - Push to remote: `git push origin main`
   - Clean up local feature branch: `git branch -d <feature-branch>`

______________________________________________________________________

### Step 6: Verification

1. Verify final log and clean tree (`BypassSandbox: false`):
   - `git -C <main-path> log -n 1 --oneline`
   - `git -C <main-path> status`
1. Confirm to the user that the change has been shipped and pushed to upstream.
