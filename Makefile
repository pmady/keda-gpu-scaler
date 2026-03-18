.PHONY: build proto test lint clean docker-build docker-push docker-release deploy undeploy help

BINARY_NAME := keda-gpu-scaler
IMAGE_REPO := ghcr.io/pmady/keda-gpu-scaler
IMAGE_TAG ?= latest
VERSION ?= v0.1.0
GOPATH := $(shell go env GOPATH)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary (requires CGO for NVML)
	CGO_ENABLED=1 go build -o bin/$(BINARY_NAME) ./cmd/keda-gpu-scaler/

proto: ## Generate protobuf Go code
	protoc --go_out=pkg/externalscaler --go_opt=paths=source_relative \
		--go-grpc_out=pkg/externalscaler --go-grpc_opt=paths=source_relative \
		-Iproto externalscaler.proto

test: ## Run unit tests
	go test -v -race ./pkg/...

lint: ## Run linter
	golangci-lint run ./...

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
