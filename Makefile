# ─────────────────────────────────────────────────────────────────────────────
#  Open Waiting Room — Makefile
#  Usage: make <target>
# ─────────────────────────────────────────────────────────────────────────────

# ── Variables ────────────────────────────────────────────────────────────────
BINARY      := owr
BUILD_DIR   := bin
CMD         := ./cmd/server
CONFIG      := config.example.yaml
IMAGE       := open-wr
GO          := go
GOFLAGS     :=

# Build metadata
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "\
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildTime=$(BUILD_TIME) \
	-s -w"

# ── Colors ───────────────────────────────────────────────────────────────────
RESET  := \033[0m
BOLD   := \033[1m
GREEN  := \033[32m
YELLOW := \033[33m
CYAN   := \033[36m
RED    := \033[31m

# ── Default target ────────────────────────────────────────────────────────────
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@printf "$(BOLD)Open Waiting Room — Available Targets$(RESET)\n\n"
	@awk 'BEGIN {FS = ":.*##"; printf "$(CYAN)%-22s$(RESET) %s\n", "Target", "Description"} \
	      /^[a-zA-Z_-]+:.*?##/ { printf "  $(GREEN)%-20s$(RESET) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf "\n$(YELLOW)Example:$(RESET) make run\n"

# ─────────────────────────────────────────────────────────────────────────────
#  Development
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: run
run: ## Run the server with example config (hot-path for development)
	$(GO) run $(CMD) --config $(CONFIG)

.PHONY: run-debug
run-debug: ## Run with debug log level
	OPENWR_LOG_LEVEL=debug $(GO) run $(CMD) --config $(CONFIG)

.PHONY: run-redis
run-redis: ## Run with Redis store (requires Redis on localhost:6379)
	OPENWR_REDIS_ENABLED=true OPENWR_REDIS_ADDR=localhost:6379 \
	$(GO) run $(CMD) --config $(CONFIG)

# ─────────────────────────────────────────────────────────────────────────────
#  Build
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build binary to ./bin/owr
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD)
	@printf "$(GREEN)✓ Built$(RESET) $(BUILD_DIR)/$(BINARY)  (version=$(VERSION) commit=$(COMMIT))\n"

.PHONY: build-linux
build-linux: ## Cross-compile for Linux/amd64 (for Docker / Fly.io / bare metal)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 $(CMD)
	@printf "$(GREEN)✓ Built$(RESET) $(BUILD_DIR)/$(BINARY)-linux-amd64\n"

.PHONY: build-arm
build-arm: ## Cross-compile for Linux/arm64 (Raspberry Pi / AWS Graviton)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 $(CMD)
	@printf "$(GREEN)✓ Built$(RESET) $(BUILD_DIR)/$(BINARY)-linux-arm64\n"

# ─────────────────────────────────────────────────────────────────────────────
#  Testing
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all unit tests
	$(GO) test ./... -count=1

.PHONY: test-v
test-v: ## Run all tests with verbose output
	$(GO) test ./... -count=1 -v

.PHONY: test-race
test-race: ## Run all tests with Go race detector
	$(GO) test ./... -count=1 -race

.PHONY: test-cover
test-cover: ## Run tests and show coverage report
	@mkdir -p $(BUILD_DIR)
	$(GO) test ./... -count=1 -race -coverprofile=$(BUILD_DIR)/coverage.out -covermode=atomic
	$(GO) tool cover -func=$(BUILD_DIR)/coverage.out | tail -1
	@printf "$(GREEN)✓ Coverage report:$(RESET) $(BUILD_DIR)/coverage.out\n"

.PHONY: test-cover-html
test-cover-html: test-cover ## Open HTML coverage report in browser
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out

.PHONY: test-pkg
test-pkg: ## Run tests for a specific package (usage: make test-pkg PKG=./internal/room)
	$(GO) test $(PKG) -count=1 -v -race

# ─────────────────────────────────────────────────────────────────────────────
#  Code Quality
# ─────────────────────────────────────────────────────────────────────────────

GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo "$(shell go env GOPATH)/bin/golangci-lint")

.PHONY: vet
vet: ## Run go vet static analysis
	$(GO) vet ./...
	@printf "$(GREEN)✓ go vet passed$(RESET)\n"

.PHONY: lint
lint: ## Run static analysis (golangci-lint + staticcheck)
	@if command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./...; then \
		true; \
	elif [ -x "$(GOLANGCI_LINT)" ] && $(GOLANGCI_LINT) run ./...; then \
		true; \
	else \
		printf "$(YELLOW)Running go vet & staticcheck lint analysis...$(RESET)\n"; \
		$(GO) vet ./... && $(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...; \
	fi
	@printf "$(GREEN)✓ Linting passed$(RESET)\n"

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix enabled
	@if [ -x "$(GOLANGCI_LINT)" ]; then \
		$(GOLANGCI_LINT) run --fix ./...; \
	else \
		golangci-lint run --fix ./...; \
	fi
	@printf "$(GREEN)✓ Auto-fix completed$(RESET)\n"

.PHONY: staticcheck
staticcheck: ## Run staticcheck static analysis tool
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...
	@printf "$(GREEN)✓ staticcheck passed$(RESET)\n"

.PHONY: vulncheck
vulncheck: ## Run official Go vulnerability checker (govulncheck)
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@printf "$(GREEN)✓ govulncheck passed$(RESET)\n"

.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...
	@printf "$(GREEN)✓ Formatted$(RESET)\n"

.PHONY: fmt-check
fmt-check: ## Check if all Go files are formatted without modifying
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then \
		printf "$(RED)Unformatted files found:$(RESET)\n$$UNFORMATTED\n"; \
		exit 1; \
	fi; \
	printf "$(GREEN)✓ Code formatting clean$(RESET)\n"

.PHONY: check
check: fmt-check vet lint test-race ## Run fmt-check + vet + lint + race tests (recommended before commit)
	@printf "\n$(GREEN)$(BOLD)✓ All checks passed!$(RESET)\n"

# ─────────────────────────────────────────────────────────────────────────────
#  Load Testing
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: load-test
load-test: ## Fire 200 concurrent requests to the waiting room (requires server running)
	@printf "$(YELLOW)Firing 200 concurrent requests to http://localhost:8080/flash/test$(RESET)\n"
	@for i in $$(seq 1 200); do \
		curl -s -c /tmp/owr_c$$i -b /tmp/owr_c$$i \
		http://localhost:8080/flash/test -o /dev/null & \
	done; wait
	@printf "$(GREEN)✓ Done$(RESET)\n"

.PHONY: load-test-api
load-test-api: ## Check admin stats endpoint
	@curl -s http://localhost:8080/api/rooms | python3 -m json.tool 2>/dev/null || \
		curl -s http://localhost:8080/api/rooms

# ─────────────────────────────────────────────────────────────────────────────
#  Docker
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .
	@printf "$(GREEN)✓ Image:$(RESET) $(IMAGE):$(VERSION)\n"

.PHONY: docker-run
docker-run: ## Run container standalone (in-memory store, no redis)
	docker run --rm -p 8080:8080 \
		-e OPENWR_COOKIE_SECRET="docker-dev-secret-change-in-prod!!" \
		-e OPENWR_LOG_LEVEL=info \
		$(IMAGE):latest

.PHONY: docker-run-redis
docker-run-redis: ## Start full stack via docker compose (open-wr + redis)
	docker compose up

.PHONY: docker-dev
docker-dev: ## Start dev stack (in-memory store, debug logs, no redis)
	docker compose --profile dev up

.PHONY: docker-redis-ui
docker-redis-ui: ## Start full stack + Redis Commander web UI (:8081)
	docker compose --profile redis-ui up

.PHONY: docker-down
docker-down: ## Stop and remove all compose containers
	docker compose down

.PHONY: docker-down-v
docker-down-v: ## Stop and remove compose containers + volumes (DELETES REDIS DATA)
	docker compose down -v
	@printf "$(YELLOW)⚠ Redis volume deleted$(RESET)\n"

.PHONY: env-init
env-init: ## Copy .env.example to .env (only if .env does not exist)
	@test -f .env && printf "$(YELLOW).env already exists — skipping$(RESET)\n" || \
		(cp .env.example .env && printf "$(GREEN)✓ .env created from .env.example$(RESET) — fill in COOKIE_SECRET!\n")

# ─────────────────────────────────────────────────────────────────────────────
#  Module Management
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: tidy
tidy: ## Tidy and verify Go modules
	$(GO) mod tidy
	$(GO) mod verify
	@printf "$(GREEN)✓ go.mod and go.sum are clean$(RESET)\n"

.PHONY: deps
deps: ## List all direct dependencies
	$(GO) list -m -f '{{if not .Indirect}}{{.}}{{end}}' all | grep -v "^github.com/semmidev"

# ─────────────────────────────────────────────────────────────────────────────
#  Utilities
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artifacts and temp files
	@rm -rf $(BUILD_DIR)
	@rm -f /tmp/owr_c*
	@printf "$(GREEN)✓ Cleaned$(RESET)\n"

.PHONY: version
version: ## Print current version info
	@printf "Version:    $(VERSION)\nCommit:     $(COMMIT)\nBuild time: $(BUILD_TIME)\n"
