# Variables
KUBECONFIG ?= $(HOME)/.kube/config
NAMESPACE ?= default
KO_DOCKER_REPO ?= ko.local

# Knative versions
KNATIVE_VERSION ?= v1.18.0

# Knative urls
KNATIVE_OPERATOR = https://github.com/knative/operator/releases/download/knative-$(KNATIVE_VERSION)
KNATIVE_SERVING = https://github.com/knative/serving/releases/download/knative-$(KNATIVE_VERSION)
KNATIVE_EVENTING = https://github.com/knative/eventing/releases/download/knative-$(KNATIVE_VERSION)

# Default target
.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Note: `kn quickstart kind` is an alternative way to do this (and that's what I've been using actually)
.PHONY: setup-knative
setup-knative: ## Install Knative Serving and Eventing components using the operator
	@echo "Installing Knative Operator..."
	kubectl apply -f $(KNATIVE_OPERATOR)/operator.yaml
	@echo "Waiting for Knative Operator to be ready..."
	kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=knative-operator -n knative-operator --timeout=300s
	@echo "Installing Knative Serving..."
	kubectl apply -f $(KNATIVE_SERVING)/serving-crds.yaml
	kubectl apply -f $(KNATIVE_SERVING)/serving-core.yaml
	@echo "Installing Knative Eventing..."
	kubectl apply -f $(KNATIVE_EVENTING)/eventing-crds.yaml
	kubectl apply -f $(KNATIVE_EVENTING)/eventing-core.yaml
	@echo "Waiting for Knative components to be ready..."
	kubectl wait --for=condition=ready pod -l app=controller -n knative-serving --timeout=600s
	kubectl wait --for=condition=ready pod -l app=eventing-controller -n knative-eventing --timeout=600s
	@echo "Knative setup complete!"

.PHONY: check-knative
check-knative: ## Check if Knative is properly installed
	@echo "Checking Knative installation..."
	@kubectl get crd | grep -E "(serving|eventing)" || (echo "Knative CRDs not found. Run 'make setup-knative' first." && exit 1)
	@echo "Knative is properly installed!"

.PHONY: build
build: ## Build the service using ko
	@echo "Building service with ko..."
	ko build ./cmd/launch-taskrun

.PHONY: deploy
deploy: check-knative ## Deploy the service using kustomize and ko
	@echo "Deploying conforma-verifier-listener..."
	@echo "Using KO_DOCKER_REPO: $(KO_DOCKER_REPO)"
	@echo "Using namespace: $(NAMESPACE)"
	kustomize build config/dev/ | KO_DOCKER_REPO=$(KO_DOCKER_REPO) ko apply -f -
	@echo "Deployment complete!"

.PHONY: deploy-with-knative-setup
deploy-with-knative-setup: setup-knative deploy ## Setup Knative and deploy the service

.PHONY: undeploy
undeploy: ## Remove the service deployment
	@echo "Removing conforma-verifier-listener..."
	kustomize build config/dev/ | ko delete -f -
	@echo "Undeployment complete!"

.PHONY: logs
logs: ## Show logs from the service
	@echo "Showing logs from conforma-verifier-listener..."
	kubectl logs -f deployment/conforma-verifier-listener -n $(NAMESPACE)

.PHONY: status
status: ## Show deployment status
	@echo "Deployment status:"
	kubectl get all -l app=conforma-verifier-listener -n $(NAMESPACE)
	@echo ""
	@echo "Knative Service status:"
	kubectl get ksvc conforma-verifier-listener -n $(NAMESPACE) || echo "Knative Service not found"
	@echo ""
	@echo "Event sources:"
	kubectl get apiserversource -n $(NAMESPACE)
	@echo ""
	@echo "Triggers:"
	kubectl get trigger -n $(NAMESPACE)

.PHONY: clean
clean: ## Clean up all resources
	@echo "Cleaning up all resources..."
	ko delete -k config/
	kubectl delete namespace knative-serving --ignore-not-found=true
	kubectl delete namespace knative-eventing --ignore-not-found=true
	@echo "Cleanup complete!"

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	cd cmd/launch-taskrun && go test -v

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	cd cmd/launch-taskrun && go test -v -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

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