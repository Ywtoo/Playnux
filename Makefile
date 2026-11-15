# Playnux Makefile
# Fast workflow for building, testing, and running Playnux on Linux.

APP_NAME ?= playnux
BUILD_DIR ?= ./bin
UI_DIR ?= ./ui
FRONTEND_DIR ?= ./ui/frontend
CORE_DIR ?= ./core
PLAYNUX_SYSROOT ?=

-include .playnux-build.mk

ifneq ($(strip $(PLAYNUX_SYSROOT)),)
export PKG_CONFIG_SYSROOT_DIR := $(abspath $(PLAYNUX_SYSROOT))
export PKG_CONFIG_PATH := $(PKG_CONFIG_SYSROOT_DIR)/usr/lib64/pkgconfig:$(PKG_CONFIG_SYSROOT_DIR)/usr/lib/pkgconfig:$(PKG_CONFIG_SYSROOT_DIR)/usr/share/pkgconfig
endif

.PHONY: all build build-frontend build-ui test test-core test-ui check-ui-deps run clean dev help

help:
	@echo "Playnux Build System"
	@echo "===================="
	@echo "make build          - Build complete frontend and native binary"
	@echo "make run            - Build and launch Playnux"
	@echo "make test           - Run all unit tests (core + ui)"
	@echo "make build-frontend - Compile Vite/TypeScript frontend bundle"
	@echo "make build-ui       - Compile Go/WebKitGTK binary to $(BUILD_DIR)/$(APP_NAME)"
	@echo "make dev            - Run frontend development server"
	@echo "make clean          - Clean build artifacts"

all: test build

# ── Frontend Build ───────────────────────────────────────────────────────────
build-frontend:
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
