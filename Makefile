SHELL := /bin/bash

ifneq ($(wildcard .env),)
  include .env
  export
endif

GOLANGCI_VERSION    := v2.12.2
GOVULNCHECK_VERSION := v1.1.4
HADOLINT_VERSION    := v2.12.0
TRIVY_VERSION       := 0.71.2
KUBECONFORM_VERSION := 0.8.0
ACTIONLINT_VERSION  := v1.7.12
GOARCHLINT_VERSION  := v1.15.0

TESTCONTAINERS_LOCALSTACK_IMAGE := localstack/localstack:3

TOOLS_DIR    := .tools
BIN_DIR      := bin
COVERAGE_DIR := .coverage
MODULE       := github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer

GOLANGCI    := $(TOOLS_DIR)/golangci-lint
GOVULNCHECK := $(TOOLS_DIR)/govulncheck
GOARCHLINT  := $(TOOLS_DIR)/go-arch-lint
HADOLINT    := $(TOOLS_DIR)/hadolint
TRIVY       := $(TOOLS_DIR)/trivy
KUBECONFORM := $(TOOLS_DIR)/kubeconform
ACTIONLINT  := $(TOOLS_DIR)/actionlint
HELM        ?= helm

BUILD_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS       := -X main.version=$(BUILD_VERSION)

COVER_PROFILE   := $(COVERAGE_DIR)/coverage.out
COVER_HTML      := $(COVERAGE_DIR)/coverage.html
COVER_THRESHOLD := 90

.PHONY: contract-check metrics-lint metrics-inventory helm-lint lint-workflows pin-base-images all tools tools-integration setup install-hooks build test test-integration test-ci \
        merge-coverage cover cover-func cover-html cover-check \
        tidy fmt-check fix check lint lint-fix arch-lint vuln \
        docker-build docker-build-mock-downstream docker-lint docker-trivy docker-check \
        clean help

all: build

## setup: Copy .env.example -> .env (if missing) and install git hooks
setup:
	@[ -f .env ] || cp .env.example .env
	@$(MAKE) install-hooks

## install-hooks: Install .githooks/pre-commit into .git/hooks
install-hooks:
	@mkdir -p .git/hooks
	@cp .githooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "✓ installed git hooks"

## tools: Install dev tooling into .tools/
tools:
	@mkdir -p $(TOOLS_DIR)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b $(PWD)/$(TOOLS_DIR) $(GOLANGCI_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) go install github.com/fe3dback/go-arch-lint@$(GOARCHLINT_VERSION)
	@OS=$$(uname -s); ARCH=$$(uname -m); \
	if [ "$$OS" = "Darwin" ]; then \
	    if command -v brew >/dev/null 2>&1; then \
	        brew install hadolint >/dev/null && cp "$$(brew --prefix)/bin/hadolint" $(TOOLS_DIR)/hadolint; \
	    else \
	        echo "hadolint: brew not found on macOS — install manually: brew install hadolint"; exit 1; \
	    fi; \
	else \
	    curl -sLo $(TOOLS_DIR)/hadolint \
	      "https://github.com/hadolint/hadolint/releases/download/$(HADOLINT_VERSION)/hadolint-$$OS-$$ARCH" && \
	    chmod +x $(TOOLS_DIR)/hadolint; \
	fi
	@OS=$$(uname -s); ARCH=$$(uname -m); \
	if [ "$$OS" = "Darwin" ] && [ "$$ARCH" = "arm64" ]; then TRIVY_OS_ARCH="macOS-ARM64"; \
	elif [ "$$OS" = "Darwin" ]; then TRIVY_OS_ARCH="macOS-64bit"; \
	elif [ "$$ARCH" = "aarch64" ] || [ "$$ARCH" = "arm64" ]; then TRIVY_OS_ARCH="Linux-ARM64"; \
	else TRIVY_OS_ARCH="Linux-64bit"; fi; \
	curl -sLo /tmp/trivy.tar.gz \
	  "https://github.com/aquasecurity/trivy/releases/download/v$(TRIVY_VERSION)/trivy_$(TRIVY_VERSION)_$$TRIVY_OS_ARCH.tar.gz" && \
	tar -xzf /tmp/trivy.tar.gz -C $(TOOLS_DIR) trivy && rm /tmp/trivy.tar.gz
	GOBIN=$(PWD)/$(TOOLS_DIR) go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); ARCH=$$(uname -m); \
	if [ "$$ARCH" = "aarch64" ]; then ARCH="arm64"; fi; \
	if [ "$$ARCH" = "x86_64" ]; then ARCH="amd64"; fi; \
	curl -sLo /tmp/kubeconform.tar.gz \
	  "https://github.com/yannh/kubeconform/releases/download/v$(KUBECONFORM_VERSION)/kubeconform-$$OS-$$ARCH.tar.gz" && \
	tar -xzf /tmp/kubeconform.tar.gz -C $(TOOLS_DIR) kubeconform && rm /tmp/kubeconform.tar.gz
	@echo "✓ tools installed to $(TOOLS_DIR)/"

## tools-integration: Pre-pull the Docker image used by the LocalStack integration test
tools-integration:
	docker pull $(TESTCONTAINERS_LOCALSTACK_IMAGE)

## build: Compile the event-consumer and mock-downstream binaries to bin/
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/event-consumer ./cmd/event-consumer
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/mock-downstream ./cmd/mock-downstream
	@echo "✓ binaries: $(BIN_DIR)/event-consumer, $(BIN_DIR)/mock-downstream"

## dev: Run the service locally (requires .env populated with queue URLs and service addrs)
dev:
	go run ./cmd/event-consumer

## test: Run unit tests with race detector and coverage
test:
	@mkdir -p $(COVERAGE_DIR)
	go test -race -count=1 \
	    -coverpkg=./internal/... \
	    -coverprofile=$(COVERAGE_DIR)/unit.out -covermode=atomic \
	    ./internal/... ./test/contract/...
	@go tool cover -func=$(COVERAGE_DIR)/unit.out | awk '/^total:/{print "total:", $$NF}'

## test-integration: Run the LocalStack integration test — requires Docker
test-integration:
	@mkdir -p $(COVERAGE_DIR)
	AWS_ACCESS_KEY_ID=test \
	AWS_SECRET_ACCESS_KEY=test \
	AWS_EC2_METADATA_DISABLED=true \
	TESTCONTAINERS_RYUK_DISABLED=true \
	go test -race -count=1 -tags integration \
	    -coverpkg=./internal/... \
	    -coverprofile=$(COVERAGE_DIR)/integration.out -covermode=atomic \
	    ./test/integration/...
	@go tool cover -func=$(COVERAGE_DIR)/integration.out | awk '/^total:/{print "total:", $$NF}'

## merge-coverage: Merge unit + integration profiles into coverage.out
merge-coverage:
	@python3 scripts/merge_coverage.py $(COVERAGE_DIR)/unit.out $(COVERAGE_DIR)/integration.out > $(COVER_PROFILE)

## test-ci: Run unit + integration suites and merge coverage
test-ci: test test-integration merge-coverage
	@go tool cover -func=$(COVER_PROFILE) | awk '/^total:/{print "total:", $$NF}'

## cover: Print total coverage from the last test-ci run
cover:
	@[ -f $(COVER_PROFILE) ] || { echo "no profile — run 'make test-ci' first"; exit 1; }
	@go tool cover -func=$(COVER_PROFILE) | awk '/^total:/{print "total:", $$NF}'

## cover-func: Print per-function coverage breakdown
cover-func:
	@[ -f $(COVER_PROFILE) ] || { echo "no profile — run 'make test-ci' first"; exit 1; }
	@go tool cover -func=$(COVER_PROFILE)

## cover-html: Open an HTML coverage report
cover-html:
	@[ -f $(COVER_PROFILE) ] || { echo "no profile — run 'make test-ci' first"; exit 1; }
	go tool cover -html=$(COVER_PROFILE) -o $(COVER_HTML)
	@open $(COVER_HTML) 2>/dev/null || xdg-open $(COVER_HTML) 2>/dev/null || true

## cover-check: Run test-ci and gate on the global coverage threshold
cover-check: test-ci
	@pct=$$(go tool cover -func=$(COVER_PROFILE) | awk '/^total:/{gsub("%","",$$NF); print $$NF}'); \
	echo "total: $$pct% (threshold $(COVER_THRESHOLD)%)"; \
	awk -v p="$$pct" -v t="$(COVER_THRESHOLD)" 'BEGIN { exit !(p+0 >= t+0) }' || \
	  { echo "✗ coverage below threshold"; exit 1; }
	@echo "✓ coverage ok"

## tidy: Run go mod tidy
tidy:
	go mod tidy

## fmt-check: Verify gofmt formatting (read-only)
fmt-check:
	@files=$$(gofmt -l cmd/ internal/ test/); if [ -n "$$files" ]; then echo "gofmt violations (run 'make fix'):"; echo "$$files"; exit 1; fi

## fix: Auto-fix formatting and lint issues
fix:
	gofmt -w cmd/ internal/ test/
	$(GOLANGCI) run --build-tags=integration --fix ./...

## check: Full local CI pass — fmt, lint, vet, coverage-gated tests
check:
	@$(MAKE) fmt-check
	$(GOLANGCI) run --build-tags=integration ./...
	go vet ./cmd/... ./internal/...
	@$(MAKE) arch-lint
	@$(MAKE) metrics-lint
	@$(MAKE) design-sync-check-test
	@$(MAKE) helm-lint
	@$(MAKE) cover-check
	@echo "✓ all checks passed"

## lint: Run golangci-lint (read-only), including integration-tagged files
lint:
	$(GOLANGCI) run --build-tags=integration ./...

## arch-lint: Enforce the import-direction rules between internal packages
arch-lint:
	$(GOARCHLINT) check --project-path .

## lint-fix: Run golangci-lint with auto-fix
lint-fix:
	$(GOLANGCI) run --build-tags=integration --fix ./...

## contract-check: Fail if a copied contract in api/contracts differs from its source (CONTRACT_SRC_DIR=<checkout> CONTRACT_SRC_REPO=<owner/repo> reads one source locally)
contract-check:
	@scripts/contract-check.sh

## metrics-lint: Observability standard: collector naming, metricslint on a real scrape, alert and doc references, inventory drift
metrics-lint:
	@bash .github/scripts/check-metric-naming.sh
	@bash .github/scripts/metricslint.sh

## metrics-inventory: Regenerate docs/observability/metric-registry.md from the registry and a real scrape
metrics-inventory:
	@METRICS_INVENTORY_WRITE=1 bash .github/scripts/metricslint.sh

## lint-workflows: Lint GitHub Actions workflow YAML
lint-workflows:
	$(ACTIONLINT)

HELM_LINT_VALUES := --set-string secretValues.EXECUTION_SERVICE_TOKEN=placeholder \
	  --set-string secretValues.DEFINITION_SERVICE_TOKEN=placeholder \
	  --set-string secretValues.WORKFLOW_STREAM_GATEWAY_TOKEN=placeholder

## helm-lint: Lint the Helm chart and validate its rendered manifests against the Kubernetes API schema
helm-lint:
	$(HELM) lint deploy/helm $(HELM_LINT_VALUES)
	@mkdir -p $(TOOLS_DIR)
	$(HELM) template deploy/helm $(HELM_LINT_VALUES) > $(TOOLS_DIR)/helm-rendered.yaml
	$(KUBECONFORM) -strict -skip ServiceMonitor $(TOOLS_DIR)/helm-rendered.yaml
	@echo "✓ helm chart valid"

## pin-base-images: Resolve current digests for the Dockerfile base images and pin them
pin-base-images:
	@for image in golang:1.26-alpine gcr.io/distroless/static-debian12:nonroot alpine:3.20; do \
	  digest=$$(docker buildx imagetools inspect $$image | awk '/^Digest:/{print $$2; exit}'); \
	  escaped=$$(printf '%s' "$$image" | sed 's/[.]/\\./g'); \
	  sed -E -i.bak "s|^(FROM $$escaped)(@sha256:[0-9a-f]+)?( AS [A-Za-z0-9_-]+)?$$|\\1@$$digest\\3|" Dockerfile; \
	done; rm -f Dockerfile.bak; echo "Pinned base images"

## vuln: Run govulncheck against dependencies
vuln:
	$(GOVULNCHECK) ./...

IMAGE_TAG ?= local

## docker-build: Build the event-consumer container image (requires GO_PRIVATE_TOKEN in env)
docker-build:
	docker buildx build \
	  --target event-consumer \
	  --secret id=go_private_token,env=GO_PRIVATE_TOKEN \
	  --build-arg BUILD_VERSION=$(IMAGE_TAG) \
	  --load \
	  -t workflow-events-consumer:$(IMAGE_TAG) .

## docker-build-mock-downstream: Build the docker-compose-only mock-downstream image
docker-build-mock-downstream:
	docker buildx build \
	  --target mock-downstream \
	  --secret id=go_private_token,env=GO_PRIVATE_TOKEN \
	  --load \
	  -t workflow-events-consumer-mock-downstream:$(IMAGE_TAG) .

## docker-lint: Lint the Dockerfile with hadolint
docker-lint:
	$(HADOLINT) --config .hadolint.yaml Dockerfile

## docker-trivy: Scan source and dependencies for HIGH/CRITICAL CVEs
docker-trivy:
	$(TRIVY) fs . --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1

## smoke-test: Build the images, start the docker-compose stack and check one event end to end (requires GO_PRIVATE_TOKEN in env)
smoke-test:
	bash scripts/smoke-test.sh

## docker-check: Dockerfile lint + dependency CVE scan (no image build required)
docker-check: docker-lint docker-trivy
	@echo "✓ all container checks passed"

## clean: Remove build and coverage artifacts
clean:
	rm -rf $(BIN_DIR) $(COVERAGE_DIR)

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'

.PHONY: design-sync-check design-sync-check-test smoke-test

## design-sync-check: Fail if a document kept in the design repository too differs (DESIGN_DIR=<checkout> reads it locally, else GH_TOKEN reads Haroon-BCBP/design)
design-sync-check:
	@scripts/design-sync-check.sh

## design-sync-check-test: Test the design-sync-check script against throwaway directories
design-sync-check-test:
	@scripts/design-sync-check_test.sh
