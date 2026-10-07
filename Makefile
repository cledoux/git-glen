.PHONY: build test install uninstall clean

BINARY_DIR ?= bin
INSTALL_BIN_DIR ?= $(HOME)/.local/bin
AGY_PLUGIN_DIR ?= $(HOME)/.gemini/config/plugins/glen

build:
	@mkdir -p $(BINARY_DIR)
	go -C src build -o ../$(BINARY_DIR)/glen ./cmd/glen
	@cp -f $(BINARY_DIR)/glen $(BINARY_DIR)/git-glen

test:
	timeout 60s go -C src test -v ./...

install: build
	@mkdir -p $(INSTALL_BIN_DIR)
	ln -sfn $(abspath $(BINARY_DIR)/glen) $(INSTALL_BIN_DIR)/glen
	ln -sfn $(abspath $(BINARY_DIR)/git-glen) $(INSTALL_BIN_DIR)/git-glen
	@mkdir -p $(AGY_PLUGIN_DIR)
	ln -sfn $(abspath plugin.json) $(AGY_PLUGIN_DIR)/plugin.json
	ln -sfn $(abspath README.md) $(AGY_PLUGIN_DIR)/README.md
	ln -sfn $(abspath assets) $(AGY_PLUGIN_DIR)/assets
	ln -sfn $(abspath rules) $(AGY_PLUGIN_DIR)/rules
	ln -sfn $(abspath skills) $(AGY_PLUGIN_DIR)/skills

uninstall:
	rm -f $(INSTALL_BIN_DIR)/glen $(INSTALL_BIN_DIR)/git-glen
	rm -rf $(AGY_PLUGIN_DIR)

clean:
	rm -rf $(BINARY_DIR)
