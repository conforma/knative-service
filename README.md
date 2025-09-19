# Conforma Knative Service

A Kubernetes-native, event-driven service that automatically triggers enterprise contract verification for application snapshots using Tekton bundles.

## Overview

The Conforma Knative Service is a CloudEvents-based service that monitors for the creation of Snapshot resources and automatically triggers compliance verification workflows. It implements an event-driven architecture to bridge CloudEvents with Tekton pipelines, using bundle resolution to dynamically fetch verification tasks from container registries.

## Architecture

### Event-Driven Processing
- Listens for CloudEvents of type `dev.knative.apiserver.resource.add`
- Processes Snapshot resources from the `appstudio.redhat.com/v1alpha1` API
- Automatically creates Tekton TaskRuns for compliance verification

### Bundle Resolution
- Uses Tekton's bundle resolver to fetch tasks from `quay.io/conforma/tekton-task:latest`
- Eliminates the need for pre-installed tasks in the cluster
- Enables dynamic task updates without redeploying the service

### Configuration Management
- ConfigMap-based configuration with caching and TTL
- Supports multiple namespaces with isolated configuration
- Configurable parameters for policy verification

## Features

- **Automated Compliance**: Triggers verification workflows without manual intervention
- **Multi-Namespace Support**: Handles snapshots across different namespaces
- **Configurable Policies**: Supports custom policy configurations and public keys
- **Cloud-Native**: Stateless, horizontally scalable, and Kubernetes-native
- **Bundle-Based**: Dynamic task resolution from container registries

## Quick Start

### Prerequisites
- [kind](https://kind.sigs.k8s.io/) installed
- [ko](https://github.com/ko-build/ko) installed  
- [kn](https://knative.dev/docs/client/) with quickstart plugin
- [kubectl](https://kubernetes.io/docs/tasks/tools/) installed
- Docker daemon running

### 1. Setup Knative Cluster

```bash
make setup-knative
```

This creates a kind cluster named "knative" with both Knative Serving and Eventing installed.

### 2. Deploy the Service

```bash
make deploy-local
```

This will automatically:
- Build the Go application locally using `ko`
- Load the image into the kind cluster (for kind) or use existing registry images (for other clusters)
- Deploy all Kubernetes resources to the `default` namespace
- Wait for pods to be ready
- Display the service URL

**Expected Duration:** ~1-2 minutes for kind clusters

**Note**: This deploys to the `default` namespace. For staging-like testing in an isolated namespace, use `make deploy-staging-local` instead.

### 3. Test the Service

```bash
make test-local
```

This creates a sample Snapshot resource to verify the service is working correctly.

## Configuration

The service reads configuration from a ConfigMap named `taskrun-config` in each namespace:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: taskrun-config
  namespace: default
data:
  POLICY_CONFIGURATION: "github.com/enterprise-contract/config//slsa3"
  PUBLIC_KEY: |
    -----BEGIN PUBLIC KEY-----
    MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZP/0htjhVt2y0ohjgtIIgICOtQtA
    naYJRuLprwIv6FDhZ5yFjYUEtsmoNcW7rx2KM6FOXGsCX3BNc7qhHELT+g==
    -----END PUBLIC KEY-----
  IGNORE_REKOR: "true"
```

## Local Development

### Smart Deployment

The `make deploy-local` target automatically detects your environment and chooses the optimal deployment strategy:

#### Kind Clusters (Default)
- **Fast local deployment**: Uses `ko build --local` + `kind load docker-image`
- **No registry required**: Images are loaded directly into the kind cluster
- **Optimized for development**: ~90 second deployment cycle

#### Other Clusters
- **Registry-based deployment**: Uses existing images from `KO_DOCKER_REPO` registry
- **Production-like**: Tests with the same images used in production

### Flexible Deployment Modes

You can override the automatic detection using parameters:

```bash
# Default: Auto-detect environment (recommended)
make deploy-local

# Force registry mode (even on kind clusters)
make deploy-local DEPLOY_MODE=registry

# Force direct image loading (kind clusters only)
make deploy-local DEPLOY_MODE=auto
```

### Use Cases for Registry Mode

Use `DEPLOY_MODE=registry` when you want to:
- **Test with existing published images**: Use pre-built images from registries
- **Validate production images**: Test the exact same images used in production
- **Skip build time**: Deploy quickly without building locally
- **Test different versions**: Easily switch between different image tags

### Registry Mode Examples

```bash
# Use latest tag (automatically appended)
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=quay.io/conforma/knative-service

# Use specific version tag
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=quay.io/conforma/knative-service:v1.2.3

# Use development tag
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=quay.io/conforma/knative-service:main-abc123

# Use local registry
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=localhost:5000/myapp:dev
```

**Note**: Registry mode uses existing images from the specified registry - it does not build or push new images.

### Development Workflow

1. **Make code changes** in `cmd/launch-taskrun/`
2. **Redeploy**: `make deploy-local` (automatically optimized for your environment)
3. **Test**: `make test-local`
4. **View logs**: `make logs`
5. **Check status**: `make status`

### Performance Comparison

| Method | Build Time | Deploy Time | Total Time | Use Case |
|--------|------------|-------------|------------|----------|
| `make deploy-local` (kind) | ~30s | ~60s | ~90s | Development with local builds |
| `make deploy-local DEPLOY_MODE=registry` | 0s | ~30-60s | ~30-60s | Testing with existing images |
| Legacy registry build | ~60s | ~2-5min | ~3-6min | Building and pushing to registry |

## Staging-like Testing

Test locally using Red Hat App Studio staging configuration:

```bash
# Deploy using infra-deployments staging config
make deploy-staging-local

# This fetches the actual staging configuration from infra-deployments
# and deploys it locally in the 'conforma-local' namespace for realistic testing

# View logs from staging deployment
make logs-staging-local

# Clean up staging deployment
make undeploy-staging-local
```

### **Namespace Usage**

Our deployment targets use different namespaces for isolation:

| Target | Namespace | Purpose | Cleanup Method |
|--------|-----------|---------|----------------|
| `make deploy-local` | `default` (configurable via `NAMESPACE` env var) | Development workflow | File-based cleanup |
| `make deploy-staging-local` | `conforma-local` (fixed) | Staging-like testing | Namespace deletion |

**Examples:**
```bash
# Deploy to default namespace
make deploy-local

# Deploy to custom namespace
make deploy-local NAMESPACE=my-dev

# Deploy to staging namespace (always conforma-local)
make deploy-staging-local

# View logs from appropriate namespace
make logs                    # default namespace (or NAMESPACE value)
make logs-staging-local      # conforma-local namespace
```

## Make Targets

### Cluster Setup
- `make setup-knative` - Install and configure kind cluster with Knative
- `make check-knative` - Verify Knative installation

### Development
- `make build` - Build the service using ko
- `make build-local` - Build locally without pushing to registry
- `make test` - Run unit tests
- `make quiet-test` - Run tests without verbose output
- `make test-coverage` - Run tests with coverage report
- `make lint` - Run linter
- `make fmt` - Format code
- `make tidy` - Tidy go modules

### Local Deployment
- `make deploy-local` - Smart deployment (auto-detects environment)
- `make deploy-local DEPLOY_MODE=registry` - Use existing images from registry (no build)
- `make undeploy-local` - Remove local deployment
- `make logs` - View service logs
- `make test-local` - Test with sample snapshot
- `make status` - Show deployment status

### Staging-like Testing
- `make deploy-staging-local` - Deploy using infra-deployments staging config (conforma-local namespace)
- `make undeploy-staging-local` - Remove staging-local deployment (deletes conforma-local namespace)
- `make logs-staging-local` - View staging-local service logs (conforma-local namespace)

### Convenience
- `make help` - Show all available targets with descriptions

## Troubleshooting

### Image Pull Errors

If you see errors like "Unable to fetch image":
- The image wasn't properly loaded into the kind cluster
- The image name doesn't match what's expected

**Solution**: Use `make deploy-local` which automatically handles image loading for kind clusters.

### Build Failures

If `ko build` fails:
- Ensure Go modules are tidy: `make tidy`
- Check that Docker daemon is running
- Verify ko is properly installed: `ko version`

### Pod Not Ready

If pods don't become ready:
- Check events: `kubectl get events -n default --sort-by='.lastTimestamp'`
- Check pod logs: `kubectl logs -l serving.knative.dev/service=conforma-knative-service -n default`
- Verify Knative is installed: `make check-knative`

### Deployment Mode Issues

If `deploy-local` hangs or fails when `KO_DOCKER_REPO` is set:
- **Fixed**: Direct mode now ignores `KO_DOCKER_REPO` and always builds locally
- **Root cause**: `ko build --local` was affected by registry environment variables
- **Solution**: Local builds now use `KO_DOCKER_REPO=ko.local` internally

### Namespace Conflicts

**Local and staging deployments are isolated:**
- `make deploy-local` → `default` namespace (or `NAMESPACE` env var)
- `make deploy-staging-local` → `conforma-local` namespace
- **They can coexist** without conflicts
- **Use appropriate cleanup**: `make undeploy-local` vs `make undeploy-staging-local`

## Usage

Once deployed, the service will automatically:

1. **Monitor** for Snapshot resource creation events
2. **Read** configuration from the snapshot's namespace
3. **Create** Tekton TaskRuns with the appropriate parameters
4. **Execute** enterprise contract verification using bundle resolution

### Example Snapshot
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  name: test-snapshot
  namespace: default
spec:
  application: application-sample
  displayName: test-snapshot
  displayDescription: my first snapshot
  components:
    - name: test-component
      containerImage: "quay.io/redhat-user-workloads/rhtap-contract-tenant/golden-container/golden-container@sha256:185f6c39e5544479863024565bb7e63c6f2f0547c3ab4ddf99ac9b5755075cc9"
```

## Architecture Diagram

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Kubernetes    │    │ Conforma         │    │   Tekton        │
│   API Server    │───▶│ Knative          │───▶│   Pipeline      │
│                 │    │ Service          │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌──────────────────┐
                       │ Bundle Registry  │
                       │ (quay.io/...)   │
                       └──────────────────┘
```

## Advanced Usage

### Custom Registry Configuration

```bash
# Use existing image from custom registry (latest tag)
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=your-registry/conforma

# Use specific version from custom registry
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=your-registry/conforma:v1.2.3

# Use image from local registry
make deploy-local DEPLOY_MODE=registry KO_DOCKER_REPO=localhost:5000/conforma:dev
```

### Manual Installation

Deploy using Kustomize and ko directly:

```bash
# Development (builds and pushes to registry)
kustomize build config/dev/ | ko apply -f -

# Using existing image (no build)
kustomize build config/dev/ | sed "s|ko://github.com/conforma/knative-service/cmd/launch-taskrun|your-registry/conforma:latest|g" | kubectl apply -f -
```

### Running Locally (Outside Kubernetes)

```bash
go run cmd/launch-taskrun/main.go
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run `make test` and `make lint`
6. Submit a pull request

### Development Tips

- **Use `make deploy-local`** for all development (automatically optimized, deploys to default namespace)
- **Use `make deploy-local DEPLOY_MODE=registry`** to test with existing published images
- **Use `make deploy-staging-local`** for testing with realistic staging configuration (deploys to conforma-local namespace)
- **Use `make help`** to see all available commands
- **Setup and deploy**: `make setup-knative && make deploy-local` for complete setup
- **Namespace isolation**: Local and staging deployments use separate namespaces and can coexist
- The locally built image includes your latest code changes
- Kind cluster persists between sessions unless deleted
- Use appropriate undeploy target for the namespace you want to clean up

## Container Images

The service is designed to be built and deployed using [ko](https://github.com/ko-build/ko), which handles container image building and deployment directly from Go source code.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
