# Glen — Guided Git Workflows (`git-glen`)

A guided Git workflow plugin for Claude Code and Antigravity, standalone CLI
(`glen` / `git glen`), and Go library (`git-glen/worktree`) that guides
day-to-day Git and GitHub workflows across isolated worktree layouts (`bare` and
`subdir`), atomic commits, pull request reviews, and post-merge teardown.

## Repository Layout

```text
git-glen/
├── Justfile                        # Build, test, and install recipes (CLI + Claude/Antigravity plugins)
├── plugin/                         # Distributable plugin directory (symlinked into agent plugin dirs)
│   ├── plugin.json                 # Antigravity plugin manifest
│   ├── hooks.json                  # Antigravity hooks
│   ├── .claude-plugin/plugin.json  # Claude Code plugin manifest
│   ├── hooks/hooks.json            # Claude Code hooks
│   ├── assets/                     # Plugin logo assets
│   ├── rules/
│   │   ├── github-pr-stacks.md     # Native GitHub PR stack creation and async merge rules
│   │   └── worktrees.md            # Worktree isolation guidance (bare and subdir layouts)
│   └── skills/                     # Workflow skills (commit, just, prime, retro, shipit, submit, worktree, worktree-init)
├── bin/                            # Compiled binaries (bin/glen, bin/git-glen)
└── src/                            # Go module (git-glen)
    ├── cmd/glen/                   # Standalone CLI entrypoint
    ├── worktree/                   # Exported Go package (layout, init, manager, topology, vscode)
    └── internal/
        ├── gitcfg/                 # Pure-Go $GIT_COMMON_DIR and [glen] config reader
        ├── hook/                   # Agent-neutral guard logic behind `glen hook`
        └── testutil/               # Hermetic Git workspace test fixtures
```

## Bundled Plugin Components

| Component                 | Path                                           | Description                                                                                                                                                                                                                                                                                        |
| :------------------------ | :--------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Guard Hooks**           | `plugin/hooks.json`, `plugin/hooks/hooks.json` | `PreToolUse` hooks (both agents) run `glen hook` to block direct edits on the default branch in initialized workspaces (`bare` and `subdir`) and `gh pr merge --delete-branch`. Fail open if `glen` is missing or errors.                                                                          |
| **Workflow Rules**        | `plugin/rules/worktrees.md`                    | Layout-aware worktree isolation (`bare` and `subdir`), Glen/skill delegation, sandbox notes, and safe PR merges (`plugin/rules/github-pr-stacks.md` covers native GitHub PR stacks). Loaded as Antigravity rules; `worktrees.md` is injected by a Claude `SessionStart` hook in `glen` workspaces. |
| **`worktree` Skill**      | `plugin/skills/worktree/SKILL.md`              | Day-to-day runbook for inspecting, creating, working within, pushing, and tearing down isolated branch worktrees via `glen` (`list`, `create`, `remove`, `prune`). Includes `references/layouts.md`.                                                                                               |
| **`worktree-init` Skill** | `plugin/skills/worktree-init/SKILL.md`         | Repository initialization, conversion, migration, and repair across `bare` (default) and `subdir` (`--layout=subdir`) layouts, including dirty-tree safety guards, `.git/info/exclude` setup, and root agent config symlinking via `glen init`.                                                    |
| **`commit` Skill**        | `plugin/skills/commit/SKILL.md`                | Branch-based atomic commit workflow and pull request creation/update. `draft` opens a draft PR, `local` commits without pushing; a later plain invocation promotes either to a ready PR.                                                                                                           |
| **`submit` Skill**        | `plugin/skills/submit/SKILL.md`                | Single-PR submission, CI/review verification, server-side branch cleanup (`delete_branch_on_merge`), default-branch synchronization, and post-merge worktree teardown.                                                                                                                             |
| **`shipit` Skill**        | `plugin/skills/shipit/SKILL.md`                | Fast-path atomic commit and direct push workflow, bypassing PR overhead for single commits, documentation, ADRs, or lightweight changes.                                                                                                                                                           |
| **`just` Skill**          | `plugin/skills/just/SKILL.md`                  | Reference for `just`, the command runner: recipe discovery, execution, and `justfile` syntax.                                                                                                                                                                                                      |
| **`prime` Skill**         | `plugin/skills/prime/SKILL.md`                 | Session context priming that delegates to an installed downstream `*prime` skill when available—or falls back to built-in repository/rules/specs discovery—and inspects active Git worktrees and open pull requests to surface ongoing work.                                                       |
| **`retro` Skill**         | `plugin/skills/retro/SKILL.md`                 | Structured end-of-session retrospective workflow to review outcomes and friction points, audit installed skills, persist durable context, and hand off a clean roadmap.                                                                                                                            |

## Supported Worktree Layouts

Glen stores per-repository layout metadata under `[glen]` in
`$GIT_COMMON_DIR/config` (`.bare/config` or `.git/config`), distinguishing
configured repositories, legacy `.bare/` workspaces, and uninitialized standard
clones without touching tracked repository files.

### `bare` Layout (Default)

Every branch—including `main` and feature branches—is a peer worktree directory
under `<project-root>/` backed by `<project-root>/.bare/`:

```text
<project-root>/
├── .git                    # Root pointer file ("gitdir: ./.bare") for IDE/VCS discovery
├── .bare/                  # Bare Git object database (glen.layout = bare)
│                           # • config.worktree: core.bare = true
│                           # • remote.origin.fetch = +refs/heads/*:refs/remotes/origin/*
│                           # • extensions.worktreeConfig = true
├── main/                   # Primary developer worktree (tracks origin/main)
├── feat/                   # Ephemeral, isolated feature worktrees matching branch names
│   └── 101-auth/           # Branch: feat/101-auth
└── fix/
    └── sigterm/            # Branch: fix/sigterm
```

### `subdir` Layout (`glen init --layout=subdir`)

Keeps an existing standard Git clone in-place (even when the working tree has
uncommitted files) and nests linked feature worktrees inside `.worktrees/`,
excluded locally via `.git/info/exclude`:

```text
<repo>/
├── .git/                   # Standard Git metadata (glen.layout = subdir)
│   └── info/exclude        # Includes "/.worktrees/" (zero tracked .gitignore diff)
├── .worktrees/             # Hidden dot-directory ignored by go test ./..., pytest, ruff, rg
│   └── feat/
│       └── 101-auth/       # Linked worktree for branch feat/101-auth
└── ...                     # Primary checkout files in-place
```

### Git Configuration (`[glen]`)

Repo-local settings live in `$GIT_COMMON_DIR/config`; global defaults can be set
in `~/.gitconfig`:

```ini
[glen]
    layout = bare               # repo-local: bare | subdir
    defaultLayout = bare        # global default for `glen init` when --layout is omitted
    worktreesDir = .worktrees   # optional subdir override (default: .worktrees)
    defaultBranch = main        # optional default-branch override
    protectMain = true          # guard direct edits on the default branch (default: true)
```

## Installation (`just install`)

Run `just install` to compile `bin/glen` and `bin/git-glen`, link them into
`~/.local/bin`, and install the plugin for each detected agent directory
(`~/.claude`, `~/.gemini`):

```bash
just install         # CLI + plugin for each detected agent directory (~/.claude, ~/.gemini)
just install-claude  # link the Claude Code plugin only
just install-agy     # link the Antigravity plugin only
```

It creates direct symlinks, so edits in the source worktree are live:

- `~/.local/bin/{glen,git-glen}` -> `<repo>/bin/{glen,git-glen}` (`git-glen`
  enables `git glen <subcommand>`)
- **Claude Code** (`~/.claude` present, or `just install-claude`):
  `~/.claude/skills/glen` -> `<repo>/plugin`, loaded as the `glen@skills-dir`
  plugin. Start a new session to pick it up.
- **Antigravity** (`~/.gemini` present, or `just install-agy`):
  `~/.gemini/config/plugins/glen` -> `<repo>/plugin`. Restart Antigravity (or
  reload the window).

`just install` links from whichever worktree you run it in. It warns when that
is not a `glen`-initialized workspace, and warns and asks before installing from
a linked feature worktree (`YES=1` skips the prompt), since those links break
once the worktree is removed.

## CLI Usage (`glen` / `git glen`)

```bash
# Initialize, convert, migrate, or repair a repository (`bare` default, or `subdir`)
# Pass --vscode to create/update <root>/<repo>.code-workspace (kept in sync by create/remove/prune)
glen init [--layout=bare|subdir] [--vscode] [target-dir-or-url] [-C <dir>]

# List active worktrees and layout status
glen list [-C <dir>] [--json]

# Create an isolated feature worktree (<root>/<branch> in bare, <repo>/.worktrees/<branch> in subdir)
glen create <branch-or-slug> [base-ref] [--issue <id>] [--name "Name" --email "user@example.com"] [-C <dir>]

# Remove a merged worktree, clean up empty parent folders, prune locks, and delete its local branch
glen remove <branch-or-slug> [-C <dir>]

# Prune stale worktree metadata and locks
glen prune [-C <dir>]

# Agent hook adapter (invoked by the plugin hooks; reads the payload on stdin)
glen hook <pre-tool-use|session-start> --agent=<claude|agy>
```
