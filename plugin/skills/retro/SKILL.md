---
name: retro
description: Conduct a structured end-of-session retrospective, persist durable context, and audit skill improvements.
---

# Session Retrospective & Knowledge Capture Workflow

Use this skill when `/retro` is invoked to conduct an end-of-session
retrospective, review trajectory successes and friction points, audit installed
skills for potential improvements, persist durable context across memory and
project files, and establish a clear roadmap before clearing session context.

______________________________________________________________________

## Retrospective Phases

### Phase 1: Trajectory & Deliverables Review

1. **Review Session Outcomes**:
   - Summarize the concrete deliverables, code modifications, and architectural
     decisions completed during the session.
1. **Review Conversations & PR Review Comments**:
   - Inspect all code review conversations and discussions across GitHub PR
     comments (`gh api repos/<owner>/<repo>/pulls/<pr>/comments`), inline
     review threads, or issue discussions.
   - Capture reviewer feedback, design discussions, and debate threads that
     occurred during the review cycle.
1. **What Went Well**:
   - Identify effective problem-solving strategies, rapid pivots, successful
     debugging workflows, and high-value artifacts created.

______________________________________________________________________

### Phase 2: Friction Points & Root-Cause Analysis

1. **What Could Have Been Improved**:
   - Review miscommunications, review pushback, design misunderstandings,
     premature actions (e.g., committing incomplete code), build or test
     failures, coverage regressions, environment mismatches, or command stalls.
1. **Analyze Review Comment Discussions**:
   - For any reviewer critiques, course corrections, or misunderstandings
     surfaced in PR/CL review comments, evaluate what led to the divergence and
     how future sessions can align faster.
1. **Root-Cause & Prevention**:
   - For every major friction point, identify the root cause and formulate a
     concrete rule or workflow adjustment to prevent recurrence.

______________________________________________________________________

### Phase 3: Installed Skills Audit & Improvements

Audit all skills utilized, considered, or active during the session (including
built-in skills, plugin skills, shared/system skills, and user-defined skills):

1. **Evaluate Skill Performance**:
   - Were skill instructions clear, complete, and practical?
   - Did any skill mandate unnecessary ceremony, produce bloated output, or miss
     platform-specific edge cases (e.g., non-root userspace, sandboxed
     terminals, Pure Bare worktree topologies, or alternative VCS)?
1. **Differentiate Skill Mutability**:
   - **Local / User-Owned Skills** (`~/.gemini/config/skills/`,
     `~/.claude/skills/`, `.agents/skills/`, `.claude/skills/`, or a locally
     checked-out plugin repository): Can be edited and improved directly in
     place during the session. In a Pure Bare repository, always make skill
     edits inside an isolated feature worktree (`<root>/<branch-name>/`), never
     directly in `<root>/main/`.
   - **Third-Party / Read-Only Skills** (built-in skills, external/system
     plugins, read-only mounts): Are read-only and **NOT directly updateable**.
1. **Mandatory Action for Third-Party / Non-Updateable Skills**:
   - When friction points, subagent prompt gaps, or execution constraints are
     identified in third-party skills, **DO NOT** simply defer to hypothetical
     upstream bug reports or suggestions.
   - **MANDATORY:** Formulate concrete, actionable operational rules, prompt
     constraints, and workarounds, and **immediately persist them into durable
     user memory** (`~/memory/default/preferences.md`,
     `~/memory/default/topics/<topic>.md`, global agent instructions such as
     `~/.gemini/GEMINI.md` or `~/.claude/CLAUDE.md`, or
     `<worktree-path>/AGENTS.md` / `CLAUDE.md` / `GEMINI.md`) in Phase 4. This
     guarantees future agent sessions will automatically apply the workaround
     without requiring user intervention.

______________________________________________________________________

### Phase 4: Durable Knowledge & Context Persistence

Ensure all valuable learnings, preferences, technical facts, and third-party
skill workarounds are permanently recorded before session context is cleared:

| Knowledge Type                        | Target Destination                                                                                              | When to Update                                                           |
| :------------------------------------ | :-------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------- |
| **User Preferences & Workflow Rules** | `~/memory/default/preferences.md` or global agent instructions (`~/.gemini/GEMINI.md`, `~/.claude/CLAUDE.md`)   | New personal habits, commit styles, tooling preferences.                 |
| **Third-Party Skill Workarounds**     | `~/memory/default/preferences.md`, `~/memory/default/topics/<topic>.md`, or global agent instructions           | Friction, prompt gaps, or constraints in read-only / third-party skills. |
| **Project-Specific Knowledge**        | `~/memory/default/projects/<project>.md`                                                                        | Architecture, container build args, API URLs, credential paths.          |
| **Repository Instructions**           | `<worktree-path>/AGENTS.md` (or `CLAUDE.md` / `GEMINI.md` per repo convention)                                  | Project-level conventions, architecture overview, build/run rules.       |
| **Reusable Workflows**                | Plugin `skills/<name>/SKILL.md`, `~/.gemini/config/skills/<name>/SKILL.md`, or `~/.claude/skills/<name>/SKILL.md` | Extract multi-step runbooks into standalone slash-command skills.        |

1. **Execute Updates**:
   - Modify or create the relevant memory files, topic guides, and repository
     instructions (`AGENTS.md`, `CLAUDE.md`, or `GEMINI.md`).
   - In a Pure Bare repository (`.bare/` + `main/` + `<branch-name>/`), tracked
     files inside `<root>/main/` must **never** be edited directly; use an
     isolated feature worktree (`<root>/<branch-name>/`).
   - Format modified Markdown files cleanly with `mdformat` if available.
   - If working copy changes are made to tracked repository files, commit and
     ship them using the **`commit`** (`../commit/SKILL.md`) or **`shipit`**
     (`../shipit/SKILL.md`) skill.

______________________________________________________________________

### Phase 5: Roadmap & Next Session Handoff

1. **Define Actionable Next Steps**:
   - List concrete tasks and phases queued for the next agent session.
1. **Verify Persistence**:
   - Ensure the roadmap is recorded in `<worktree-path>/AGENTS.md` (or
     `CLAUDE.md` / `GEMINI.md` per repo convention) and project memory so the
     next agent is immediately oriented upon `/clear`.
1. **Verify Repository State**:
   - Confirm `git -C <worktree-path> status` (or `jj status`) is clean before
     concluding the retrospective.
