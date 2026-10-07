---
name: commit
description: >-
  Execute branch-based atomic commit workflow and open or update pull requests
  for review.
---

# Commit & Review Preparation Workflow

Inspect the working copy, partition changes into one or more logical review
units, construct structured atomic commits on dedicated feature branches or
isolated worktrees, push to remote, and open pull requests for code review.

______________________________________________________________________

## Workflow Steps

### Step 1: VCS Discovery & Working Copy Inspection

1. Discover repository topology and VCS markers (`.bare` + `.git` + `main` Pure
   Bare worktree topology, standard `.git` clone, `.jj`, or `.hg`).
1. Run detailed working tree status checks inside the target worktree
   (`BypassSandbox: false`):
   - Git worktree: `git -C <worktree-path> status -uall` and
     `git -C <worktree-path> diff --stat`
   - Jujutsu: `jj status` and `jj diff --stat`
   - Mercurial: `hg status`
1. Identify all untracked, modified, and deleted files.

______________________________________________________________________

### Step 2: Foreign Work Guard, Loose Threads Audit & PR Decomposition

Before staging, branching, or committing, perform critical safety and planning
checks:

1. **Foreign / Unrelated Changes Guard**:
   - If there are modifications or untracked files in the working copy that the
     current session is **not** responsible for, **STOP IMMEDIATELY** and ask
     the user for guidance before touching them.
1. **Loose Threads Audit**:
   - Verify that the active task is genuinely finished with no dangling loose
     threads (e.g., half-implemented functions, unresolved temporary TODOs,
     broken configurations, missing dependencies).
   - **If any loose threads remain, STOP and finish them or consult the user
     before proceeding.**
1. **PR Decomposition & Scope Planning**:
   - Evaluate whether the working changes represent a **single PR** or should be
     partitioned into **multiple PRs** (e.g., distinct features, standalone
     refactorings, documentation vs. code, or multi-phase tasks).
   - **Single PR**: All changes form a cohesive, single deliverable.
   - **Multiple Independent PRs**: Changes address distinct issues or
     independent subsystems. Plan separate feature worktrees/branches based off
     the target base branch (e.g., `origin/main`).
   - **Stacked / Dependent PRs**: Changes where subsequent work depends on a
     foundational change. Plan chained branches (e.g., `feat/part-2` based on
     `feat/part-1`) and configure the PR base target accordingly.
   - For non-trivial multi-PR splits, confirm the branch names, scope division,
     and dependency plan with the user before proceeding.

______________________________________________________________________

### Step 3: Quality Audits & Standard Checks

Ensure standard project hygiene and verification checks pass:

1. **Formatters & Linters**:
   - Run relevant code formatters and linters for the modified file types (e.g.,
     `mdformat` for Markdown, `gofmt`/`goimports`, project-configured linters).
1. **Automated Tests**:
   - Run relevant test suites for the modified components with an outer timeout
     (e.g., `timeout 60s make test`) and verify that all tests pass.
1. **Repository & Security Hygiene**:
   - Ensure temporary scratch files, build artifacts, test dumps, or sensitive
     credentials are not accidentally staged.

______________________________________________________________________

### Step 4: Worktree / Branch Creation & Atomic Commit Construction

For each planned PR:

1. **Ensure Isolated Feature Worktree or Branch**:

   - **Pure Bare Topology (`<root>/.bare` + `<root>/<branch-name>`)**: Ensure
     changes reside in an isolated feature worktree at `<root>/<branch-name>`
     (using the **`worktree`** skill or `glen create <branch-name> -C <root>`
     with `BypassSandbox: true`).
   - **Standard Clone**: Branch off the appropriate base
     (`git checkout -b <branch-name> [base-branch]` with `BypassSandbox: true`).
   - Use clear, descriptive branch names (e.g., `feat/<name>`, `fix/<name>`,
     `refactor/<name>`, `docs/<name>`).

1. **Construct Atomic Commits**:

   - Break the changes into minimal, self-contained atomic commits.
   - **Branch-Based Storytelling**: Use a sequence of small, focused commits to
     tell a cohesive story for the reviewer. Individual commits on a feature
     branch do not need to be independently mergeable as long as the entire
     branch is in a mergeable, passing state.
   - **Do not bundle unrelated changes**: Keep commits focused on a single
     logical unit of work.

1. **Commit Message Format**:

   - Every commit message MUST clearly record both the **Why** and the **What**:

   ```text
   <Concise one-line summary describing the logical change>

   <Paragraph or bullets explaining WHY this change was made, the context, and motivation.>

   <Paragraph or bullets detailing WHAT was modified, added, or removed.>
   ```

   *Note on Amending:* When amending commits (e.g., `git commit --amend`,
   `hg amend`, `jj describe`), do not attempt to update the commit message
   inline in the terminal.

1. **Stage & Commit**:

   - Stage target files (`git -C <worktree-path> add path/to/files` with
     `BypassSandbox: true`).
   - Execute the commit with the structured message (`BypassSandbox: true`).
   - Repeat for each atomic commit on the branch.

______________________________________________________________________

### Step 5: Remote Push & Pull Request Creation (CLI-First)

1. **Push Branch to Remote**:

   - Push the feature branch to upstream (`BypassSandbox: true`):
     ```bash
     git -C <worktree-path> push -u origin <branch-name>
     ```

1. **Create Pull Request via CLI (`gh pr create`)**:

   - Prefer command-line tools over web UIs.
   - Ensure the repository has `delete_branch_on_merge` enabled:
     ```bash
     gh repo edit --delete-branch-on-merge
     ```
   - Use `gh pr create` with structured arguments or body template:
     - Title (`--title "..."`): Concise summary of the PR.
     - Base Branch (`--base <base>`): Specify if targeting a branch other than
       default (essential for stacked PRs).
     - Body (`--body "..."` or `--body-file ...`): Structured with context,
       motivation, changes, issue references, and testing evidence.
     - Reviewers / Labels (`--reviewer ...`, `--label ...`): Apply if
       applicable.

1. **Pull Request Body Format**:

   ```markdown
   ## Summary & Motivation
   <Why this change is needed, context, architecture or design rationale.>

   ## Changes
   <Detailed list of additions, modifications, or deletions.>

   ## Related Issues
   <e.g. Closes #123, Fixes #456, or References #789>

   ## Verification & Testing
   <Test suites executed, test commands, outputs, or manual verification steps.>
   ```

______________________________________________________________________

### Step 6: Multi-PR Iteration & Dependency Tracking

If the change set was decomposed into multiple PRs:

1. Repeat Steps 4 and 5 for each subsequent PR.
1. If PRs are stacked or dependent:
   - Base the second branch/worktree on the first feature branch.
   - Target the first feature branch as base in
     `gh pr create --base <parent-branch>`.
   - Explicitly document dependencies in the PR description (e.g.,
     `Depends on #<pr_number>`).

______________________________________________________________________

### Step 7: Final Verification & Next Steps

1. Verify the local working copy and branches (`BypassSandbox: false`):
   - `git -C <worktree-path> status` (confirm working tree is clean)
   - `git -C <worktree-path> log -n <count> --oneline`
1. Present a concise summary to the user:
   - Pull Request link(s) / URL(s)
   - Branch name(s), worktree path(s), and base target(s)
   - Linked issues / tracking IDs
   - Brief summary of atomic commits included in each PR
1. Remind the user that once reviews and CI checks pass, the **`submit`** skill
   (`../submit/SKILL.md`) can be invoked to merge the PR and clean up the
   worktree and branch.
