.PHONY: build test lint fmt install clean help

BIN_DIR := bin
BIN := $(BIN_DIR)/gc-vault
PKG := ./cmd/gc-vault
INSTALL_DIR := $(shell go env GOPATH)/bin

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.1-dev")
LDFLAGS := -s -w -X github.com/zenn-dev/gc-vault/internal/version.Version=$(VERSION)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into bin/gc-vault
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

test: ## Run all tests
	go test ./...

test-cover: ## Run tests with coverage
	go test -cover ./...

lint: ## Run go vet and gofmt check
	go vet ./...
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
	  echo "gofmt issues in:"; \
	  echo "$$diff"; \
	  exit 1; \
	fi

fmt: ## Format all Go files
	gofmt -w .

install: build ## Install the binary to $GOPATH/bin
	@mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN) $(INSTALL_DIR)/gc-vault
	@echo "Installed: $(INSTALL_DIR)/gc-vault"

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
