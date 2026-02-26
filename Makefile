# Config Manager — Build & Package
# Usage: make build | make build-all | make deb-all | make clean

APP      := cm
VERSION  ?= $(shell (git describe --tags --always --dirty 2>/dev/null || echo dev) | sed 's/^v//')
BUILD_DIR := build

# Go build flags
LDFLAGS := -s -w -X main.version=$(VERSION)

# Cross-compile targets
TARGETS := \
	linux/amd64 \
	linux/arm64 \
	linux/arm/7

.PHONY: build build-all clean lint test deb deb-all help

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

## build: Build for current platform
build: $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP) ./cmd/cm

## build-all: Cross-compile for all targets
build-all: $(BUILD_DIR) $(TARGETS)

linux/amd64: | $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-amd64 ./cmd/cm

linux/arm64: | $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-arm64 ./cmd/cm

linux/arm/7: | $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP)-linux-armv7 ./cmd/cm

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## test: Run all tests
test:
	go test -count=1 ./...

## deb: Build .deb for a single ARCH (default: amd64)
deb: build-all
	$(eval _ARCH := $(or $(ARCH),amd64))
	$(eval _SUFFIX := $(if $(filter armhf,$(_ARCH)),armv7,$(if $(filter arm64,$(_ARCH)),arm64,amd64)))
	cp $(BUILD_DIR)/$(APP)-linux-$(_SUFFIX) $(BUILD_DIR)/$(APP)
	VERSION=$(VERSION) ARCH=$(_ARCH) nfpm package --packager deb --target $(BUILD_DIR)/

## deb-all: Build .deb packages for all architectures
deb-all: build-all
	cp $(BUILD_DIR)/$(APP)-linux-amd64 $(BUILD_DIR)/$(APP) && VERSION=$(VERSION) ARCH=amd64 nfpm package --packager deb --target $(BUILD_DIR)/
	cp $(BUILD_DIR)/$(APP)-linux-arm64 $(BUILD_DIR)/$(APP) && VERSION=$(VERSION) ARCH=arm64 nfpm package --packager deb --target $(BUILD_DIR)/
	cp $(BUILD_DIR)/$(APP)-linux-armv7 $(BUILD_DIR)/$(APP) && VERSION=$(VERSION) ARCH=armhf nfpm package --packager deb --target $(BUILD_DIR)/

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
