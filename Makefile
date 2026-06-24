.PHONY: build proto test test-e2e fmt check-fmt lint clean docker-build docker-push docker-release deploy undeploy helm-lint helm-template helm-test help

BINARY_NAME := keda-gpu-scaler
IMAGE_REPO := ghcr.io/pmady/keda-gpu-scaler
IMAGE_TAG ?= latest
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)
GOPATH := $(shell go env GOPATH)

# Inject version metadata into the binaries at link time (see pkg/version).
VERSION_PKG := github.com/pmady/keda-gpu-scaler/pkg/version
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the KEDA scaler binary (requires CGO for NVML)
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/keda-gpu-scaler/

build-metrics: ## Build the standalone GPU metrics CLI
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/gpu-metrics ./cmd/gpu-metrics/

build-all: build build-metrics ## Build all binaries

proto: ## Generate protobuf Go code
	protoc --go_out=pkg/externalscaler --go_opt=paths=source_relative \
		--go-grpc_out=pkg/externalscaler --go-grpc_opt=paths=source_relative \
		-Iproto externalscaler.proto

test: ## Run unit tests (pkg + cmd; e2e is build-tagged out, run via test-e2e)
	go test -v -race ./...

test-e2e: ## Run e2e integration tests (no GPU required — uses mock collector)
	go test -v -tags=e2e -race ./tests/e2e/...

fmt: ## Format Go source files
	go fmt ./...

check-fmt: ## Check formatting without modifying files (fails if any file needs gofmt)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files need formatting (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

lint: ## Run linter
	golangci-lint run ./...

vet: ## Run vet
	go vet ./...

clean: ## Remove build artifacts
	rm -rf bin/

docker-build: ## Build Docker image
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

docker-push: ## Push Docker image
	docker push $(IMAGE_REPO):$(IMAGE_TAG)

docker-release: ## Build, tag, and push a release image (use VERSION=v0.1.0)
	docker build -t $(IMAGE_REPO):$(VERSION) .
	docker tag $(IMAGE_REPO):$(VERSION) $(IMAGE_REPO):latest
	docker push $(IMAGE_REPO):$(VERSION)
	docker push $(IMAGE_REPO):latest

deploy: ## Deploy DaemonSet and Service to the cluster
	kubectl apply -f deploy/manifests.yaml

undeploy: ## Remove DaemonSet and Service from the cluster
	kubectl delete -f deploy/manifests.yaml --ignore-not-found

tidy: ## Tidy Go modules
	go mod tidy

helm-lint: ## Lint Helm chart
	helm lint deploy/helm/keda-gpu-scaler

helm-template: ## Render Helm templates
	helm template keda-gpu-scaler deploy/helm/keda-gpu-scaler

helm-test: ## Validate Helm chart renders correctly with default and custom values
	helm lint deploy/helm/keda-gpu-scaler
	helm template keda-gpu-scaler deploy/helm/keda-gpu-scaler > /dev/null
	helm template keda-gpu-scaler deploy/helm/keda-gpu-scaler --set grpc.port=50051 --set logLevel=debug > /dev/null
	@echo "Helm chart validation passed"
