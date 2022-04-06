DIST_PATH  ?= dist
DOCKER_TAG ?= httpbingo:latest
PORT       ?= 8080

TOOL_BIN_DIR ?= $(shell go env GOPATH)/bin
TOOL_REFLEX  := $(TOOL_BIN_DIR)/reflex

build: $(DIST_PATH)/httpbingo

$(DIST_PATH)/httpbingo: main.go go.mod go.sum
	mkdir -p $(DIST_PATH)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(DIST_PATH)/httpbingo

clean:
	rm -rf $(DIST_PATH)
.PHONY: clean

run: build
	PORT=$(PORT) $(DIST_PATH)/httpbingo
.PHONY: run

watch: $(TOOL_REFLEX)
	reflex -s -r '\.(go|html)$$' make run
.PHONY: watch

deploy:
	flyctl deploy
.PHONY: deploy

image:
	DOCKER_BUILDKIT=1 docker build -t $(DOCKER_TAG) .
.PHONY: image

$(TOOL_REFLEX):
	go install github.com/cespare/reflex@0.3.1
