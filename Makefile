.PHONY: help db db-stop db-clean migrate migrate-down backend scrape-once backend-build \
	frontend frontend-deps frontend-build gen-api api-check install-service deploy logs \
	tools fmt fmt-check lint test check

SHELL := bash

MIGRATE_DIR := db/migrations
DB_URL      := postgres://bbb:bbb@localhost:5435/bbb?sslmode=disable
BIN         := $(CURDIR)/bin
export PATH := $(BIN):$(PATH)

# Pinned tool versions; Renovate bumps them via the comments.
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION := v2.14.0
# renovate: datasource=github-releases depName=sbdchd/squawk
SQUAWK_VERSION        := v2.69.0
# renovate: datasource=github-releases depName=koalaman/shellcheck
SHELLCHECK_VERSION    := v0.11.0
# renovate: datasource=go depName=mvdan.cc/sh/v3
SHFMT_VERSION         := v3.14.1
# renovate: datasource=go depName=github.com/zricethezav/gitleaks/v8
GITLEAKS_VERSION      := v8.30.1
# renovate: datasource=go depName=github.com/evilmartians/lefthook/v2
LEFTHOOK_VERSION      := v2.2.2
# renovate: datasource=go depName=github.com/golang-migrate/migrate/v4
MIGRATE_VERSION       := v4.19.1
# renovate: datasource=pypi depName=sqlfluff
SQLFLUFF_VERSION      := 4.4.0

SQLFLUFF  := uvx sqlfluff==$(SQLFLUFF_VERSION)
PRETTIER  := frontend/node_modules/.bin/prettier
SH_FILES  := $(shell git ls-files --cached --others --exclude-standard scripts)
SQL_FILES := $(wildcard $(MIGRATE_DIR)/*.sql)
# Down migrations drop things by design, so squawk only checks the ups.
UP_SQL    := $(wildcard $(MIGRATE_DIR)/*.up.sql)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

# ── Database ──────────────────────────────────────────────────────────────────

db: ## Start Postgres in Docker (detached)
	docker compose up -d --wait postgres

db-stop: ## Stop Postgres (data preserved)
	docker compose stop postgres

db-clean: ## Remove the Postgres container AND its data volume
	docker compose down -v

migrate: ## Run all pending up migrations
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL)" up

migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL)" down 1

# ── Backend ───────────────────────────────────────────────────────────────────

backend: ## Run the API server and poller on 127.0.0.1:8094
	cd backend && go run ./cmd/server

scrape-once: ## Poll the page once and exit
	cd backend && go run ./cmd/server -scrape-once

backend-build: ## Compile the server to backend/bin/server
	cd backend && go build -o bin/server ./cmd/server

# ── Frontend ──────────────────────────────────────────────────────────────────

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci
	@touch $@

frontend-deps: frontend/node_modules ## Install frontend packages from the lockfile

frontend: frontend/node_modules ## Run the Vite dev server on :5173 (proxies /api to `make backend`)
	cd frontend && npm run dev

frontend-build: frontend/node_modules ## Build the PWA into frontend/dist (served by Go via WEB_DIR)
	cd frontend && npm run build

gen-api: frontend/node_modules ## Regenerate frontend/src/api/schema.d.ts from backend/api/openapi.yaml
	cd frontend && npm run gen:api

api-check: gen-api ## Fail if schema.d.ts is stale
	git diff --exit-code -- frontend/src/api/schema.d.ts

# ── Deploy ────────────────────────────────────────────────────────────────────

install-service: ## Link and enable the systemd user unit (starts at boot; needs linger)
	systemctl --user link $(CURDIR)/deploy/bbb.service
	systemctl --user enable bbb.service
	@loginctl show-user $$USER -p Linger | grep -q yes || \
		echo "note: run 'loginctl enable-linger $$USER' so it starts at boot without a login"

deploy: backend-build frontend-build ## Build both and (re)start the deployed unit
	systemctl --user daemon-reload
	systemctl --user restart bbb.service

logs: ## Follow the deployed unit's logs
	journalctl --user -u bbb.service -f

# ── Checks ────────────────────────────────────────────────────────────────────

tools: ## Install pinned lint/format tools into ./bin and the git hooks
	@mkdir -p $(BIN)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(BIN) $(GOLANGCI_LINT_VERSION)
	curl -sSfL -o $(BIN)/squawk \
		https://github.com/sbdchd/squawk/releases/download/$(SQUAWK_VERSION)/squawk-linux-x64
	chmod +x $(BIN)/squawk
	curl -sSfL https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).linux.x86_64.tar.xz \
		| tar -xJ -C $(BIN) --strip-components=1 shellcheck-$(SHELLCHECK_VERSION)/shellcheck
	GOBIN=$(BIN) go install mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION)
	GOBIN=$(BIN) go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)
	GOBIN=$(BIN) go install github.com/evilmartians/lefthook/v2@$(LEFTHOOK_VERSION)
	GOBIN=$(BIN) go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
	cd frontend && npm ci
	lefthook install

fmt: frontend/node_modules ## Format everything in place
	cd backend && golangci-lint fmt
	$(PRETTIER) --write --log-level warn .
	$(SQLFLUFF) fix $(SQL_FILES)
	shfmt -w $(SH_FILES)

fmt-check: frontend/node_modules ## Fail if anything needs formatting (sqlfluff's layout rules run in lint)
	cd backend && golangci-lint fmt --diff
	$(PRETTIER) --check --log-level warn .
	shfmt -d $(SH_FILES)

lint: frontend/node_modules ## Run every linter
	cd backend && golangci-lint run
	cd backend && go tool govulncheck ./...
	cd frontend && npm run --silent lint
	squawk $(UP_SQL)
	$(SQLFLUFF) lint $(SQL_FILES)
	shellcheck $(SH_FILES)

test: ## Run tests (DB tests use the docker-compose database; `make db migrate` first)
	cd backend && BBB_TEST_DATABASE_URL=$${BBB_TEST_DATABASE_URL:-$(DB_URL)} go test -race -shuffle=on ./...
	cd frontend && npm run --silent test

check: fmt-check lint test api-check ## Everything CI runs
	gitleaks git --no-banner --redact .
