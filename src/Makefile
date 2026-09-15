.PHONY: all build build-embed server server-embed cli frontend frontend-deps \
        lint test fmt swagger clean dev dev-monitor tidy help \
        skills-bundle skills-submodules skills-clean

# ── Build configuration ──────────────────────────────────────────────────────
# VERSION is injected into main.Version (and main.version for the CLI).
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
MODULE      = antelope

# EMBED_TAGS are the Go build tags for the self-contained (release) build:
#   embed       → bake the compiled Vue frontend (web_src/dist) into the binary
#   skills      → bake the generated agent skill bundle into the binary
#   timetzdata  → bake the IANA timezone database into the binary
# Comma-separated (not space) so the value needs no shell/ldflag quoting.
EMBED_TAGS ?= embed,skills,timetzdata

# Where cmd/skillsbundle writes the generated skill library. Must stay in sync
# with the //go:embed directive in that package and with
# skillbundle.DefaultDiskRoot.
SKILLS_BUNDLE_DIR = internal/modules/agent/skillbundle/bundle

# Set INCLUDE_UNLICENSED=1 to also bundle sources marked redistribute:false in
# skills/sources.yaml. Those carry no redistribution grant — read the rationale
# in that file before turning this on.
SKILLS_BUNDLE_FLAGS = $(if $(INCLUDE_UNLICENSED),-include-unlicensed,)

# Separate ldflag sets — note the server uses exported main.Version/main.Tags
# while the CLI (cmd/cli) uses its own unexported main.version.
SERVER_EMBED_LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.Tags=$(EMBED_TAGS)
SERVER_LDFLAGS       = -s -w -X main.Version=$(VERSION)
CLI_LDFLAGS          = -s -w -X main.version=$(VERSION)

all: lint test build ## Run lint, test, and the plain build

# ── Backend ──────────────────────────────────────────────────────────────────
build: server cli ## Build the plain server (no embedded UI) + CLI

server: ## Build the server WITHOUT an embedded frontend (for local dev)
	go build -ldflags "$(SERVER_LDFLAGS)" -o bin/antelope .

server-embed: $(SKILLS_BUNDLE_DIR) ## Build the server WITH the embedded frontend + skills (run `make frontend` first)
	go build -tags "$(EMBED_TAGS)" -ldflags "$(SERVER_EMBED_LDFLAGS)" -o bin/antelope .

cli: ## Build the CLI client
	go build -ldflags "$(CLI_LDFLAGS)" -o bin/antelope-cli ./cmd/cli

# ── Agent skills ─────────────────────────────────────────────────────────────
# The built-in skill library is generated from the upstream libraries vendored
# as submodules under skills/upstream/. It is NOT committed: regenerate it with
# `make skills-bundle` after checking the submodules out or moving them.
#
# Builds tagged `skills` embed the result; untagged builds read it off disk, and
# tolerate its absence (the agent then has no built-in skills).
skills-submodules: ## Check out / update the upstream skill submodules (shallow)
	git submodule update --init --depth 1 --recursive skills/upstream

skills-bundle: ## Generate the built-in agent skill bundle from skills/upstream/
	go run ./cmd/skillsbundle $(SKILLS_BUNDLE_FLAGS)

skills-clean: ## Remove the generated skill bundle
	rm -rf $(SKILLS_BUNDLE_DIR)

# Order-only-style guard: an embedded build needs the bundle to exist, because
# //go:embed fails the compile without it. Generating it here rather than
# erroring keeps `make build-embed` working from a fresh checkout.
$(SKILLS_BUNDLE_DIR):
	$(MAKE) skills-bundle

# ── Frontend ─────────────────────────────────────────────────────────────────
frontend-deps: ## Install frontend dependencies (pnpm)
	cd web_src && pnpm install --frozen-lockfile

frontend: frontend-deps ## Build the Vue frontend into web_src/dist
	cd web_src && pnpm run build

# ── Combined ─────────────────────────────────────────────────────────────────
build-embed: frontend server-embed cli ## Full self-contained build (frontend + embedded server + CLI)

# ── Quality ──────────────────────────────────────────────────────────────────
lint: ## Run Go linters
	golangci-lint run ./...

test: ## Run tests (race + coverage)
	go test -race -cover ./...

fmt: ## Format Go code
	gofmt -s -w .
	goimports -w -local $(MODULE) .

swagger: ## Regenerate Swagger docs
	swag init -g main.go -o api --parseDependency --parseInternal --exclude ./reference

# ── Dev ──────────────────────────────────────────────────────────────────────
dev: ## Run the server (HTTP + Nomad monitor) in debug mode
	ANTELOPE_SYSTEM_MODE=debug go run . web

dev-monitor: ## Run the standalone Nomad monitor in debug mode
	ANTELOPE_SYSTEM_MODE=debug go run . monitor

tidy: ## Tidy and verify module dependencies
	go mod tidy
	go mod verify

clean: ## Remove build artifacts (keeps the generated skill bundle; see skills-clean)
	rm -rf bin/ dist/ web_src/dist/

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
