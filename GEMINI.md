# Glen (`git-glen`)

An Antigravity plugin, CLI (`glen` / `git-glen`), and zero-dependency Go package
(`git-glen/worktree`) for Pure Bare repository topologies and guided Git/GitHub
workflows.

## Directory Structure

```text
git-glen/
├── Makefile                    # Root build, test, and symlink installation
├── plugin.json                 # Antigravity plugin manifest
├── assets/                     # Plugin logo assets
├── rules/                      # Modular Antigravity rules (pure-bare-worktrees.md)
├── skills/                     # Workflow skills (worktree, worktree-init, commit, submit, shipit)
├── bin/                        # Compiled binaries (bin/glen, bin/git-glen)
└── src/                        # Go module (git-glen)
    ├── cmd/glen/               # Standalone CLI entrypoint (init, list, create, remove, prune, repair)
    ├── worktree/               # Exported Go package for Pure Bare init, worktree lifecycle, and topology discovery
    └── internal/testutil/      # Hermetic Git workspace test fixtures
```

## Development & Testing Commands

```bash
make build      # Compile bin/glen and bin/git-glen from src/
make test       # Run all unit and integration tests (timeout 60s go -C src test -v ./...)
make install    # Build and symlink binaries to ~/.local/bin and plugin files to ~/.gemini/config/plugins/glen/
make uninstall  # Remove installed symlinks
make clean      # Clean build artifacts
```

<!-- glen:pure-bare-guard -->

## Pure Bare Worktree Discipline

This repository uses the Pure Bare topology (`.bare/` + `main/` + `<branch>/`).

- NEVER edit feature or bugfix code directly in `main/`.
- NEVER run working-tree Git commands at the workspace root.
- ALWAYS use `glen create <branch-name>` or the `worktree` skill before
  modifying code.
