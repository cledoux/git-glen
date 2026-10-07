# Git Worktree & Pure Bare Topology Discipline

When operating in any repository that uses the **Pure Bare Repository Root
Topology** (`.bare/` + `.git` + `main/` + `<branch-name>/`), you MUST adhere to
the following rules:

## 1. Topology Recognition & Root Execution Guard

- **Pure Bare Root Structure**: A Pure Bare workspace root (`<root>/`) contains
  only:
  - `<root>/.bare/`: The bare Git object database (`core.bare = true`).
  - `<root>/.git`: A one-line pointer file (`gitdir: ./.bare`) that lets IDEs,
    Antigravity, and Git metadata commands (`git worktree`, `git fetch`,
    `git branch`) recognize `<root>/` as a Git workspace.
  - `<root>/main/`: The primary linked worktree tracking the default branch.
  - `<root>/<branch-name>/`: Isolated, ephemeral feature worktrees matching
    their branch names directly at the workspace root (e.g.,
    `<root>/feat/101-auth/`, `<root>/fix/sigterm/`).
- **Never Run Working-Tree Commands at `<root>/`**: Even with `<root>/.git`
  pointing to `./.bare`, the workspace root `<root>/` is NOT a Git working tree
  (`core.bare = true`). Running working-tree Git commands (`git status`,
  `git diff`, `git add`, `git commit`, `git checkout`) directly at `<root>/`
  fails with `fatal: this operation must be run in a work tree`. Always target a
  specific worktree via `git -C <root>/main ...` or
  `git -C <root>/<branch-name> ...`.

## 2. Mandatory Feature Worktree Isolation

- **Never Modify `main/` Directly for Feature/Bugfix Work**: Do NOT write or
  edit feature, bugfix, or refactoring code directly inside `<root>/main/`.
- **Work Strictly in `<root>/<branch-name>`**: Before writing any code, activate
  the **`worktree`** skill and create an isolated worktree at
  `<root>/<branch-name>`. All file edits, builds, linters, and test suites MUST
  be executed strictly inside that worktree directory.
- **Direct Branch Folder Naming**: Worktree directories inside `<root>/` MUST
  mirror the branch name directly (e.g., branch `feat/finding-ingestion` ->
  `<root>/feat/finding-ingestion`; branch `feat/101-sqlite-state` ->
  `<root>/feat/101-sqlite-state`; branch `main` -> `<root>/main`).

## 3. Required Bare & Per-Worktree Plumbing Configuration

- **Root `.git` Pointer**: Every Pure Bare workspace root SHOULD have
  `echo "gitdir: ./.bare" > <root>/.git`.
- **Bare Repository Configs**: Every `.bare` repository database MUST be
  configured with:
  - `git --git-dir=<root>/.bare config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"`
  - `git --git-dir=<root>/.bare config extensions.worktreeConfig true`
  - `git --git-dir=<root>/.bare config push.autoSetupRemote true`
  - `git --git-dir=<root>/.bare config branch.autoSetupMerge simple`
  - `git --git-dir=<root>/.bare config fetch.prune true`
  - `git --git-dir=<root>/.bare config worktree.guessRemote true`
- **Per-Worktree `core.bare` Override**: Every linked worktree attached to
  `.bare` MUST set `core.bare = false` in its isolated worktree config:
  - `git -C <worktree-path> config --worktree core.bare false`
- **Per-Worktree Author Isolation**: When operating under a specific agent or
  bot identity, configure `user.name` and `user.email` using
  `git -C <worktree-path> config --worktree` so credentials never leak across
  `main/` or peer worktrees.

## 4. Antigravity Terminal Sandbox Compatibility

- The Antigravity terminal sandbox mounts `.git` and `.bare` directories
  read-only and blocks `reftable` transaction locks (causing
  `Read-only file system` or `fatal: reftable: transaction prepare: I/O error`).
- **Read-Only Git Commands**: Run `git status`, `git diff`, `git log`, and
  `git worktree list` in standard sandbox mode (`BypassSandbox: false`).
- **State-Changing Git Commands**: Always set `BypassSandbox: true` when
  executing commands that mutate `.bare/` or worktree metadata
  (`git worktree add`, `git worktree remove`, `git worktree prune`,
  `git worktree repair`, `git config --worktree`, `git commit`, `git branch`,
  `git fetch`, `git pull`, `git push`).

## 5. Pull Request Merge & Cleanup Safety

- **Enable Server-Side Branch Deletion**: Ensure the GitHub repository has
  `delete_branch_on_merge` enabled (`gh repo edit --delete-branch-on-merge`).
- **Omit `--delete-branch` on Merge**: When merging a pull request from a
  worktree-backed repository, run `gh pr merge <pr-number> --squash` (or
  `--rebase`) **WITHOUT** passing `--delete-branch`. Passing `--delete-branch`
  causes `gh` to attempt a local `git checkout main` inside the feature
  worktree, which fails because `main` is already checked out in `<root>/main/`.
- **Clean Up Worktrees Post-Merge**: After merging, fast-forward `<root>/main/`,
  remove the merged worktree (`glen remove <branch-name>` or
  `git --git-dir=<root>/.bare worktree remove <root>/<branch-name>`), prune
  stale metadata (`git --git-dir=<root>/.bare worktree prune --expire now`), and
  delete the local feature branch
  (`git --git-dir=<root>/.bare branch -D <branch>`).
