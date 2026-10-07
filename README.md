# Glen — Guided Git Workflows (`git-glen`)

An Antigravity plugin, standalone CLI (`glen` / `git glen`), and Go library
(`git-glen/worktree`) that gently guides day-to-day Git and GitHub
workflows—from **Pure Bare Repository Root** initialization and isolated branch
worktrees to atomic commits, pull request reviews, and clean post-merge
teardown.

## Bundled Plugin Components

| Component                 | Path                            | Description                                                                                                                                                                                                                            |
| :------------------------ | :------------------------------ | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Always-On Rule**        | `rules/AGENTS.md`               | Enforces Pure Bare root awareness, feature worktree isolation (`<root>/<branch-name>`), per-worktree `core.bare=false` & author config, Antigravity sandbox rules for `.bare` mutations, and safe PR merges without `--delete-branch`. |
| **`worktree` Skill**      | `skills/worktree/SKILL.md`      | Day-to-day runbook for inspecting, creating, working within, pushing, and tearing down isolated branch worktrees via `glen` (`list`, `create`, `remove`, `prune`). Includes `references/pure-bare-topology.md`.                        |
| **`worktree-init` Skill** | `skills/worktree-init/SKILL.md` | 5-scenario repository initialization, in-place standard clone conversion (with zero-tolerance dirty-tree guard), legacy `repo/` sibling migration, and worktree pointer/lock repair via `glen init`.                                   |
| **`commit` Skill**        | `skills/commit/SKILL.md`        | Branch-based atomic commit workflow and pull request creation/update for code review across isolated worktrees.                                                                                                                        |
| **`submit` Skill**        | `skills/submit/SKILL.md`        | Single-PR submission, CI/review verification, server-side branch cleanup (`delete_branch_on_merge`), `<root>/main` synchronization, and post-merge worktree teardown.                                                                  |
| **`shipit` Skill**        | `skills/shipit/SKILL.md`        | Fast-path atomic commit and direct push workflow, bypassing PR overhead for single commits, documentation, ADRs, or lightweight changes.                                                                                               |

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

## CLI Usage (`glen` / `git glen`)

Build `bin/glen` and `bin/git-glen` with `make build`:

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
```

Because `make build` also produces `bin/git-glen`, placing `bin/` on your
`$PATH` enables native Git subcommand invocation (`git glen init`,
`git glen create`, `git glen list`, `git glen remove`).

## Antigravity Plugin Installation

Symlink this repository into your global Antigravity plugins directory:

```bash
ln -s /path/to/git-glen/main ~/.gemini/config/plugins/glen
```

Restart Antigravity (or reload the window) for the newly symlinked plugin
directory to be discovered.
