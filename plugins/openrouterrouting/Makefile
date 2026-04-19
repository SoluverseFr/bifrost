.PHONY: all build build-test-plugin dev clean install help

PLUGIN_NAME = openrouter_provider_routing
OUTPUT_DIR = build
OUTPUT_ABS = $(abspath $(OUTPUT))
GO ?= go

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	PLUGIN_EXT = .so
	PLATFORM = linux
endif
ifeq ($(UNAME_S),Darwin)
	PLUGIN_EXT = .so
	PLATFORM = darwin
endif

UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
	ARCH = amd64
endif
ifeq ($(UNAME_M),arm64)
	ARCH = arm64
endif

OUTPUT = $(OUTPUT_DIR)/$(PLUGIN_NAME)$(PLUGIN_EXT)

build: ## Build the plugin for current platform
	@echo "Building plugin for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=1 GOOS=$(PLATFORM) GOARCH=$(ARCH) $(GO) build -buildmode=plugin -a -trimpath -o "$(OUTPUT_ABS)" main.go
	@echo "Plugin built successfully: $(OUTPUT)"

build-test-plugin: build

dev: build

clean: ## Remove build artifacts
	@rm -rf $(OUTPUT_DIR)

install: build ## Build and install to Bifrost plugins directory
	@mkdir -p ~/.bifrost/plugins
	@cp $(OUTPUT) ~/.bifrost/plugins/
	@echo "Plugin installed to ~/.bifrost/plugins/"
