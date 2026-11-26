---
name: prime
description: >-
  Prime session context by delegating to an installed downstream *prime skill
  when available—or falling back to built-in repository and rules/specs
  discovery—while adding ongoing Git work context from active worktrees and open
  pull requests.
---

# Session Context Priming

Prime the session with repository context and ongoing Git work (active worktrees
and open pull requests). When a downstream `*prime` skill is installed, delegate
project-specific priming to it; otherwise, run the built-in priming pass
directly so `/prime` remains useful even when downstream plugins retire their
`*prime` skill.

## Workflow

1. **Resolve Optional Downstream `*prime` Skill**:
   - Inspect the skills available in the current session (`<skills>` in
     Antigravity or the `Skill` tool listing in Claude Code) for a skill whose
     name ends with `-prime` or `:prime`.
   - Ignore `prime` and `glen:prime` to avoid recursive self-invocation.
   - If a downstream `*prime` skill is found, report which skill was selected
     and execute it, forwarding any arguments or hints from the user's prompt.
   - If no downstream `*prime` skill is found, note briefly that built-in
     priming is running and continue directly to Step 2.
1. **Built-In Repository Context Discovery** *(when no downstream `*prime` skill
   ran)*:
   - Discover the repository layout (`bare` `.bare/` + `main/`, `subdir` `.git/`
     - `.worktrees/`, or uninitialized standard clone) and inspect top-level
       instructions, rules, and project manifests inside the primary checkout
       (`AGENTS.md`, `GEMINI.md`, `CLAUDE.md`, `README.md`, `.agents/rules/`,
       `Justfile`, `Makefile`, `pyproject.toml`, `go.mod`, `package.json`,
       `castor_config.txtpb`).
   - If specification or ADR indices exist (e.g.,
     `docs/openspec/specs/index.md`, `docs/adr/index.md`, `g3doc/SITEMAP.md`),
     read the index to understand governed components and architecture
     decisions.
   - If the user passed a topic, package, or hint in the prompt, read the
     directly relevant spec(s) and entry files for that area.
1. **Gather Ongoing Git Work** *(always)*:
   - If the workspace is a Git repository, inspect active worktrees (`glen list`
     or `git worktree list`), check branch ahead/behind and working-tree status
     across worktrees, sample recent commits on the default branch
     (`git -C <default-worktree> log --oneline -10`), and check open pull
     requests (`gh pr list`, if `gh` is available and authenticated).
1. **Present Concise Priming Summary**:
   - When a downstream `*prime` skill ran, append the ongoing Git work section
     using its format.
   - Otherwise, output a brief structured summary covering: (1) repository
     overview and key rules/specs loaded, (2) active worktrees and uncommitted
     or unpushed branches, (3) open pull requests, and (4) any immediate
     conflicts or suggested starting points for the user's task.
