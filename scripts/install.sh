#!/usr/bin/env bash
# Symlink glen's CLI and plugin components into each agent found on PATH.
#
# Installs from wherever it is run, deliberately: installing from a feature
# worktree is how plugin changes get tested locally. Unusual sources are
# reported, never silently redirected.
#
# Environment (set by the Makefile; all overridable):
#   BINARY_DIR         built binaries (relative to the repo root)
#   INSTALL_BIN_DIR    where glen / git-glen links go
#   AGY_PLUGIN_DIR     Antigravity plugin directory
#   CLAUDE_PLUGIN_DIR  Claude Code skills-dir plugin directory
#   AGENTS             "auto" (detect claude/agy on PATH), a list such as
#                      "claude agy", or empty for the CLI only
#   YES                non-empty to skip the confirmation prompt
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
: "${BINARY_DIR:=bin}" "${INSTALL_BIN_DIR:=$HOME/.local/bin}"
: "${AGY_PLUGIN_DIR:=$HOME/.gemini/config/plugins/glen}"
: "${CLAUDE_PLUGIN_DIR:=$HOME/.claude/skills/glen}"
: "${AGENTS=auto}" "${YES=}"

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

# --- Source checks: warn about deviations, never redirect. ---
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

# --- Agent detection. ---
if [[ $AGENTS == auto ]]; then
	AGENTS=""
	command -v claude >/dev/null 2>&1 && AGENTS+=" claude"
	command -v agy >/dev/null 2>&1 && AGENTS+=" agy"
	[[ -n $AGENTS ]] || warn "neither claude nor agy found on PATH; installing the CLI only."
fi

link() { ln -sfn "$repo/$1" "$2"; echo "  $2 -> $repo/$1"; }

echo "Installing glen CLI into $INSTALL_BIN_DIR"
mkdir -p "$INSTALL_BIN_DIR"
link "$BINARY_DIR/glen" "$INSTALL_BIN_DIR/glen"
link "$BINARY_DIR/git-glen" "$INSTALL_BIN_DIR/git-glen"

for agent in $AGENTS; do
	case $agent in
	agy)
		echo "Installing Antigravity plugin into $AGY_PLUGIN_DIR"
		mkdir -p "$AGY_PLUGIN_DIR"
		for c in plugin.json README.md assets rules skills hooks.json; do
			link "$c" "$AGY_PLUGIN_DIR/$c"
		done
		echo "  Restart Antigravity (or reload the window) to load it."
		;;
	claude)
		echo "Installing Claude Code plugin into $CLAUDE_PLUGIN_DIR"
		mkdir -p "$CLAUDE_PLUGIN_DIR"
		for c in .claude-plugin README.md rules skills hooks; do
			link "$c" "$CLAUDE_PLUGIN_DIR/$c"
		done
		echo "  Start a new Claude Code session to load it as glen@skills-dir."
		;;
	*)
		warn "unknown agent '$agent' in AGENTS; skipping."
		;;
	esac
done

case :$PATH: in
*":$INSTALL_BIN_DIR:"*) ;;
*) warn "$INSTALL_BIN_DIR is not on PATH; the agent hooks need \`glen\` on PATH." ;;
esac
