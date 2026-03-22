# Config Manager — Build & Package
# Usage: make build | make build-all | make deb-all | make clean

APP      := cm
VERSION  ?= $(shell (git describe --tags --always --dirty 2>/dev/null || echo dev) | sed 's/^v//')
BUILD_DIR := build

# Sanitise VERSION: single-quoted printf prevents shell expansion of VERSION
# content, tr strips everything except semver-safe characters.
CLEAN_VERSION := $(shell printf '%s' '$(VERSION)' | tr -cd 'a-zA-Z0-9.-')

# Allowed .deb architectures (validated in deb target).
VALID_ARCHS := amd64 arm64 armhf

# Go build flags — inject version into core and all plugins
LDFLAGS := -s -w \
	-X main.version=$(CLEAN_VERSION) \
	-X github.com/msutara/cm-plugin-update.version=$(CLEAN_VERSION) \
	-X github.com/msutara/cm-plugin-network.version=$(CLEAN_VERSION)

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
	$(if $(filter $(_ARCH),$(VALID_ARCHS)),,$(error ARCH=$(_ARCH) is not valid; expected one of: $(VALID_ARCHS)))
	$(eval _SUFFIX := $(if $(filter armhf,$(_ARCH)),armv7,$(if $(filter arm64,$(_ARCH)),arm64,amd64)))
	cp $(BUILD_DIR)/$(APP)-linux-$(_SUFFIX) $(BUILD_DIR)/$(APP)
	VERSION=$(CLEAN_VERSION) ARCH=$(_ARCH) nfpm package --packager deb --target $(BUILD_DIR)/

## deb-all: Build .deb packages for all architectures
deb-all: build-all
	cp $(BUILD_DIR)/$(APP)-linux-amd64 $(BUILD_DIR)/$(APP) && VERSION=$(CLEAN_VERSION) ARCH=amd64 nfpm package --packager deb --target $(BUILD_DIR)/
	cp $(BUILD_DIR)/$(APP)-linux-arm64 $(BUILD_DIR)/$(APP) && VERSION=$(CLEAN_VERSION) ARCH=arm64 nfpm package --packager deb --target $(BUILD_DIR)/
	cp $(BUILD_DIR)/$(APP)-linux-armv7 $(BUILD_DIR)/$(APP) && VERSION=$(CLEAN_VERSION) ARCH=armhf nfpm package --packager deb --target $(BUILD_DIR)/

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
