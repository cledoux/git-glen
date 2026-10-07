# Glen (`git-glen`)

An Antigravity plugin, CLI (`glen` / `git-glen`), and zero-dependency Go package
(`git-glen/worktree`) for Pure Bare repository topologies and guided Git/GitHub
workflows.

## Directory Structure

```text
git-glen/
├── assets/           # Plugin logo assets
├── bin/              # Compiled binaries (bin/glen, bin/git-glen)
├── cmd/glen/         # Standalone CLI entrypoint (init, list, create, remove, prune, repair)
├── internal/
│   └── testutil/     # Hermetic Git workspace test fixtures
├── rules/            # Always-on Antigravity rules (AGENTS.md)
├── skills/           # Bundled Antigravity workflow skills (worktree, worktree-init, commit, submit, shipit)
├── worktree/         # Exported Go package for Pure Bare init, worktree lifecycle, and topology discovery
├── Makefile          # Build and test orchestration
└── plugin.json       # Antigravity plugin manifest
```

## Development & Testing Commands

```bash
make build    # Compile bin/glen and bin/git-glen
make test     # Run all unit and integration tests (timeout 60s go test -v ./...)
make clean    # Clean build artifacts
```
