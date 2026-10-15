GO      ?= go
COMPOSE  = docker compose -f deploy/compose.dev.yaml
TEST_DB ?= postgres://colony:colony@localhost:5433/colony_test?sslmode=disable
SQLC     = docker run --rm -u $$(id -u):$$(id -g) -v "$(CURDIR)":/src -w /src sqlc/sqlc:1.31.1

.PHONY: help dev down logs run-mem fake-agent test test-pg lint vet fmt build gen gen-check

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
	COLONY_STORE=memory COLONY_DEV_SEED=1 $(GO) run ./cmd/colonyhall

fake-agent:      ## Run the fake agent against the local server
	$(GO) run ./cmd/fake-agent -token col_dev_agent_token_do_not_use_in_production -scenario scenarios/ping.yaml

test:            ## Unit, contract (memory) and end-to-end tests
	$(GO) test ./...

test-pg:         ## Store contract tests against Postgres (needs `make dev`)
	COLONY_TEST_DATABASE_URL='$(TEST_DB)' $(GO) test ./internal/storetest/ -run TestPostgres -v

gen:             ## Regenerate code from api/openapi.yaml and SQL queries
	$(GO) tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	$(SQLC) generate

gen-check:       ## Fail if generated code is out of date
	$(MAKE) gen
	git diff --exit-code -- internal/apispec $$(find internal -type d -name sqlcgen)

lint:            ## golangci-lint with the depguard layer rules
	golangci-lint run ./...

vet:             ## go vet
	$(GO) vet ./...

fmt:             ## gofmt
	gofmt -w cmd internal migrations

build:           ## Build binaries into bin/
	$(GO) build -o bin/ ./cmd/...
