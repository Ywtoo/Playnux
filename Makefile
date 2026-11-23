# Playnux Makefile
# Fast workflow for building, testing, and running Playnux on Linux.

APP_NAME ?= playnux
BUILD_DIR ?= ./bin
UI_DIR ?= ./ui
FRONTEND_DIR ?= ./ui/frontend
CORE_DIR ?= ./core
WAILS ?= wails3
PLAYNUX_SYSROOT ?=
RELEASE_ARCH := $(shell go env GOARCH)
RELEASE_VERSION := $(shell cat $(UI_DIR)/internal/release/version.txt)
RELEASE_NAME := playnux-$(RELEASE_VERSION)-linux-$(RELEASE_ARCH)

-include .playnux-build.mk

ifneq ($(strip $(PLAYNUX_SYSROOT)),)
export PKG_CONFIG_SYSROOT_DIR := $(abspath $(PLAYNUX_SYSROOT))
export PKG_CONFIG_PATH := $(PKG_CONFIG_SYSROOT_DIR)/usr/lib64/pkgconfig:$(PKG_CONFIG_SYSROOT_DIR)/usr/lib/pkgconfig:$(PKG_CONFIG_SYSROOT_DIR)/usr/share/pkgconfig
endif

.PHONY: all build build-bindings build-frontend build-ui package-linux test test-core test-ui check-ui-deps run clean dev help

help:
	@echo "Playnux Build System"
	@echo "===================="
	@echo "make build          - Build complete frontend and native binary"
	@echo "make run            - Build and launch Playnux"
	@echo "make test           - Run all unit tests (core + ui)"
	@echo "make build-frontend - Compile Vite/TypeScript frontend bundle"
	@echo "make build-ui       - Compile Go/WebKitGTK binary to $(BUILD_DIR)/$(APP_NAME)"
	@echo "make package-linux  - Build Linux RPM, DEB and binary archive with checksums"
	@echo "make dev            - Run frontend development server"
	@echo "make clean          - Clean build artifacts"

all: test build

# ── Frontend Build ───────────────────────────────────────────────────────────
build-bindings: check-ui-deps
	@mkdir -p $(UI_DIR)/frontend/dist
	@test -n "$$(ls -A $(UI_DIR)/frontend/dist)" || touch $(UI_DIR)/frontend/dist/.gitkeep
	@cd $(UI_DIR) && $(WAILS) generate bindings -f '-tags gtk3' -ts -i -clean=false .

build-frontend: build-bindings
	@echo "▶ Building frontend assets (Vite/TypeScript)..."
	@cd $(FRONTEND_DIR) && npm run build

check-ui-deps:
	@command -v pkg-config >/dev/null || { echo "Instale pkg-config para compilar a interface." >&2; exit 1; }
	@pkg-config --exists --print-errors x11 gtk+-3.0 webkit2gtk-4.1 gdk-3.0 || { \
		echo "No Fedora/Nobara, instale libX11-devel, gtk3-devel e webkit2gtk4.1-devel." >&2; \
		echo "Para usar dependências extraídas, execute make build PLAYNUX_SYSROOT=/caminho/do/sysroot." >&2; exit 1; }

# ── Go/UI Binary Build ───────────────────────────────────────────────────────
build-ui: check-ui-deps
	@echo "▶ Compiling Playnux native binary (GTK3)..."
	@mkdir -p $(BUILD_DIR)
	@cd $(UI_DIR) && GOFLAGS="-tags=gtk3,production" go build -o ../$(BUILD_DIR)/$(APP_NAME) .
	@echo "✓ Binary generated at $(BUILD_DIR)/$(APP_NAME)"

# ── Full Build ───────────────────────────────────────────────────────────────
build: check-ui-deps build-frontend
	@$(MAKE) build-ui

package-linux: build
	@mkdir -p $(BUILD_DIR)/release
	@cd $(UI_DIR) && GOARCH="$(RELEASE_ARCH)" PLAYNUX_PACKAGE_VERSION="$(RELEASE_VERSION)" PLAYNUX_PACKAGE_BINARY="$(abspath $(BUILD_DIR)/$(APP_NAME))" $(WAILS) tool package -name "$(RELEASE_NAME)" -format rpm -config build/linux/nfpm/nfpm.yaml -out "$(abspath $(BUILD_DIR)/release)"
	@cd $(UI_DIR) && GOARCH="$(RELEASE_ARCH)" PLAYNUX_PACKAGE_VERSION="$(RELEASE_VERSION)" PLAYNUX_PACKAGE_BINARY="$(abspath $(BUILD_DIR)/$(APP_NAME))" $(WAILS) tool package -name "$(RELEASE_NAME)" -format deb -config build/linux/nfpm/nfpm.yaml -out "$(abspath $(BUILD_DIR)/release)"
	@stage=$$(mktemp -d); trap 'rm -rf "$$stage"' EXIT; \
		mkdir "$$stage/$(RELEASE_NAME)"; \
		install -m 755 "$(BUILD_DIR)/$(APP_NAME)" "$$stage/$(RELEASE_NAME)/playnux" && \
		install -m 755 "$(UI_DIR)/build/linux/install.sh" "$$stage/$(RELEASE_NAME)/install.sh" && \
		install -m 644 "$(UI_DIR)/build/linux/desktop" "$$stage/$(RELEASE_NAME)/playnux.desktop" && \
		install -m 644 "$(UI_DIR)/build/appicon.png" "$$stage/$(RELEASE_NAME)/playnux.png" && \
		cp LICENSE.md CREDITS.md "$$stage/$(RELEASE_NAME)/" && \
		tar -czf "$(BUILD_DIR)/release/$(RELEASE_NAME).tar.gz" -C "$$stage" "$(RELEASE_NAME)"
	@cd $(BUILD_DIR)/release && sha256sum "$(RELEASE_NAME).rpm" "$(RELEASE_NAME).deb" "$(RELEASE_NAME).tar.gz" > SHA256SUMS

# ── Test Suite ───────────────────────────────────────────────────────────────
test-core:
	@echo "▶ Running Core test suite..."
	@cd $(CORE_DIR) && go test ./...

test-ui: check-ui-deps
	@echo "▶ Running UI host test suite..."
	@cd $(UI_DIR) && GOFLAGS="-tags=gtk3" go test ./...

test: test-core test-ui
	@echo "✓ All tests passed!"

# ── Run Application ─────────────────────────────────────────────────────────
run: build
	@echo "▶ Launching Playnux..."
	@$(BUILD_DIR)/$(APP_NAME)

# ── Frontend Dev Mode ───────────────────────────────────────────────────────
dev:
	@cd $(FRONTEND_DIR) && npm run dev

# ── Clean ───────────────────────────────────────────────────────────────────
clean:
	@echo "▶ Cleaning build directories..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(FRONTEND_DIR)/dist
	@echo "✓ Clean complete."
