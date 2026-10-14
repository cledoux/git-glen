# Glen (`git-glen`)

A plugin for Claude Code and Antigravity, a CLI (`glen` / `git-glen`), and a
zero-dependency Go package (`git-glen/worktree`) for Pure Bare repository
topologies and guided Git/GitHub workflows.

This file is the shared instruction source for every agent. `CLAUDE.md` imports
it; Antigravity reads `AGENTS.md` natively. Put agent-specific notes only in
that agent's own file.

## Directory Structure

```text
git-glen/
├── Justfile                        # Build, test, and install recipes (CLI + Claude/Antigravity plugins)
├── plugin/                         # Distributable plugin directory (symlinked into agent plugin dirs)
│   ├── plugin.json                 # Antigravity plugin manifest
│   ├── hooks.json                  # Antigravity hooks (PreToolUse -> glen hook)
│   ├── .claude-plugin/plugin.json  # Claude Code plugin manifest
│   ├── hooks/hooks.json            # Claude Code hooks (SessionStart, PreToolUse -> glen hook)
│   ├── assets/                     # Plugin logo assets
│   ├── rules/                      # Pure Bare guidance (Antigravity rule; Claude SessionStart context)
│   └── skills/                     # Shared skills (worktree, worktree-init, commit, submit, shipit, just, prime, retro)
├── bin/                            # Compiled binaries (bin/glen, bin/git-glen)
└── src/                            # Go module (git-glen)
    ├── cmd/glen/                   # CLI entrypoint (init, list, create, remove, prune, repair, hook)
    ├── worktree/                   # Exported package: Pure Bare init, worktree lifecycle, topology discovery
    └── internal/
        ├── hook/                   # Agent-neutral guard logic behind `glen hook`
        └── testutil/               # Hermetic Git workspace test fixtures
```

## Development & Testing Commands

```bash
just build           # Compile bin/glen and bin/git-glen from src/
just test            # Run all tests (timeout 60s go -C src test -v ./...)
just install         # Build, then link the CLI and plugins for any detected agent directories (~/.claude, ~/.gemini)
just install-claude  # Symlink the Claude Code plugin into CLAUDE_PLUGIN_DIR
just install-agy     # Symlink the Antigravity plugin into AGY_PLUGIN_DIR
just uninstall       # Remove installed symlinks and plugin directories
just clean           # Clean build artifacts
```

`just install` (and `just install-claude` / `just install-agy`) links from the
worktree it runs in. From a feature worktree it warns and asks first (`YES=1`
skips the prompt); this is the intended way to test plugin changes before
merging.

## Working in This Repository

This repository is itself a Pure Bare workspace (`.bare/` + `main/` +
`<branch>/`). Make changes in a feature worktree created with
`glen create <branch-name>`, never in `main/`. Glen's own hooks enforce this
once the plugin is installed.

## Conventions

- **Go**: format with `goimports` and `gofmt -s`. Tests use only the standard
  `testing` package (no assert libraries). Hook code must stay fast and
  dependency-free because it runs on every agent tool call.
- **Hooks fail open**: `glen hook` must never block an agent because of its own
  error. Any parse or I/O failure returns a pass-through response.
- **Agent neutrality**: Skills and rules must not reference one agent's tool
  parameters (e.g. Antigravity's `BypassSandbox`). Describe intent ("re-run
  with sandbox escalation") and let each agent map it.
- **Two hook manifests**: Guard behavior changes go in `src/internal/hook`;
  `plugin/hooks.json` (Antigravity) and `plugin/hooks/hooks.json` (Claude Code)
  only wire events to `glen hook`. Keep both manifests in sync.
- **Manifest versions**: Keep `version` identical in `plugin/plugin.json` and
  `plugin/.claude-plugin/plugin.json`.

## Debugging Hooks

Set `GLEN_HOOK_LOG=/path/to/file` in the agent's environment to append every
raw hook payload to that file.

Antigravity hook behavior (verified against agy, 2026-10):

- Any JSON reply without an explicit `"allow"` (including `{}` and
  `{"decision":""}`) **denies** the tool call. Empty stdout is the only
  neutral reply, and it leaves normal permission prompts intact.
- A hook command that exits non-zero (e.g. `glen` not on PATH) **denies every
  tool call**. Hook commands are therefore wrapped in
  `command -v glen ... && glen hook ... || true`; keep that wrapper.
- Edit tools are `write_to_file` and `replace_file_content` with the path in
  `TargetFile`; `run_command` carries `CommandLine` and `Cwd`.
- To test the plugin without installing it globally, declare it in a scratch
  workspace's `.agents/plugins.json`:
  `{"entries":[{"path":"/path/to/worktree/plugin"}]}` (or run
  `claude --plugin-dir /path/to/worktree/plugin` for Claude Code).
