# === CONFIGURATION VARIABLES ===
KUBECONFIG ?= $(HOME)/.kube/config
NAMESPACE ?= conforma
KO_DOCKER_REPO ?= ko.local
DEPLOY_MODE ?= auto
KNATIVE_VERSION ?= v1.18.2

# === PROJECT CONSTANTS ===
SERVICE_NAME := conforma-knative-service
IMAGE_PATH := ko://github.com/conforma/knative-service/cmd/trigger-vsa
SERVICE_SELECTOR := app=$(SERVICE_NAME)
EVENT_SOURCE_SELECTOR := eventing.knative.dev/sourceName=snapshot-events

# === DERIVED VARIABLES ===
CLUSTER_NAME = $(shell kubectl config current-context | sed 's/kind-//')
IS_KIND_CLUSTER = $(shell kubectl config current-context | grep -q "kind" && echo "true" || echo "false")

# === LICENSE CHECKING ===
.PHONY: license-check
license-check: ## Check license headers in source files
	@go run -modfile tools/go.mod github.com/google/addlicense -check -ignore '.github/ISSUE_TEMPLATE/**' -ignore '.github/PULL_REQUEST_TEMPLATE/**' -ignore '.github/dependabot.yml' -ignore 'vendor/**' -ignore 'node_modules/**' -ignore '*.md' -ignore '*.json' -ignore 'go.mod' -ignore 'go.sum' -ignore 'LICENSE' -ignore 'ko.yaml' -ignore '.ko.yaml' -ignore '.golangci.yml' -c 'The Conforma Contributors' -s -y 2025 .

.PHONY: license-add
license-add: ## Add license headers to source files that are missing them
	@go run -modfile tools/go.mod github.com/google/addlicense -ignore '.github/ISSUE_TEMPLATE/**' -ignore '.github/PULL_REQUEST_TEMPLATE/**' -ignore '.github/dependabot.yml' -ignore 'vendor/**' -ignore 'node_modules/**' -ignore '*.md' -ignore '*.json' -ignore 'go.mod' -ignore 'go.sum' -ignore 'LICENSE' -ignore 'ko.yaml' -ignore '.ko.yaml' -ignore '.golangci.yml' -c 'The Conforma Contributors' -s -y 2025 .

# === SHELL FUNCTIONS ===
SHELL_FUNCTIONS = resolve_registry_image() { if [[ "$(KO_DOCKER_REPO)" == *":"* ]]; then echo "$(KO_DOCKER_REPO)"; else echo "$(KO_DOCKER_REPO):latest"; fi; }; \
build_local_image() { echo "🔨 Building image locally with ko..." >&2; KO_DOCKER_REPO=ko.local ko build --local ./cmd/trigger-vsa 2>/dev/null | tail -1; }; \
deploy_with_image() { echo "🚀 Deploying to cluster..." >&2; kustomize build config/dev/ | sed "s|$(IMAGE_PATH)|$$1|g" | kubectl apply -f -; }; \
wait_for_deployment() { echo "⏳ Waiting for pods to be ready..." >&2; hack/wait-for-ready-pod.sh $(SERVICE_SELECTOR) $(NAMESPACE); echo "⏳ Waiting for ApiServerSource to be ready..." >&2; kubectl wait --for=condition=Ready apiserversource/snapshot-events -n $(NAMESPACE) --timeout=300s || true; }; \
show_service_url() { echo "✅ Deployment complete!"; echo "Service URL:"; kubectl get service $(SERVICE_NAME) -n $(NAMESPACE) -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}' && echo; }; \
show_logs() { local namespace=$$1; local suffix=$$2; echo "Showing logs from $(SERVICE_NAME)$$suffix..."; kubectl logs -n $$namespace -l $(SERVICE_SELECTOR) --tail=100 -f; }; \
undeploy_from_namespace() { local target_namespace=$$1; local suffix=$$2; echo "Removing $(SERVICE_NAME)$$suffix..."; kustomize build config/dev/ | ko delete --ignore-not-found -f -; echo "Undeployment complete!"; }

# === DEFAULT TARGET ===
.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# === CLUSTER SETUP TARGETS ===
.PHONY: setup-knative
setup-knative: ## Install and configure a kind cluster with knative installed
	@# Nuke the existing cluster if it exists
	kind delete cluster -n knative
	@# Create a new one
	@# We need eventing but we need don't need serving
	kn quickstart kind --install-eventing

.PHONY: check-knative
check-knative: ## Check if Knative and Tekton are properly installed and ready
	@echo "Checking Knative installation..."
	@kubectl get crd | grep "eventing" || (echo "Knative Eventing CRDs not found. Run 'make setup-knative' first." && exit 1)
	@echo "Checking Tekton installation..."
	@kubectl get crd tasks.tekton.dev > /dev/null 2>&1 || (echo "Tekton CRDs not found. Installing..." && kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml)
	@echo "Waiting for Tekton to be ready..."
	@kubectl wait --for=condition=ready pod -l app=tekton-pipelines-webhook -n tekton-pipelines --timeout=60s > /dev/null 2>&1 || echo "⚠️  Tekton webhook not ready yet, deployment may have timing issues"
	@echo "Knative and Tekton are properly installed!"

# === BUILD TARGETS ===
.PHONY: build
build: ## Build the service using ko
	@echo "Building service with ko..."
	ko build ./cmd/trigger-vsa

.PHONY: build-local
build-local: ## Build the service locally using ko (for testing)
	@echo "Building service locally with ko..."
	ko build --local ./cmd/trigger-vsa

# === DEPLOYMENT TARGETS ===

.PHONY: deploy-local
deploy-local: check-knative ## Deploy the service to local development environment
	@$(SHELL_FUNCTIONS); \
	if kubectl config current-context | grep -q "kind" && [ "$(DEPLOY_MODE)" != "registry" ]; then \
		echo "🔍 Detected kind cluster, using optimized local deployment..."; \
		echo "💡 Use 'make deploy-local DEPLOY_MODE=registry' to force registry-based deployment"; \
		image_name=$$(build_local_image); \
		echo "Built image: $$image_name"; \
		echo "📦 Loading image into kind cluster..."; \
		kind load docker-image "$$image_name" --name "$(CLUSTER_NAME)"; \
		deploy_with_image "$$image_name"; \
	else \
		if kubectl config current-context | grep -q "kind"; then \
			echo "🌐 Using registry-based deployment for kind cluster (DEPLOY_MODE=registry)..."; \
		else \
			echo "🌐 Using registry-based deployment for non-kind cluster..."; \
		fi; \
		image_name=$$(resolve_registry_image); \
		echo "Using existing image: $$image_name"; \
		deploy_with_image "$$image_name"; \
	fi; \
	wait_for_deployment; \
	show_service_url


.PHONY: undeploy-local
undeploy-local: ## Remove the local deployment
	@$(SHELL_FUNCTIONS); \
	undeploy_from_namespace "$(NAMESPACE)" ""


# === TESTING TARGETS ===
.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	cd vsajob && go test ./... -v
	cd cmd/trigger-vsa && go test ./... -v

.PHONY: quiet-test
quiet-test: ## Run tests without -v
	@cd vsajob && go test ./...
	@cd cmd/trigger-vsa && go test ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	cd cmd/trigger-vsa && go test -v -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: acceptance
acceptance: ## Run acceptance tests
	@echo "Running acceptance tests..."
	cd acceptance && go test ./... -v

.PHONY: test-local
test-local: ## Test local deployment with a sample snapshot
	@echo "Testing local deployment with sample snapshot..."
	kubectl apply -f test-snapshot.yaml -n $(NAMESPACE)
	@echo "Sample snapshot created. Check TaskRuns with:"
	@echo "kubectl get taskruns -n $(NAMESPACE)"

# === MONITORING TARGETS ===
.PHONY: logs
logs: ## Show logs from the service
	@$(SHELL_FUNCTIONS); \
	show_logs "$(NAMESPACE)" ""


.PHONY: status
status: ## Show deployment status
	@echo "Deployment status:"
	kubectl get all -l app=$(SERVICE_NAME) -n $(NAMESPACE)
	@echo ""
	@echo "Service status:"
	kubectl get service $(SERVICE_NAME) -n $(NAMESPACE) || echo "Service not found"
	@echo ""
	@echo "Event sources:"
	kubectl get apiserversource -n $(NAMESPACE)
	@echo ""
	@echo "Triggers:"
	kubectl get trigger -n $(NAMESPACE)

# === CODE QUALITY TARGETS ===
.PHONY: lint
lint: ## Run linter
	@echo "Running linter..."
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...
	cd acceptance && go fmt ./...
	@# go fmt ignores this due to the build tag, so we use gofmt
	gofmt -w tools/tools.go

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy
