# VERSION defines the project version for the tool.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 4.14.0

# You can use podman or docker as a container engine. Notice that there are some options that might be only valid for one of them.
ENGINE ?= podman

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
IMAGE_TAG_BASE ?= quay.io/lochoa/ibu-imager

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)

default: help

.PHONY: fmt
fmt: ## Run go fmt against code.
	@echo "Running go fmt"
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	@echo "Running go vet"
	go vet ./...

.PHONY: deps-update
deps-update: ## Run go mod tidy and vendor against code.
	go mod tidy && go mod vendor

.PHONY: shellcheck
shellcheck: ## Run shellcheck
	@echo "Running shellcheck"
	hack/shellcheck.sh

.PHONY: bashate
bashate: ## Run bashate
	@echo "Running bashate"
	hack/bashate.sh

lint: vendor-diff
	golangci-lint run -v

vendor-diff:
	go mod vendor && git diff --exit-code vendor

generate:
	go generate $(shell go list ./...)
	$(MAKE) fmt

##@ Build

build: deps-update fmt vet ## Build manager binary.
	go build -o bin/ibu-imager main.go

run: deps-update fmt vet ## Run the tool from your host.
	go run ./main.go

docker-build: ## Build container image with the tool.
	${ENGINE} build -t ${IMG} -f Dockerfile .

docker-push: ## Push container image with the tool.
	${ENGINE} push ${IMG}

.PHONY: help
help:   ## Shows this message.
	@grep -E '^[a-zA-Z_\.\-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
