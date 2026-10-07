---
name: submit
description: >-
  Merge approved pull requests, synchronize upstream branches, and clean up
  local and remote feature branches.
---

# Pull Request Submission & Cleanup Workflow

Merge approved pull requests, verify CI and review approvals, synchronize the
default branch (`main` / `trunk`), and clean up local and remote feature
worktrees and branches.

______________________________________________________________________

## Core Invariants & Multi-Agent Safety Rules

1. **Strict Single-PR Scoping**: `submit` MUST strictly submit the single pull
   request corresponding to the current working branch
   (`git -C <worktree-path> branch --show-current`).
1. **Never Merge Unowned PRs**: In multi-agent or multi-branch repositories,
   NEVER scan `gh pr list` to discover and merge other open pull requests. Each
   agent session owns exactly ONE feature branch/PR.
1. **No Autonomous Stacked Merges**: Never autonomously merge multiple PRs in a
   single invocation unless the user explicitly passes multiple PR numbers
   (e.g., `/submit #83 #85`).
1. **Isolated Worktree Cleanup**: Only delete the local branch and worktree
   corresponding strictly to the merged branch (`<root>/<branch-name>`). NEVER
   touch or delete adjacent peer worktrees inside `<root>/`.

______________________________________________________________________

## Workflow Steps

### Step 1: Active Branch & PR Discovery

1. Discover repository topology and VCS markers (`.bare` + `.git` + `main` Pure
   Bare topology, standard `.git` clone, `.jj`, or `.hg`).
1. Identify the active branch in the target worktree (`BypassSandbox: false`):
   ```bash
   git -C <worktree-path> branch --show-current
   ```
   If on `main` or `trunk` without a branch or PR argument, **STOP** and ask the
   user which specific PR or branch to submit.
1. Look up the PR associated strictly with the active branch:
   ```bash
   gh pr view --head "<branch-name>" --json number,title,state,reviewDecision,mergeable,headRefName
   ```
   Verify that `headRefName` matches `<branch-name>`.

______________________________________________________________________

### Step 2: Pre-Merge Verification & Readiness Checks

Before initiating merge:

1. **CI / Presubmit Status**:
   - Check status of automated checks: `gh pr checks "<branch-name>"`
   - Verify all required CI checks have passed.
   - If checks are pending or failing, **STOP** and inform the user before
     attempting to merge.
1. **Review Approvals**:
   - Verify review approval status
     (`gh pr view "<branch-name>" --json reviewDecision,reviews`).
   - If changes are requested or required approvals are missing, warn the user
     and request confirmation.
1. **Working Tree Cleanliness**:
   - Run `git -C <worktree-path> status -uall` (`BypassSandbox: false`).
   - Ensure the working tree is clean with no uncommitted or untracked changes.
   - If uncommitted changes exist, **STOP** and ask the user how to handle them.

______________________________________________________________________

### Step 3: Execute PR Merge (CLI-First)

1. **Ensure Remote Branch Auto-Deletion**:

   - Verify that the target repository has `delete_branch_on_merge` enabled:
     ```bash
     gh repo edit --delete-branch-on-merge
     ```
   - Enabling this setting ensures GitHub automatically deletes the remote
     branch server-side on PR merge, avoiding local worktree checkout
     collisions.

1. **Merge the Pull Request via GitHub CLI**:

   - Prefer command-line tools over web UIs.
   - Execute merge **without** `--delete-branch` (which triggers local
     `git checkout main` errors when `main` is checked out in `<root>/main/`):
     - Squash merge: `gh pr merge <pr-number> --squash`
     - Rebase merge: `gh pr merge <pr-number> --rebase`
   - Default to `--squash` or `--rebase` to preserve a clean, linear Git history
     on `main`. Avoid merge commits unless explicitly mandated by repository
     policy.
   - **Remote Branch Deletion Fallback**: If `delete_branch_on_merge` is not
     enabled on the repository and the remote branch remains after merge:
     ```bash
     git -C <root>/main push origin --delete "<branch-name>"
     ```

1. **Merge Conflict Handling**:

   - If the merge fails due to remote conflicts, **STOP** and present
     conflict-resolution options to the user.

______________________________________________________________________

### Step 4: Upstream Synchronization & Local Worktree / Branch Cleanup

1. **If operating in a Pure Bare worktree topology (`<root>/.bare` +
   `<root>/main` + `<root>/<branch-name>`)** (`BypassSandbox: true`):
   - Synchronize the primary `<root>/main` worktree with upstream:
     ```bash
     git -C <root>/main pull --ff-only origin main
     ```
   - Remove the merged feature worktree, prune stale metadata, and delete the
     local feature branch:
     ```bash
     glen remove "<branch-name>" -C <root>
     # or manually:
     git --git-dir=<root>/.bare worktree remove <root>/<branch-name>
     git --git-dir=<root>/.bare worktree prune --expire now
     git --git-dir=<root>/.bare branch -D "<branch-name>"
     git --git-dir=<root>/.bare fetch --prune origin
     ```
1. **If operating in a standard single-worktree clone** (`BypassSandbox: true`):
   - Switch to default branch: `git checkout main`
   - Pull latest upstream commits: `git pull --ff-only origin main`
   - Delete local feature branch: `git branch -D "<branch-name>"`
   - Prune remote references: `git fetch --prune origin`

______________________________________________________________________

### Step 5: Final Verification & Summary

1. Verify clean repository and worktree status (`BypassSandbox: false`):
   - `git -C <root>/main status`
   - `git -C <root>/main log -n 3 --oneline`
   - `git --git-dir=<root>/.bare worktree list`
1. Present a concise completion summary to the user:
   - Merged PR URL, number, and title
   - Merge strategy used (e.g., Squash & Merge)
   - Local feature worktree and branch deleted
   - Current commit on `main`
