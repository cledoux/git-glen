---
name: commit
description: >-
  Execute branch-based atomic commit workflow and open or update pull requests
  for review. Accepts an optional mode: `draft` (push and open a draft PR) or
  `local` (commit only, no push); a later plain invocation promotes earlier
  draft/local work to a ready-for-review PR.
argument-hint: "[draft|local] [scope or context]"
---

# Commit & Review Preparation Workflow

Inspect the working copy, partition changes into one or more logical review
units, construct structured atomic commits on dedicated feature branches or
isolated worktrees, push to remote, and open pull requests for code review.

______________________________________________________________________

## Modes

The first argument, if it is `draft` or `local`, selects the mode. Any remaining
arguments scope which changes to commit or add context on intent; with none,
consider the whole working tree.

| Mode        | Commits | Push | Pull request                      |
| :---------- | :------ | :--- | :-------------------------------- |
| *(default)* | yes     | yes  | open (ready for review) or update |
| `draft`     | yes     | yes  | open as draft (`--draft`)         |
| `local`     | yes     | no   | none                              |

**Promotion**: a plain (default-mode) invocation always finishes the branch's
journey to a ready-for-review PR, even with nothing new to commit:

- Branch has unpushed commits (from an earlier `local` run) -> push it.
- No PR exists for the branch -> open one.
- A draft PR exists (from an earlier `draft` run) -> push any new commits and
  mark it ready with `gh pr ready <pr>`.

Detect this state in Step 1 with `git -C <worktree-path> status -sb` (ahead
count / missing upstream) and `gh pr view --json number,isDraft,url` from the
worktree. A `draft` invocation on a branch that already has a ready PR does not
convert it back to draft.

______________________________________________________________________

## Workflow Steps

### Step 1: VCS Discovery & Working Copy Inspection

1. Discover repository topology and VCS markers (`.bare` + `.git` + `main` Pure
   Bare worktree topology, standard `.git` clone, `.jj`, or `.hg`).
1. Run detailed working tree status checks inside the target worktree:
   - Git worktree: `git -C <worktree-path> status -uall` and
     `git -C <worktree-path> diff --stat`
   - Jujutsu: `jj status` and `jj diff --stat`
   - Mercurial: `hg status`
1. Identify all untracked, modified, and deleted files.
1. Sample recent history (`git -C <worktree-path> log --oneline -10`, plus a
   full message or two) to learn the repository's commit conventions: voice,
   subject length, body usage, and issue references.
1. Determine the branch's promotion state (upstream, ahead count, existing PR
   and its draft status) as described in **Modes**. If the working tree is
   clean, skip to the step that the mode and promotion state still require.

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
   - If the split or the intent is genuinely ambiguous (two plausible groupings,
     or a *why* that the diff and conversation do not reveal), ask briefly
     rather than guessing. A wrong atomic split is harder to untangle afterwards
     than a short question now.

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
     credentials are not accidentally staged. Inspect staged content for
     anything that looks like a secret, even when the filename looks innocuous,
     and flag it to the user instead of committing it.

______________________________________________________________________

### Step 4: Worktree / Branch Creation & Atomic Commit Construction

For each planned PR:

1. **Ensure Isolated Feature Worktree or Branch**:

   - **Pure Bare Topology (`<root>/.bare` + `<root>/<branch-name>`)**: Ensure
     changes reside in an isolated feature worktree at `<root>/<branch-name>`
     (using the **`worktree`** skill or `glen create <branch-name> -C <root>`).
   - **Standard Clone**: Branch off the appropriate base
     (`git checkout -b <branch-name> [base-branch]`).
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

   - Every commit message MUST clearly record both the **Why** and the **What**.
     The subject line is the what; the body carries the reasoning the diff
     cannot show: key decisions, rejected alternatives, and non-obvious
     constraints. Summarize what changed, but do not restate the diff line by
     line.
   - Match the repository's conventions sampled in Step 1 (style, issue
     references) rather than imposing an external style.
   - Keep bodies about this change. General workflow lessons or operational
     notes belong in the project's agent instructions file (`AGENTS.md`,
     `CLAUDE.md`, or `GEMINI.md`), not in commit messages.

   ```text
   <Concise one-line summary describing the logical change>

   <Paragraph or bullets explaining WHY this change was made, the context, and motivation.>

   <Paragraph or bullets detailing WHAT was modified, added, or removed.>
   ```

   *Note on Amending:* When amending commits (e.g., `git commit --amend`,
   `hg amend`, `jj describe`), do not attempt to update the commit message
   inline in the terminal.

1. **Stage & Commit**:

   - Stage target files explicitly per commit
     (`git -C <worktree-path> add path/to/files`). Never use a blanket
     `git add -A` / `git add .`, which sweeps unrelated in-progress edits into
     the wrong commit.
   - Execute the commit with the structured message.
   - Repeat for each atomic commit on the branch.

1. **Commit Safety**:

   - Never bypass hooks or signing (`--no-verify`, `--no-gpg-sign`,
     `-c commit.gpgsign=false`). If a hook fails, fix the cause and create a new
     commit.
   - Never amend an existing commit unless the user explicitly asks.

1. **`local` mode**: skip Step 5 and the PR parts of Step 6; still build each
   planned branch's commits, then go to Step 7.

______________________________________________________________________

### Step 5: Remote Push & Pull Request Creation (CLI-First)

1. **Push Branch to Remote**:

   - Push the feature branch to upstream:
     ```bash
     git -C <worktree-path> push -u origin <branch-name>
     ```

1. **Create or Update the Pull Request via CLI**:

   - If a PR already exists for the branch, the push updates it. In default
     mode, if it is a draft, mark it ready: `gh pr ready <pr>`. Then skip to
     Step 6.
   - Otherwise create one with `gh pr create`, adding `--draft` in `draft` mode.
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
   - **Register Native GitHub PR Stack (When Explicitly Stacking PRs)**:
     - **When to Register a Stack**: Whenever the user explicitly asks to
       **stack** a PR (e.g., *"stack it on top of PR #72"*, *"stack the PR"*,
       *"create a stacked PR"*) or when decomposing work into a logical chain of
       **Stacked PRs** (Step 6), remember that
       `gh pr create --base <parent-branch>` alone only sets the Git base
       ref—you **MUST** also create or update the native GitHub Pull Request
       Stack (`PullRequestStack`) so GitHub's UI links them as a stack
       automatically.
     - **When NOT to Register a Stack**: Do **not** blindly create a native
       GitHub PR Stack every time `--base <non-default-branch>` is used if the
       branch was merely cut from in-flight work to avoid waiting and is not
       intended as a stacked PR chain.
     - **How to Create or Update the Native Stack**:
       1. Find the parent PR number for `<parent-branch>` and check if it
          already belongs to a native stack:
          ```bash
          PARENT_PR=$(gh pr view "<parent-branch>" --json number --jq .number)
          gh api graphql -F owner="<owner>" -F name="<repo>" -F pr="$PARENT_PR" -f query='
            query($owner: String!, $name: String!, $pr: Int!) {
              repository(owner: $owner, name: $name) {
                pullRequest(number: $pr) {
                  number
                  ViewerPullRequestStacks {
                    number
                    id
                    pullRequests { number }
                  }
                }
              }
            }'
          ```
       1. If `ViewerPullRequestStacks` is empty on the parent PR, create a new
          native stack via the REST API (ordered bottom/base-first to
          top/head-last):
          ```bash
          echo "{\"pull_requests\": [$PARENT_PR, $NEW_PR]}" | \
            gh api -X POST "repos/<owner>/<repo>/stacks" --input -
          ```
       1. If a stack (`<stack_number>`) already exists on the parent PR, append
          `$NEW_PR` to the existing ordered `pull_requests` array via `PATCH`:
          ```bash
          echo "{\"pull_requests\": [<existing_pr_numbers_in_order>, $NEW_PR]}" | \
            gh api -X PATCH "repos/<owner>/<repo>/stacks/<stack_number>" --input -
          ```

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
1. If PRs are stacked as a logical dependent chain:
   - Base the second branch/worktree on the first feature branch.
   - Target the first feature branch as base in
     `gh pr create --base <parent-branch>`.
   - Explicitly document dependencies in the PR description (e.g.,
     `Depends on #<pr_number>`).
   - Register or update the native GitHub PR Stack via
     `POST /repos/<owner>/<repo>/stacks` (or
     `PATCH /repos/<owner>/<repo>/stacks/<stack_number>`) with the full ordered
     list `{"pull_requests": [<pr_1>, <pr_2>, ...]}` as detailed in Step 5.

______________________________________________________________________

### Step 7: Final Verification & Next Steps

1. Verify the local working copy and branches:
   - `git -C <worktree-path> status` (confirm working tree is clean)
   - `git -C <worktree-path> log -n <count> --oneline`
1. Present a concise summary to the user:
   - Mode used, and whether the PR is draft or ready (or that commits are local
     only, with a reminder that a plain `commit` invocation will push and open
     the PR)
   - Pull Request link(s) / URL(s) (and native GitHub PR Stack number
     `Stack #<number>` whenever the PR is stacked)
   - Branch name(s), worktree path(s), and base target(s)
   - Linked issues / tracking IDs
   - Brief summary of atomic commits included in each PR
1. Remind the user that once reviews and CI checks pass, the **`submit`** skill
   (`../submit/SKILL.md`) can be invoked to merge the PR and clean up the
   worktree and branch.
