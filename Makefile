# === CONFIGURATION VARIABLES ===
KUBECONFIG ?= $(HOME)/.kube/config
NAMESPACE ?= default
KO_DOCKER_REPO ?= ko.local
DEPLOY_MODE ?= auto
KNATIVE_VERSION ?= v1.18.2

# === PROJECT CONSTANTS ===
SERVICE_NAME := conforma-knative-service
IMAGE_PATH := ko://github.com/conforma/knative-service/cmd/launch-taskrun
STAGING_NAMESPACE := conforma-local
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
build_local_image() { echo "🔨 Building image locally with ko..." >&2; KO_DOCKER_REPO=ko.local ko build --local ./cmd/launch-taskrun 2>/dev/null | tail -1; }; \
deploy_with_image() { echo "🚀 Deploying to cluster..." >&2; kustomize build config/dev/ | sed "s|$(IMAGE_PATH)|$$1|g" | kubectl apply -f -; }; \
wait_for_deployment() { echo "⏳ Waiting for pods to be ready..." >&2; hack/wait-for-ready-pod.sh $(EVENT_SOURCE_SELECTOR) $(NAMESPACE); hack/wait-for-ready-pod.sh $(SERVICE_SELECTOR) $(NAMESPACE); }; \
show_service_url() { echo "✅ Deployment complete!"; echo "Service URL:"; kubectl get service $(SERVICE_NAME) -n $(NAMESPACE) -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}' && echo; }; \
show_logs() { local namespace=$$1; local suffix=$$2; echo "Showing logs from $(SERVICE_NAME)$$suffix..."; kubectl logs -n $$namespace -l $(SERVICE_SELECTOR) --tail=100 -f; }; \
undeploy_from_namespace() { local target_namespace=$$1; local suffix=$$2; echo "Removing $(SERVICE_NAME)$$suffix..."; if [ "$$target_namespace" = "$(STAGING_NAMESPACE)" ]; then kubectl delete namespace $$target_namespace --ignore-not-found; echo "Staging-local undeployment complete!"; else kustomize build config/dev/ | ko delete --ignore-not-found -f -; echo "Undeployment complete!"; fi; }

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
	kn quickstart kind

.PHONY: check-knative
check-knative: ## Check if Knative is properly installed
	@echo "Checking Knative installation..."
	@kubectl get crd | grep "eventing" || (echo "Knative Eventing CRDs not found. Run 'make setup-knative' first." && exit 1)
	@echo "Knative is properly installed!"

# === BUILD TARGETS ===
.PHONY: build
build: ## Build the service using ko
	@echo "Building service with ko..."
	ko build ./cmd/launch-taskrun

.PHONY: build-local
build-local: ## Build the service locally using ko (for testing)
	@echo "Building service locally with ko..."
	ko build --local ./cmd/launch-taskrun

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

.PHONY: deploy-staging-local
deploy-staging-local: check-knative ## Deploy locally using infra-deployments staging configuration
	@echo "Deploying $(SERVICE_NAME) using infra-deployments staging config..."
	@echo "Using KO_DOCKER_REPO: $(KO_DOCKER_REPO)"
	@echo "Fetching staging configuration from infra-deployments..."
	@trap 'rm -rf /tmp/staging-remote /tmp/staging-kustomization.yaml /tmp/fallback-staging' EXIT; \
	if curl -s https://raw.githubusercontent.com/redhat-appstudio/infra-deployments/main/components/conforma-knative-service/staging/kustomization.yaml > /tmp/staging-kustomization.yaml 2>/dev/null && [ -s /tmp/staging-kustomization.yaml ] && ! grep -q "404" /tmp/staging-kustomization.yaml; then \
		echo "✅ Found infra-deployments staging config"; \
		mkdir -p /tmp/staging-remote; \
		sed 's/namespace: .*/namespace: $(STAGING_NAMESPACE)/' /tmp/staging-kustomization.yaml > /tmp/staging-remote/kustomization.yaml; \
		kustomize build /tmp/staging-remote | KO_DOCKER_REPO=$(KO_DOCKER_REPO) ko apply --bare -f -; \
	else \
		echo "⚠️  infra-deployments staging config not yet available, using fallback..."; \
		echo "Creating namespace..."; \
		kubectl create namespace $(STAGING_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -; \
		echo "Deploying with basic configuration..."; \
		mkdir -p /tmp/fallback-staging; \
		cp -r config/base/* /tmp/fallback-staging/; \
		echo "apiVersion: kustomize.config.k8s.io/v1beta1" > /tmp/fallback-staging/kustomization.yaml; \
		echo "kind: Kustomization" >> /tmp/fallback-staging/kustomization.yaml; \
		echo "namespace: $(STAGING_NAMESPACE)" >> /tmp/fallback-staging/kustomization.yaml; \
		echo "resources:" >> /tmp/fallback-staging/kustomization.yaml; \
		for file in $$(ls /tmp/fallback-staging/*.yaml | grep -v kustomization); do echo "- $$(basename $$file)" >> /tmp/fallback-staging/kustomization.yaml; done; \
		kustomize build /tmp/fallback-staging | KO_DOCKER_REPO=$(KO_DOCKER_REPO) ko apply --bare -f -; \
	fi
	@echo "Staging-local deployment complete!"
	@echo "Service URL:"
	@kubectl get service $(SERVICE_NAME) -n $(STAGING_NAMESPACE) -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}' && echo

.PHONY: undeploy-local
undeploy-local: ## Remove the local deployment
	@$(SHELL_FUNCTIONS); \
	undeploy_from_namespace "$(NAMESPACE)" ""

.PHONY: undeploy-staging-local
undeploy-staging-local: ## Remove the staging-local deployment
	@$(SHELL_FUNCTIONS); \
	undeploy_from_namespace "$(STAGING_NAMESPACE)" " from staging-local environment"

# === TESTING TARGETS ===
.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	cd cmd/launch-taskrun && go test ./... -v

.PHONY: quiet-test
quiet-test: ## Run tests without -v
	@cd cmd/launch-taskrun && go test ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	cd cmd/launch-taskrun && go test -v -coverprofile=coverage.out
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

.PHONY: logs-staging-local
logs-staging-local: ## Show logs from the staging-local service
	@$(SHELL_FUNCTIONS); \
	show_logs "$(STAGING_NAMESPACE)" " in staging-local environment"

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

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy
