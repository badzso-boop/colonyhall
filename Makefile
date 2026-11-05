GO      ?= go
COMPOSE  = docker compose -f deploy/compose.dev.yaml
TEST_DB ?= postgres://colony:colony@localhost:5433/colony_test?sslmode=disable
SQLC     = docker run --rm -u $$(id -u):$$(id -g) -v "$(CURDIR)":/src -w /src sqlc/sqlc:1.31.1

.PHONY: help dev down logs run-mem fake-agent test test-web test-pg lint vet fmt build gen gen-check web release-local shellcheck test-install

help:            ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

dev:             ## Start Postgres + server (http://localhost:8080, dev seed)
	$(COMPOSE) up --build -d
	@echo "UI: http://localhost:8080  admin token: col_dev_admin_token_do_not_use_in_production"

down:            ## Stop the dev stack (data is kept)
	$(COMPOSE) down

logs:            ## Follow server logs
	$(COMPOSE) logs -f server

run-mem:         ## Run the server in-process with in-memory stores (no Docker)
	COLONY_STORE=memory COLONY_DEV_SEED=1 COLONY_SECURE_COOKIES=0 $(GO) run ./cmd/colonyhall

fake-agent:      ## Run the fake agent against the local server
	$(GO) run ./cmd/fake-agent -token col_dev_agent_token_do_not_use_in_production -scenario scenarios/ping.yaml

test:            ## Unit, contract (memory) and end-to-end tests
	$(GO) test ./...

WEB_DEPS = web/node_modules/.package-lock.json

$(WEB_DEPS): web/package.json web/package-lock.json
	npm --prefix web ci --no-audit --no-fund

web: $(WEB_DEPS) ## Build the PWA into web/dist (embedded, served at /app/)
	npm --prefix web run build

test-web: $(WEB_DEPS) ## Frontend tests: dev page (Node) and PWA (typecheck, lint, Vitest)
	node --test internal/httpserver/web/*.test.mjs
	npm --prefix web run typecheck
	npm --prefix web run lint
	npm --prefix web test

test-pg:         ## Store contract tests against Postgres (needs `make dev`)
	COLONY_TEST_DATABASE_URL='$(TEST_DB)' $(GO) test ./internal/storetest/ -run TestPostgres -v

gen: $(WEB_DEPS) ## Regenerate code from api/openapi.yaml (Go and TypeScript) and SQL queries
	$(GO) tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	npm --prefix web run gen:api
	$(SQLC) generate

gen-check:       ## Fail if generated code is out of date
	$(MAKE) gen
	git diff --exit-code -- internal/apispec web/src/shared/api/schema.d.ts $$(find internal -type d -name sqlcgen)

lint:            ## golangci-lint with the depguard layer rules
	golangci-lint run ./...

vet:             ## go vet
	$(GO) vet ./...

fmt:             ## gofmt
	gofmt -w cmd internal migrations

build:           ## Build binaries into bin/
	$(GO) build -o bin/ ./cmd/...

VERSION    ?= v0.0.0-dev
SHELLCHECK  = $(shell command -v shellcheck 2>/dev/null || echo docker run --rm -v "$(CURDIR)":/mnt -w /mnt koalaman/shellcheck:stable)

release-local: $(WEB_DEPS) ## Build the release artifacts into dist/ (VERSION=v1.2.3)
	scripts/release.sh $(VERSION) dist

shellcheck:      ## shellcheck every shell script (local binary or the Docker image)
	$(SHELLCHECK) scripts/*.sh

test-install:    ## install.sh tests in Debian, Ubuntu and Fedora containers (needs Docker)
	scripts/test-install.sh
