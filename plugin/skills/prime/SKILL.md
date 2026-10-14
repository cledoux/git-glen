---
name: prime
description: >-
  Delegate to a downstream *prime skill while adding ongoing Git work context
  from active worktrees and open pull requests.
---

# Session Context Priming

Delegate context priming to an installed downstream `*prime` skill, forwarding
any user inputs and supplementing it with ongoing Git work (active worktrees and
open pull requests).

## Workflow

1. **Resolve the Downstream `*prime` Skill**:
   - Inspect the skills available in the current session (`<skills>` in
     Antigravity or the `Skill` tool listing in Claude Code) for a skill whose
     name ends with `-prime` or `:prime`.
   - Ignore `prime` and `glen:prime` to avoid recursive self-invocation.
   - If no downstream `*prime` skill is found, **stop immediately** and tell the
     user that no downstream prime skill is installed.
1. **Report the Resolved Skill**:
   - Output the name of the downstream `*prime` skill being used so the user can
     verify which skill was selected.
1. **Run the Downstream Skill & Gather Ongoing Git Work**:
   - Execute the resolved downstream `*prime` skill (via the `Skill` tool in
     Claude Code or by reading and following its `SKILL.md` in Antigravity),
     forwarding any arguments or hints from the user's prompt.
   - If the workspace is a Git repository, also inspect active worktrees (e.g.,
     `glen list` or `git worktree list`) and open pull requests (e.g.,
     `gh pr list`) to understand what work is currently ongoing, and include
     that context alongside the downstream skill's output using its format.
