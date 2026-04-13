# Dermoscope Helper - Build Makefile
#
# Cross-compilation Note:
# This project uses CGO (via gousb for USB support). Cross-compilation requires:
# - For Windows: mingw-w64 cross-compiler (brew install mingw-w64)
# - For Linux from macOS: musl-cross (brew install FiloSottile/musl-cross/musl-cross)
# Native builds on each platform work without additional setup.
#
# For CI/CD, prefer building on native runners for each target platform.

.PHONY: all build build-windows build-darwin build-darwin-amd64 build-darwin-arm64 build-linux build-all clean dev test test-coverage deps vet fmt list-profiles run run-debug

VERSION := 1.0.0
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"
BINARY_NAME := dermoscope-helper
DIST_DIR := dist

# Default target - build for all platforms
all: build-windows build-darwin build-linux

# Build for current platform
build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/dermoscope-helper

# Development build (current platform, no optimizations)
dev:
	go build -o $(BINARY_NAME) ./cmd/dermoscope-helper

# Build for Windows
build-windows:
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-windows-amd64.exe \
		./cmd/dermoscope-helper

# Build for macOS (Intel)
build-darwin-amd64:
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-amd64 \
		./cmd/dermoscope-helper

# Build for macOS (Apple Silicon)
build-darwin-arm64:
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-arm64 \
		./cmd/dermoscope-helper

# Build for macOS (both architectures)
build-darwin: build-darwin-amd64 build-darwin-arm64

# Build for Linux
build-linux:
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-linux-amd64 \
		./cmd/dermoscope-helper

# Build all platforms (alias for `all` with clean)
build-all: clean all

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run go vet
vet:
	go vet ./...

# Run gofmt
fmt:
	gofmt -s -w .

# Clean build artifacts
clean:
	rm -rf $(DIST_DIR)/
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe
	rm -f coverage.out coverage.html

# List supported device profiles
list-profiles: build
	./$(BINARY_NAME) --list-profiles

# Run the application
run: build
	./$(BINARY_NAME)

# Run with debug logging
run-debug: build
	./$(BINARY_NAME) --debug
