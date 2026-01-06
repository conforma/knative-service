# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, GitHub Copilot, Cursor, etc.) when working with code in this repository.

## Project Overview

This is a Kubernetes-native, event-driven service that automatically triggers enterprise contract verification for application snapshots using Tekton bundles. It listens for CloudEvents when Snapshot resources are created and automatically creates Tekton TaskRuns for compliance verification.

**Key Tech Stack:**
- Go 1.25.0
- Knative Eventing (CloudEvents-based)
- Tekton Pipelines (TaskRun execution)
- Kubernetes client-go and controller-runtime
- Kind for local development

## Common Commands

### Build & Test
```bash
# Run unit tests
make test                    # Verbose output
make quiet-test             # Minimal output
make test-coverage          # Generate coverage report

# Run acceptance tests (full end-to-end)
make acceptance

# Build locally
make build-local            # Build without pushing to registry

# Linting and formatting
make lint                   # Run golangci-lint
make fmt                    # Format code
make tidy                   # Tidy go modules
make license-check          # Check license headers
make license-add            # Add missing license headers
```

### Development Workflow
```bash
# 1. Setup Knative cluster (one-time)
make setup-knative

# 2. Deploy to local kind cluster (auto-detects environment)
make deploy-local

# 3. Test with sample snapshot
make test-local

# 4. View logs and status
make logs
make status

# 5. Redeploy after code changes
make deploy-local           # Rebuilds and redeploys automatically
```

### Deployment Modes
```bash
# Auto mode (default): Detects kind cluster and uses optimized local build
make deploy-local

# Registry mode: Use existing images from registry (no build)
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=quay.io/conforma/knative-service:v1.2.3

# Custom namespace
make deploy-local NAMESPACE=my-namespace
```


### Acceptance Tests
```bash
# Run all scenarios
make acceptance

# Run with persistence for debugging
cd acceptance && go test . -args -persist

# Run against persisted environment
cd acceptance && go test . -args -restore

# Run specific scenarios by tag
cd acceptance && go test . -args -tags=@focus

# Control concurrency
ACCEPTANCE_CONCURRENCY=2 make acceptance
```

## Architecture

### High-Level Flow

```
Kubernetes API (Snapshot created)
  ↓
Knative Eventing (ApiServerSource + Trigger)
  ↓
CloudEvents HTTP delivery to /events endpoint
  ↓
Service processes Snapshot:
  1. Reads ConfigMap (vsa-config) from service namespace
  2. Finds EnterpriseContractPolicy via ReleasePlan/ReleasePlanAdmission lookup
  3. Creates Kubernetes Job using vsajob library
  ↓
Job executes VSA generation CLI
  ↓
VSA (Verification Summary Attestation) uploaded to Rekor
```

### Code Organization

```
cmd/trigger-vsa/           # Main service entry point
├── main.go                   # HTTP server, CloudEvent handling
├── main_test.go              # Tests for CloudEvent handler
├── k8s/                      # Kubernetes client utilities
│   └── client.go             # K8s config and controller-runtime client setup
└── mocks/                    # Generated mocks for testing
    └── CloudEventsClient.go  # Mock for CloudEventsClient interface

vsajob/                       # Reusable library for VSA job creation
├── interface.go              # Executor interface definition
├── executor.go               # Main implementation
├── executor_test.go          # Comprehensive unit tests
├── policy_finder.go          # EnterpriseContractPolicy lookup logic
├── konflux_types.go          # Snapshot, ReleasePlan, ReleasePlanAdmission CRDs
├── options.go                # Configuration structs
├── mocks/                    # Generated mocks (internal use)
│   └── controllerRuntimeClient.go
└── vsajobtest/               # Mocks for external library users
    └── mock_executor.go      # MockExecutor for testing code that uses vsajob

config/
├── base/                     # Base Kubernetes manifests
│   ├── knative-service.yaml  # Regular K8s Deployment (NOT Knative Serving)
│   ├── event-source.yaml     # ApiServerSource for Snapshot events
│   ├── trigger.yaml          # Routes CloudEvents to service
│   ├── configmap.yaml        # VSA Job configuration
│   ├── rbac.yaml             # Service account and permissions
│   └── task-runner-rbac.yaml # Job execution RBAC
├── dev/                      # Development overlay
└── test/                     # Test configurations

acceptance/                   # BDD acceptance tests using Godog
├── acceptance_test.go        # Test suite entry point
├── kubernetes/               # Cluster management steps
├── knative/                  # Knative installation and deployment steps
├── snapshot/                 # Snapshot resource creation steps
├── tekton/                   # TaskRun verification steps
├── vsa/                      # VSA and Rekor integration steps
└── testenv/                  # Test environment utilities

features/                     # Cucumber/Gherkin test scenarios
└── knative_service.feature
```

### vsajob Library Architecture

The `vsajob` package is a reusable library that handles all VSA job creation logic:

- **Executor Interface**: Main entry point for creating VSA generation jobs
- **Configuration Management**: Reads job settings from ConfigMap (container image, resources, secrets)
- **Policy Discovery**: Traverses ReleasePlan → ReleasePlanAdmission → EnterpriseContractPolicy
- **Job Creation**: Creates Kubernetes Jobs with proper configuration

External users can use this library in their own controllers:

```go
import "github.com/conforma/knative-service/vsajob"

executor := vsajob.NewExecutor(k8sClient, logger).
    WithNamespace(os.Getenv("POD_NAMESPACE")).
    WithConfigMapName("vsa-config")

snapshot := vsajob.Snapshot{
    Name:      "my-snapshot",
    Namespace: "my-namespace",
    Spec:      snapshotSpecJSON,
}

err := executor.CreateVSAJob(ctx, snapshot)
```

For testing code that uses the vsajob library, use the exported mock:

```go
import "github.com/conforma/knative-service/vsajob/vsajobtest"

mockExecutor := vsajobtest.NewMockExecutor(t)
mockExecutor.EXPECT().CreateVSAJob(mock.Anything, mock.Anything).Return(nil)
```

### Configuration

Configuration is loaded from ConfigMap (default name: `vsa-config`):

**Required fields:**
- `PUBLIC_KEY`: Public key for VSA verification
- `VSA_UPLOAD_URL`: Rekor server URL for attestation upload
- `VSA_SIGNING_KEY_SECRET_NAME`: Secret containing signing key

**Optional fields:**
- `GENERATOR_IMAGE`: Container image for VSA generation (default: latest)
- `SERVICE_ACCOUNT_NAME`: ServiceAccount for job execution
- `CPU_REQUEST`, `MEMORY_REQUEST`, `MEMORY_LIMIT`: Resource limits
- `BACKOFF_LIMIT`: Job retry limit
- `WORKERS`: Parallel workers for VSA generation
- `STRICT`: Strict mode for policy enforcement
- `IGNORE_REKOR`: Skip Rekor upload
- `DEBUG`: Enable debug logging

### Enterprise Contract Policy Lookup

The service finds the correct policy through a multi-step lookup:

1. Extract `application` field from Snapshot spec
2. Find `ReleasePlan` for that application in Snapshot's namespace
3. Extract RPA namespace and name from ReleasePlan labels
4. Find `ReleasePlanAdmission` in RPA namespace
5. Extract `EnterpriseContractPolicy` reference from RPA spec

If no ReleasePlan/RPA is found, the service returns `nil` without error (assumes Snapshot won't be released).

See [vsajob/policy_finder.go](vsajob/policy_finder.go) for implementation.

### Deployment Architecture

**IMPORTANT**: This service uses a **regular Kubernetes Deployment**, NOT Knative Serving. The name "knative-service" refers to it being a service that integrates with Knative Eventing for event delivery.

## Testing Strategy

### Unit Tests

**cmd/trigger-vsa/main_test.go:**
- Tests CloudEvent handling and HTTP layer
- Uses mockery-generated mocks (`vsajobtest.MockExecutor`, `mocks.CloudEventsClient`)
- Uses `testr.New(t)` for test logger

**vsajob/executor_test.go:**
- Comprehensive tests for VSA job creation logic
- Tests all error conditions (invalid config, missing policy, job creation failures)
- Uses table-driven tests with mockery-generated mocks
- Tests configuration validation and defaults

**Mock Generation:**
```bash
# Generate all mocks
go generate ./...

# Mocks are generated via go:generate directives:
# - vsajob/interface.go: Generates vsajobtest.MockExecutor
# - cmd/trigger-vsa/main.go: Generates mocks.CloudEventsClient
# - vsajob/executor.go: Generates internal mocks.ControllerRuntimeClient
```

### Acceptance Tests

- Production-ready BDD tests using Godog/Cucumber
- Real Kubernetes cluster (kind) with actual Knative and Tekton installations
- Complete end-to-end verification including VSA creation and Rekor upload
- See [acceptance/README.md](acceptance/README.md) for detailed documentation

## Development Notes

### Building and Deploying with ko

This project uses [ko](https://github.com/ko-build/ko) for building and deploying:
- `ko build`: Builds Go binaries and creates container images
- `ko apply`: Applies Kubernetes manifests with image references resolved
- Image references use `ko://` prefix in manifests (e.g., `ko://github.com/conforma/knative-service/cmd/trigger-vsa`)

### Local Development with Kind

The Makefile automatically optimizes for kind clusters:
- Builds images locally with `ko build --local`
- Loads images directly into kind cluster (no registry needed)
- Much faster than pushing to remote registry (~90s vs 3-6min)

### Namespace Isolation

- **Default deployment**: Uses `conforma` namespace (or `NAMESPACE` env var)
- Multiple deployments can coexist using different namespace configurations

### Configuration via ConfigMap

The service reads the ConfigMap from its own namespace (via `POD_NAMESPACE` env var), not from the Snapshot's namespace. This allows centralized configuration.

### Health Check Endpoint

The service exposes `/health` endpoint for readiness/liveness probes.

### CloudEvent Filtering

Only processes events with:
- Type: `dev.knative.apiserver.resource.add`
- Kind: `Snapshot`
- APIVersion: `appstudio.redhat.com/v1alpha1`

### Running Single Tests

```bash
# Run specific test function
go test ./vsajob -v -run TestCreateVSAJob

# Run acceptance test scenario by tag
cd acceptance && go test . -args -tags=@wip
```

## Important Gotchas

1. **ConfigMap namespace**: Service reads config from its own namespace, not Snapshot namespace
2. **Not Knative Serving**: Despite the name, uses regular K8s Deployment
3. **ReleasePlan lookup**: If no ReleasePlan found, VSA creation is skipped (returns `nil`, not an error)
4. **Image references**: Use `ko://` prefix in manifests for ko to resolve
5. **Kind image loading**: Must use `kind load docker-image` when building locally
6. **License headers**: All source files require Apache 2.0 license headers (use `make license-add`)
7. **Mock location**: External mocks in `vsajobtest/` package (doesn't affect coverage), internal mocks in `mocks/`

## Troubleshooting

### Tests Failing
```bash
# Check if kind cluster is running
kubectl cluster-info

# Recreate kind cluster
make setup-knative

# Check Tekton installation
make check-knative
```

### Deployment Issues
```bash
# Check pod status
kubectl get pods -l app=conforma-knative-service

# View events
kubectl get events --sort-by='.lastTimestamp'

# Check logs
make logs
```

### Acceptance Test Issues
```bash
# Persist environment for debugging
cd acceptance && go test . -args -persist

# Then manually inspect cluster
kubectl get all -A
kubectl logs <pod-name>

# Clean up persisted environment
kind delete cluster --name <cluster-name>
```

## CI/CD

GitHub Actions workflows:
- `.github/workflows/acceptance.yml`: Runs acceptance tests on PR and main branch
- Automatically sets up kind cluster, deploys service, and runs all test scenarios
