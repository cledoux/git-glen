# Glen — Guided Git Workflows (`git-glen`)

A guided Git workflow plugin for Claude Code and Antigravity, standalone CLI (`glen` / `git glen`), and Go
library (`git-glen/worktree`) that gently guides day-to-day Git and GitHub
workflows—from **Pure Bare Repository Root** initialization and isolated branch
worktrees to atomic commits, pull request reviews, and clean post-merge
teardown.

### Repository Layout

```text
git-glen/
├── Makefile                    # Build, test, and install (delegates to scripts/install.sh)
├── scripts/install.sh          # Detects claude/agy and symlinks the CLI + plugin into each
├── plugin.json                 # Antigravity plugin manifest
├── hooks.json                  # Antigravity hooks
├── .claude-plugin/plugin.json  # Claude Code plugin manifest
├── hooks/hooks.json            # Claude Code hooks
├── assets/                     # Plugin logo assets
├── rules/
│   └── pure-bare-worktrees.md  # Pure Bare workflow guidance shared by both agents
├── skills/                     # Workflow skills (commit, just, retro, shipit, submit, worktree, worktree-init)
├── bin/                        # Compiled binaries (bin/glen, bin/git-glen)
└── src/                        # Go module (git-glen)
    ├── cmd/glen/               # Standalone CLI entrypoint
    ├── worktree/               # Exported Go package (init, manager, topology)
    └── internal/
        ├── hook/               # Agent-neutral guard logic behind `glen hook`
        └── testutil/           # Hermetic Git workspace test fixtures
```

## Bundled Plugin Components

| Component                 | Path                            | Description                                                                                                                                                                                                     |
| :------------------------ | :------------------------------ | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Guard Hooks**           | `hooks.json`, `hooks/hooks.json` | `PreToolUse` hooks (both agents) run `glen hook` to block edits inside the default-branch worktree and `gh pr merge --delete-branch`. Fail open if `glen` is missing or errors.                               |
| **Workflow Rule**         | `rules/pure-bare-worktrees.md`  | Pure Bare root awareness, feature worktree isolation (`<root>/<branch-name>`), Glen/skill delegation, sandbox notes, and safe PR merges. Loaded as an Antigravity rule; injected by a Claude `SessionStart` hook inside Pure Bare workspaces. |
| **`worktree` Skill**      | `skills/worktree/SKILL.md`      | Day-to-day runbook for inspecting, creating, working within, pushing, and tearing down isolated branch worktrees via `glen` (`list`, `create`, `remove`, `prune`). Includes `references/pure-bare-topology.md`. |
| **`worktree-init` Skill** | `skills/worktree-init/SKILL.md` | 5-scenario repository initialization, in-place standard clone conversion (with zero-tolerance dirty-tree guard), legacy `repo/` sibling migration, and worktree pointer/lock repair via `glen init`.            |
| **`commit` Skill**        | `skills/commit/SKILL.md`        | Branch-based atomic commit workflow and pull request creation/update. `draft` opens a draft PR, `local` commits without pushing; a later plain invocation promotes either to a ready PR.                       |
| **`submit` Skill**        | `skills/submit/SKILL.md`        | Single-PR submission, CI/review verification, server-side branch cleanup (`delete_branch_on_merge`), `<root>/main` synchronization, and post-merge worktree teardown.                                           |
| **`shipit` Skill**        | `skills/shipit/SKILL.md`        | Fast-path atomic commit and direct push workflow, bypassing PR overhead for single commits, documentation, ADRs, or lightweight changes.                                                                        |
| **`just` Skill**          | `skills/just/SKILL.md`          | Reference for `just`, the command runner: recipe discovery, execution, and `justfile` syntax.                                                                                                                   |
| **`retro` Skill**         | `skills/retro/SKILL.md`         | Structured end-of-session retrospective workflow to review outcomes and friction points, audit installed skills, persist durable context, and hand off a clean roadmap.                                         |

## Pure Bare Repository Topology Overview

```text
<project-root>/
├── .git                    # Root pointer file ("gitdir: ./.bare") for IDE/VCS discovery
├── .bare/                  # Bare Git object database (core.bare = true)
│                           # • remote.origin.fetch = +refs/heads/*:refs/remotes/origin/*
│                           # • extensions.worktreeConfig = true
│                           # • push.autoSetupRemote = true, branch.autoSetupMerge = simple
│                           # • fetch.prune = true, worktree.guessRemote = true
├── main/                   # Primary developer worktree (tracks origin/main)
│                           # • .git pointer -> ../.bare/worktrees/main
│                           # • config.worktree: core.bare = false
├── feat/                   # Ephemeral, isolated feature worktrees matching branch names
│   └── 101-auth/           # Branch: feat/101-auth
└── fix/
    └── sigterm/            # Branch: fix/sigterm
```

## Installation (`make install`)

Run `make install` to compile `bin/glen` and `bin/git-glen`, link them onto your
PATH, and install the plugin for every supported agent found on PATH:

```bash
make install                # CLI + plugin for each of claude / agy present
make install AGENTS=claude  # limit to one agent (AGENTS= for the CLI only)
```

It creates direct symlinks, so edits in the source worktree are live:

- `~/.local/bin/{glen,git-glen}` -> `<repo>/bin/{glen,git-glen}`
  (`git-glen` enables `git glen <subcommand>`)
- **Claude Code** (`claude` on PATH):
  `~/.claude/skills/glen/{.claude-plugin,README.md,rules,skills,hooks}` ->
  `<repo>/...`, loaded as the `glen@skills-dir` plugin. Start a new session to
  pick it up.
- **Antigravity** (`agy` on PATH):
  `~/.gemini/config/plugins/glen/{plugin.json,README.md,assets,rules,skills,hooks.json}`
  -> `<repo>/...`. Restart Antigravity (or reload the window).

The hooks call `glen` from PATH, so keep `~/.local/bin` on PATH.

`make install` links from whichever worktree you run it in. It warns when that
is not a glen-managed workspace, and warns and asks before installing from a
worktree other than the default branch (`YES=1` skips the prompt), since those
links break once the worktree is removed.

## CLI Usage (`glen` / `git glen`)

```bash
# Initialize, convert, migrate, or repair a repository into Pure Bare topology
glen init [target-dir-or-url] [-C <dir>]

# List active worktrees
glen list [-C <dir>] [--json]

# Create an isolated feature worktree at <root>/<branch-name>
glen create <branch-or-slug> [base-ref] [--issue <id>] [--name "Name" --email "user@example.com"] [-C <dir>]

# Remove a merged worktree, clean up empty parent folders, prune locks, and delete its local branch
glen remove <branch-or-slug> [-C <dir>]

# Prune stale worktree metadata and locks
glen prune [-C <dir>]

# Agent hook adapter (invoked by the plugin hooks; reads the payload on stdin)
glen hook <pre-tool-use|session-start> --agent=<claude|agy>
```
