SHELL := /bin/sh
GO ?= go
GO_PACKAGES := ./cmd/... ./internal/...
COMPOSE ?= docker compose
GOLANGCI_VERSION := v2.14.0
GOVULNCHECK_VERSION := v1.8.0
GO_TOOLCHAIN := go1.27.1

.PHONY: dev build test test-race test-integration lint fmt fmt-check vet migrate compose-up compose-down observability ui tools security terraform-check

dev: compose-up

compose-up:
	$(COMPOSE) up --build -d --wait

compose-down:
	$(COMPOSE) --profile ui --profile observability down

observability:
	OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 $(COMPOSE) --profile observability up --build -d --wait

ui:
	$(COMPOSE) --profile ui up --build -d --wait

build:
	@mkdir -p bin
	@for service in api engine worker migrate; do $(GO) build -trimpath -o bin/$$service ./cmd/$$service || exit; done

test:
	$(GO) test -coverprofile=coverage.out $(GO_PACKAGES)

test-race:
	$(GO) test -race $(GO_PACKAGES)

# TEST_DATABASE_URL and TEST_NATS_URL must point to an isolated test environment.
test-integration:
	@if [ -n "$$TEST_DATABASE_URL" ] && [ -n "$$TEST_NATS_URL" ]; then \
	  $(GO) test -race -tags=integration $(GO_PACKAGES); \
	else \
	  ./scripts/test-integration.sh; \
	fi

lint:
	golangci-lint run $(GO_PACKAGES)

fmt:
	@$(GO) fmt $(GO_PACKAGES)

fmt-check:
	@test -z "$$(gofmt -l $$(rg --files cmd internal -g '*.go'))" || (gofmt -l $$(rg --files cmd internal -g '*.go'); exit 1)

vet:
	$(GO) vet $(GO_PACKAGES)

migrate:
	$(COMPOSE) run --rm migrate

tools:
	GOTOOLCHAIN=$(GO_TOOLCHAIN) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	GOTOOLCHAIN=$(GO_TOOLCHAIN) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

security:
	govulncheck $(GO_PACKAGES)

terraform-check:
	terraform -chdir=deployments/terraform fmt -check -recursive
	terraform -chdir=deployments/terraform/environments/dev init -backend=false -input=false
	terraform -chdir=deployments/terraform/environments/dev validate
