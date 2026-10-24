BINARY := adb-mcp
INSTALL_DIR ?= $(HOME)/.local/bin

# Version: prefer an exact/annotated `git describe` tag match (v-prefix
# stripped to match VERSION/main.go's bare-number convention), fall back to
# the VERSION file (git describe --always would otherwise mask a missing tag
# by returning a bare commit hash, so it's deliberately not used here).
VERSION := $(shell git describe --tags --dirty 2>/dev/null | sed 's/^v//' || cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build install uninstall test vet check run version clean fmt tidy docs docs-check plugin-test plugin-pin plugin-pin-check

build: ## Compile the server binary into ./bin
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/adb-mcp

install: build ## Build and install to $(INSTALL_DIR), re-signing so macOS doesn't SIGKILL it
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY)   # overwriting in place invalidates the code-sign cache -> "Killed: 9"
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@codesign --force --sign - $(INSTALL_DIR)/$(BINARY) 2>/dev/null || true  # ad-hoc re-sign (Apple Silicon)
	@echo "installed $(VERSION) -> $(INSTALL_DIR)/$(BINARY)"

uninstall: ## Remove the installed binary
	rm -f $(INSTALL_DIR)/$(BINARY)

version: ## Print the version that would be built
	@echo $(VERSION)

test: ## Run unit tests (no emulator required)
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	go fmt ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

docs: ## Sync the tool/guide/version counts in the docs from the code
	go run ./scripts/docsync

docs-check: ## Fail if any doc count has drifted from the code (run in CI)
	go run ./scripts/docsync -check

plugin-test: ## Test the plugin launcher and the pin check
	node --test plugin/tests/launch.test.js
	scripts/check-plugin-pin.test.sh

plugin-pin: ## Pin the plugin to a published release: make plugin-pin TAG=v0.24.3
	scripts/pin-plugin.sh $(TAG)

plugin-pin-check: ## Fail if the plugin's pin disagrees with plugin.json or the latest release's recorded checksums
	@tag=$$(jq -r '"v" + .version' plugin/adb-mcp/launcher/release.json) && \
	  curl -fsSL "https://github.com/iksnerd/adb-mcp/releases/download/$$tag/plugin-checksums.txt" -o bin/plugin-checksums.txt && \
	  scripts/check-plugin-pin.sh plugin/adb-mcp/launcher plugin/adb-mcp/.claude-plugin/plugin.json bin/plugin-checksums.txt

check: vet test docs-check plugin-test ## vet + test + doc counts + plugin launcher

run: build ## Build and run over stdio (for manual JSON-RPC poking)
	./bin/$(BINARY)

clean:
	rm -rf bin
