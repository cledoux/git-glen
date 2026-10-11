.PHONY: build test install uninstall clean

BINARY_DIR ?= bin
INSTALL_BIN_DIR ?= $(HOME)/.local/bin
AGY_PLUGIN_DIR ?= $(HOME)/.gemini/config/plugins/glen
CLAUDE_PLUGIN_DIR ?= $(HOME)/.claude/skills/glen
# "auto" installs the plugin for each of claude / agy found on PATH.
AGENTS ?= auto
# Set YES=1 to skip the prompt when installing from a non-default worktree.
YES ?=

export BINARY_DIR INSTALL_BIN_DIR AGY_PLUGIN_DIR CLAUDE_PLUGIN_DIR AGENTS YES

build:
	@mkdir -p $(BINARY_DIR)
	go -C src build -o ../$(BINARY_DIR)/glen ./cmd/glen
	@cp -f $(BINARY_DIR)/glen $(BINARY_DIR)/git-glen

test:
	timeout 60s go -C src test -v ./...

install: build
	@scripts/install.sh

uninstall:
	rm -f $(INSTALL_BIN_DIR)/glen $(INSTALL_BIN_DIR)/git-glen
	rm -rf $(AGY_PLUGIN_DIR) $(CLAUDE_PLUGIN_DIR)

clean:
	rm -rf $(BINARY_DIR)
