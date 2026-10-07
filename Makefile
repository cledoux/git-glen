.PHONY: build test clean

BINARY_DIR=bin

build:
	@mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/glen ./cmd/glen
	@cp -f $(BINARY_DIR)/glen $(BINARY_DIR)/git-glen

test:
	timeout 60s go test -v ./...

clean:
	rm -rf $(BINARY_DIR)
