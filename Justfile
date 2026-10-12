export BINARY_DIR := env('BINARY_DIR', 'bin')
export INSTALL_BIN_DIR := env('INSTALL_BIN_DIR', env('HOME') / '.local/bin')
export AGY_DIR := env('AGY_DIR', env('HOME') / '.gemini')
export AGY_PLUGIN_DIR := env('AGY_PLUGIN_DIR', AGY_DIR / 'config/plugins/glen')
export CLAUDE_DIR := env('CLAUDE_DIR', env('HOME') / '.claude')
export CLAUDE_PLUGIN_DIR := env('CLAUDE_PLUGIN_DIR', CLAUDE_DIR / 'skills/glen')
# Set YES=1 to skip the prompt when installing from a non-default worktree.
export YES := env('YES', '')

# Compile bin/glen and bin/git-glen from src/
build:
  @mkdir -p {{ BINARY_DIR }}
  go -C src build -o ../{{ BINARY_DIR }}/glen ./cmd/glen
  @cp -f {{ BINARY_DIR }}/glen {{ BINARY_DIR }}/git-glen

# Run all tests
test:
  timeout 60s go -C src test -v ./...

# Warn when installing from a non-Pure-Bare repo or a non-default worktree
_check-worktree:
  #!/usr/bin/env bash
  set -euo pipefail
  repo={{ quote(justfile_directory()) }}
  warn() { printf 'WARNING: %s\n' "$*" >&2; }
  confirm() {
    [[ -n $YES ]] && return 0
    if [[ ! -t 0 ]]; then
      warn "not running interactively; re-run with YES=1 to install anyway."
      exit 1
    fi
    read -r -p "$1 [y/N] " reply
    [[ $reply == [yY]* ]] || { echo "Aborted."; exit 1; }
  }
  common=$(git -C "$repo" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)
  if [[ $(basename "$common") != .bare ]]; then
    warn "$repo is not in a glen-managed (Pure Bare) workspace; installing from it anyway."
  else
    root=$(dirname "$common")
    default=$(git --git-dir="$common" symbolic-ref --quiet --short HEAD 2>/dev/null || echo main)
    if [[ $repo != "$root/$default" ]]; then
      warn "installing from worktree $repo, not the default-branch worktree $root/$default."
      warn "Installed links will point here and break if this worktree is removed."
      confirm "Install from this worktree?"
    fi
  fi

# Symlink the Claude Code plugin into CLAUDE_PLUGIN_DIR
install-claude: _check-worktree
  #!/usr/bin/env bash
  set -euo pipefail
  repo={{ quote(justfile_directory()) }}
  echo "Installing Claude Code plugin into $CLAUDE_PLUGIN_DIR"
  mkdir -p "$CLAUDE_PLUGIN_DIR"
  for c in .claude-plugin README.md rules skills hooks; do
    ln -sfn "$repo/$c" "$CLAUDE_PLUGIN_DIR/$c"
    echo "  $CLAUDE_PLUGIN_DIR/$c -> $repo/$c"
  done
  echo "  Start a new Claude Code session to load it as glen@skills-dir."

# Symlink the Antigravity plugin into AGY_PLUGIN_DIR
install-agy: _check-worktree
  #!/usr/bin/env bash
  set -euo pipefail
  repo={{ quote(justfile_directory()) }}
  echo "Installing Antigravity plugin into $AGY_PLUGIN_DIR"
  mkdir -p "$AGY_PLUGIN_DIR"
  for c in plugin.json README.md assets rules skills hooks.json; do
    ln -sfn "$repo/$c" "$AGY_PLUGIN_DIR/$c"
    echo "  $AGY_PLUGIN_DIR/$c -> $repo/$c"
  done
  echo "  Restart Antigravity (or reload the window) to load it."

# Build, then link the CLI and plugins for any detected agent directories
install: build _check-worktree
  #!/usr/bin/env bash
  set -euo pipefail
  repo={{ quote(justfile_directory()) }}
  echo "Installing glen CLI into $INSTALL_BIN_DIR"
  mkdir -p "$INSTALL_BIN_DIR"
  for b in glen git-glen; do
    ln -sfn "$repo/$BINARY_DIR/$b" "$INSTALL_BIN_DIR/$b"
    echo "  $INSTALL_BIN_DIR/$b -> $repo/$BINARY_DIR/$b"
  done
  installed_agent=0
  if [[ -d "$CLAUDE_DIR" ]]; then
    {{ quote(just_executable()) }} --justfile {{ quote(justfile()) }} --no-deps install-claude
    installed_agent=1
  fi
  if [[ -d "$AGY_DIR" ]]; then
    {{ quote(just_executable()) }} --justfile {{ quote(justfile()) }} --no-deps install-agy
    installed_agent=1
  fi
  if [[ $installed_agent -eq 0 ]]; then
    printf 'WARNING: %s\n' "neither $CLAUDE_DIR nor $AGY_DIR found; installed the CLI only." >&2
  fi
  case :$PATH: in
    *":$INSTALL_BIN_DIR:"*) ;;
    *) printf 'WARNING: %s\n' "$INSTALL_BIN_DIR is not on PATH; the agent hooks need \`glen\` on PATH." >&2 ;;
  esac

# Remove installed symlinks and plugin directories
uninstall:
  rm -f {{ INSTALL_BIN_DIR }}/glen {{ INSTALL_BIN_DIR }}/git-glen
  rm -rf {{ AGY_PLUGIN_DIR }} {{ CLAUDE_PLUGIN_DIR }}

# Clean build artifacts
clean:
  rm -rf {{ BINARY_DIR }}
