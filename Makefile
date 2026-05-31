# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  iPShadowT Makefile
#  Build, test, and release automation
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

BINARY     := ipshadowt
MODULE     := github.com/iPmart/iPShadowT
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

GO         := go
GOFLAGS    := -trimpath
CGO        := 0

PLATFORMS  := linux/amd64 linux/arm64 linux/arm linux/mips linux/mipsle \
              windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 freebsd/amd64

OUT_DIR    := build
CMD_DIR    := ./cmd/ipshadowt

.PHONY: all build build-all test test-race test-cover bench fuzz lint vet \
        clean docker docker-push release install help

# ─── Default ──────────────────────────────────────
all: test build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Build ────────────────────────────────────────
build: ## Build for current platform
	CGO_ENABLED=$(CGO) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY) $(CMD_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY) ($(VERSION))"

build-linux: ## Build for Linux AMD64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY)-linux-amd64 $(CMD_DIR)

build-linux-arm: ## Build for Linux ARM64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY)-linux-arm64 $(CMD_DIR)

build-windows: ## Build for Windows AMD64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY)-windows-amd64.exe $(CMD_DIR)

build-darwin: ## Build for macOS (Intel + Apple Silicon)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY)-darwin-amd64 $(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY)-darwin-arm64 $(CMD_DIR)

build-all: clean ## Build for all platforms
	@mkdir -p $(OUT_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) \
			-ldflags "$(LDFLAGS)" \
			-o $(OUT_DIR)/$(BINARY)-$$os-$$arch$$ext $(CMD_DIR) || exit 1; \
	done
	@echo "All builds complete in $(OUT_DIR)/"
	@ls -lh $(OUT_DIR)/

# ─── Test ─────────────────────────────────────────
test: ## Run tests
	$(GO) test ./...

test-race: ## Run tests with race detector
	$(GO) test -race -v ./...

test-cover: ## Run tests with coverage
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem -run=^$$ ./...

fuzz: ## Run fuzz tests (60 seconds each)
	$(GO) test -fuzz=FuzzAEAD -fuzztime=60s ./internal/crypto/
	$(GO) test -fuzz=FuzzPadding -fuzztime=60s ./internal/crypto/
	$(GO) test -fuzz=FuzzConfigParse -fuzztime=60s ./internal/config/

# ─── Quality ──────────────────────────────────────
vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint (must be installed)
	golangci-lint run ./...

# ─── Docker ───────────────────────────────────────
docker: ## Build Docker image
	docker build -t $(BINARY):$(VERSION) -t $(BINARY):latest .

docker-push: docker ## Push to GHCR
	docker tag $(BINARY):$(VERSION) ghcr.io/ipmartnetwork/$(BINARY):$(VERSION)
	docker tag $(BINARY):latest ghcr.io/ipmartnetwork/$(BINARY):latest
	docker push ghcr.io/ipmartnetwork/$(BINARY):$(VERSION)
	docker push ghcr.io/ipmartnetwork/$(BINARY):latest

# ─── Release ──────────────────────────────────────
release: build-all ## Create release archives with checksums
	@cd $(OUT_DIR) && sha256sum * > SHA256SUMS.txt
	@echo "Release ready in $(OUT_DIR)/"

# ─── Install ──────────────────────────────────────
install: build ## Install to /usr/local/bin
	sudo cp $(OUT_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	sudo chmod +x /usr/local/bin/$(BINARY)
	@echo "Installed: /usr/local/bin/$(BINARY)"

# ─── Clean ────────────────────────────────────────
clean: ## Remove build artifacts
	rm -rf $(OUT_DIR) coverage.out coverage.html
	$(GO) clean -cache -testcache
