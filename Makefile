# Config Manager — Build & Package
# Usage: make build | make build-all | make deb-all | make clean

APP      := cm
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
BUILD_DIR := build

# Go build flags
LDFLAGS := -s -w -X main.version=$(VERSION)

# Cross-compile targets
TARGETS := \
	linux/amd64 \
	linux/arm64 \
	linux/arm/7

.PHONY: build build-all clean lint test deb deb-all help

## build: Build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP) ./cmd/cm

## build-all: Cross-compile for all targets
build-all: $(TARGETS)

linux/amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-amd64 ./cmd/cm

linux/arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-arm64 ./cmd/cm

linux/arm/7:
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-armv7 ./cmd/cm

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## test: Run all tests
test:
	go test -race -count=1 ./...

## deb: Build .deb for a single ARCH (default: amd64)
deb: build-all
	ARCH=$${ARCH:-amd64} NFPM_BINARY_SUFFIX=$$(case $${ARCH:-amd64} in armhf) echo armv7;; arm64) echo arm64;; *) echo amd64;; esac) \
		VERSION=$(VERSION) nfpm package --packager deb --target $(BUILD_DIR)/

## deb-all: Build .deb packages for all architectures
deb-all: build-all
	VERSION=$(VERSION) ARCH=amd64 NFPM_BINARY_SUFFIX=amd64 nfpm package --packager deb --target $(BUILD_DIR)/
	VERSION=$(VERSION) ARCH=arm64 NFPM_BINARY_SUFFIX=arm64 nfpm package --packager deb --target $(BUILD_DIR)/
	VERSION=$(VERSION) ARCH=armhf NFPM_BINARY_SUFFIX=armv7 nfpm package --packager deb --target $(BUILD_DIR)/

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
