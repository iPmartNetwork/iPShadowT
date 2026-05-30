# iPShadowT Makefile
VERSION := v1.0.0-alpha1
BUILD_TIME := $(shell date -u +%Y%m%d%H%M%S)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -s -w"

.PHONY: build build-linux build-all clean test

# Build for current platform
build:
	go build $(LDFLAGS) -o ipshadowt ./cmd/ipshadowt/

# Build for Linux AMD64
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o ipshadowt-linux-amd64 ./cmd/ipshadowt/

# Build for Linux ARM64
build-linux-arm:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o ipshadowt-linux-arm64 ./cmd/ipshadowt/

# Build all platforms
build-all: build-linux build-linux-arm
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o ipshadowt-windows-amd64.exe ./cmd/ipshadowt/
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o ipshadowt-darwin-amd64 ./cmd/ipshadowt/
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o ipshadowt-darwin-arm64 ./cmd/ipshadowt/

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f ipshadowt ipshadowt-*

# Install dependencies
deps:
	go mod tidy
	go mod download

# Run server (development)
run-server:
	go run ./cmd/ipshadowt/ -c configs/server.toml

# Run client (development)
run-client:
	go run ./cmd/ipshadowt/ -c configs/client.toml
