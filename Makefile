# Built binaries will be placed here
DIST_PATH ?= dist

# Default flags used by the test, testci, testcover targets
COVERAGE_PATH ?= coverage.txt
COVERAGE_ARGS ?= -covermode=atomic -coverprofile=$(COVERAGE_PATH)
TEST_ARGS     ?= -race

# 3rd party tools
GOFUMPT     := go run mvdan.cc/gofumpt@v0.9.2
REFLEX      := go run github.com/cespare/reflex@v0.3.2
REVIVE      := go run github.com/mgechev/revive@v1.15.0
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@2026.1

# Port to listen on when running locally
PORT ?= 8080

# Build with compile-time otel instrumentation:
# https://opentelemetry.io/docs/zero-code/go/compile-time/
GO_BUILD ?= go run go.opentelemetry.io/otelc/tool/cmd/otelc@v1.0.1 go build

# Capture version at deploy time as ${branch}@${commit}
DEPLOY_VERSION ?= $(shell git rev-parse --abbrev-ref HEAD)@$(shell git describe --always --dirty)

# =============================================================================
# build
# =============================================================================
build: $(DIST_PATH)/httpbingo

$(DIST_PATH)/httpbingo: main.go go.mod go.sum
	mkdir -p $(DIST_PATH)
	CGO_ENABLED=0 $(GO_BUILD) -buildvcs=false -ldflags="-s -w" -o $(DIST_PATH)/httpbingo

clean:
	rm -rf $(DIST_PATH)
.PHONY: clean

bump-version:
	./bump-version
.PHONY: bump-version

# =============================================================================
# run locally
# =============================================================================
run: build
	PORT=$(PORT) $(DIST_PATH)/httpbingo
.PHONY: run

watch:
	$(REFLEX) -s -r '\.(go|html|tmpl)$$' make run
.PHONY: watch

# ===========================================================================
# lint/format
# ===========================================================================
lint:
	$(GOFUMPT) -d .
	go vet ./...
	$(REVIVE) -set_exit_status ./...
	$(STATICCHECK) ./...
.PHONY: lint

fmt:
	$(GOFUMPT) -w .
.PHONY: fmt

# ===========================================================================
# deploy
# ===========================================================================
deploy:
	fly deploy --env OTEL_RESOURCE_ATTRIBUTES="service.version=$(DEPLOY_VERSION)"
.PHONY: deploy

image:
	DOCKER_BUILDKIT=1 docker build -t $(DOCKER_TAG) .
.PHONY: image
