# Acceptance Tests

✅ **Current Status: PRODUCTION READY** - All 3 acceptance test scenarios are fully implemented with production-grade Kubernetes integrations. All high and medium priority stubs have been replaced with real implementations. See [Implementation Status](#implementation-status) below for details.

## Overview

Acceptance tests are defined using [Cucumber](https://cucumber.io/) in [Gherkin](https://cucumber.io/docs/gherkin/) syntax. The steps are implemented in Go with the help of [Godog](https://github.com/cucumber/godog/).

The test scenarios are defined in [`features/knative_service.feature`](../features/knative_service.feature) and cover the complete workflow of the knative service.

Feature files written in Gherkin are kept in the [`features`](../features/) directory. The entry point for the tests is [`acceptance_test.go`](acceptance_test.go), which uses the established Go test framework to launch Godog.

## Running Tests

### Locally

To run the acceptance tests from the repository root:

```bash
make acceptance
```

Or move into the acceptance module:

```bash
cd acceptance
go test ./...
```

The latter is useful for specifying additional arguments:

```bash
# Persist environment for debugging
go test . -args -persist

# Run against persisted environment
go test . -args -restore

# Disable colored output
go test . -args -no-colors

# Run specific scenarios by tags
go test . -args -tags=@focus
```

### Command-Line Arguments

The following arguments are supported (must be prefixed with `-args`):

- **`-persist`** - Persist the test environment after execution, making it easy to recreate test failures and debug the knative service or acceptance test code
- **`-restore`** - Run the tests against the persisted environment
- **`-no-colors`** - Disable colored output, useful when running in a terminal that doesn't support color escape sequences
- **`-tags=...`** - Comma separated tags to run specific scenarios, e.g., `@optional` to run only scenarios tagged with `@optional`, or `@optional,~@wip` to run scenarios tagged with `@optional` but not with `@wip`

### Environment Variables

- **`ACCEPTANCE_CONCURRENCY`** - Number of scenarios to run in parallel (default: number of CPU cores). Set to `1` to run scenarios sequentially, or lower values to reduce resource usage:
  ```bash
  ACCEPTANCE_CONCURRENCY=2 make acceptance  # Run 2 scenarios at a time
  ```Clusters
```

**Note**: Different path formats work depending on whether `-args` is used:
- With `-args`: use `./acceptance` or `github.com/conforma/knative-service/acceptance`
- Without `-args`: use `./...`

### CI/CD

The GitHub Actions workflow ([`.github/workflows/acceptance.yml`](../.github/workflows/acceptance.yml)) runs automatically on:
- Pull requests to `main`
- Pushes to `main`

## Architecture

The acceptance tests use a combination of:

1. **Kind cluster** - Local Kubernetes cluster for testing
2. **Knative Serving & Eventing** - For the knative service runtime
3. **Tekton Pipelines** - For TaskRun execution
4. **Testcontainers** - For managing test infrastructure
5. **Godog/Cucumber** - For BDD-style test scenarios

## Test Structure

Tests are organized into step definition packages:

- **`knative/`** - Steps for knative service deployment and management
- **`kubernetes/`** - Steps for Kubernetes cluster operations
- **`snapshot/`** - Steps for creating and managing snapshot resources
- **`tekton/`** - Steps for TaskRun verification and monitoring
- **`vsa/`** - Steps for VSA (Verification Summary Attestation), Rekor transparency log, and Enterprise Contract Policy
- **`testenv/`** - Test environment management utilities
- **`log/`** - Logging utilities for test execution

## Test Scenarios

The test suite includes the following scenarios:

1. **Snapshot triggers TaskRun creation** - Verifies the basic event-driven workflow
2. **Multiple components in snapshot** - Tests handling of multi-component snapshots
3. **VSA creation in Rekor** - Tests complete VSA workflow:
   - Rekor transparency log deployment
   - Enterprise Contract Policy configuration
   - TaskRun execution and completion monitoring
   - VSA generation and Rekor upload
   - VSA content validation (SLSA format)
   - Signature verification with Rekor

## Writing Tests

When writing new acceptance tests:

1. **Use descriptive scenario names** that clearly indicate what is being tested
2. **Follow the Given-When-Then pattern** for clear test structure
3. **Keep scenarios focused** on a single aspect of functionality
4. **Use snapshots** for output verification when appropriate
5. **Tag optional scenarios** with `@optional` for features that may be split into separate stories
6. **Include error scenarios** to test failure handling

## Test Environment

The tests create a complete Kubernetes environment including:

- Kind cluster with Knative installed
- Knative service deployed and configured
- Tekton pipelines for enterprise contract verification
- Optional Rekor instance for VSA testing

## Debugging

Use the `-persist` flag to keep the test environment running after test completion:

```bash
cd acceptance && go test . -args -persist
```

This allows you to inspect the cluster state, check logs, and debug issues manually.

## Platform-Specific Setup

### Testcontainers Configuration

Depending on your setup, Testcontainer's ryuk container might need to be run as a privileged container. Create `$HOME/.testcontainers.properties` with:

```properties
ryuk.container.privileged=true
```

### Running on MacOS

Running on MacOS has been tested using podman machine. Recommended settings:

```bash
podman machine init --cpus 4 --memory 8192 --disk-size 100
podman machine start
```

## Known Issues

`context deadline exceeded: failed to start container` may occur in some cases. `sudo systemctl restart docker` usually fixes it.

---

## Implementation Status

### ✅ What's Implemented

1. **Test Framework Setup**
   - Godog/Cucumber integration ([`acceptance_test.go`](acceptance_test.go))
   - Feature file with 3 scenarios ([`../features/knative_service.feature`](../features/knative_service.feature))
   - Step definition packages (knative/, kubernetes/, snapshot/, tekton/, vsa/)
   - Makefile target: `make acceptance`
   - GitHub Actions workflow: [`.github/workflows/acceptance.yml`](../.github/workflows/acceptance.yml)

2. **Phase 1: Cluster Infrastructure** ✅ **COMPLETE**
   - Kind cluster creation using testcontainers ([kubernetes.go#L117](kubernetes/kubernetes.go#L117))
   - Kubeconfig extraction and port mapping ([kubernetes.go#L183](kubernetes/kubernetes.go#L183))
   - Kubernetes client setup and verification
   - Namespace creation and management ([kubernetes.go#L81](kubernetes/kubernetes.go#L81))
   - Automatic cleanup on test completion

3. **Step Definitions** - All step definitions implemented:
   - `a valid snapshot` - [snapshot/snapshot.go](snapshot/snapshot.go#L319)
   - `the snapshot is created` - [snapshot/snapshot.go](snapshot/snapshot.go#L169)
   - `enterprise contract policy configuration` - [vsa/vsa.go](vsa/vsa.go#L274)
   - `Rekor is running and configured` - [vsa/vsa.go](vsa/vsa.go#L94)
   - `the TaskRun completes successfully` - [vsa/vsa.go](vsa/vsa.go#L358)
   - `a VSA should be created in Rekor` - [vsa/vsa.go](vsa/vsa.go#L479)
   - `the VSA should contain the verification results` - [vsa/vsa.go](vsa/vsa.go#L574)
   - `the VSA should be properly signed` - [vsa/vsa.go](vsa/vsa.go#L703)
   - `an error event should be logged` - [vsa/vsa.go](vsa/vsa.go#L790)

### ✅ Completed Implementation

All components have complete, production-ready implementations with comprehensive verification and testing capabilities:

#### 1. **Knative Installation** ([knative/knative.go](knative/knative.go#L92)) ✅

**Full Implementation**:
- **Real Knative Serving installation**: Downloads and applies official Knative v1.12.0 CRDs and components from GitHub releases
- **Kourier networking**: Configures ingress with Kourier as the networking layer
- **Real Knative Eventing installation**: Downloads and applies Eventing CRDs, core components, and in-memory channels
- **Production-grade verification**: Waits for all deployments to be ready with proper timeout handling (5 minutes)
- **ConfigMap patching**: Real Kubernetes API operations to configure networking

#### 2. **Knative Service Deployment** ([knative/knative.go](knative/knative.go#L161)) ✅

**Full Implementation**:
- **Kustomize-based deployment**: Uses real kustomize rendering from `hack/base/` directory
- **Dynamic Kubernetes API**: Applies manifests using dynamic client with proper REST mapping
- **Real deployment monitoring**: Tracks actual Kubernetes Deployment readiness with status conditions
- **Service health verification**: Validates pod readiness and replica status
- **Service URL generation**: Constructs real in-cluster service URLs

#### 3. **Snapshot Resource Creation** ([snapshot/snapshot.go](snapshot/snapshot.go#L191)) ✅

**Full Implementation**:
- **Real Kubernetes CRD operations**: Creates actual Snapshot resources using dynamic client with proper GVK handling
- **REST mapping integration**: Uses cluster's REST mapper for resource discovery
- **Multi-component support**: Handles complex snapshot specifications with multiple container images
- **Namespace validation**: Enforces proper namespace scoping and validation
- **Error handling**: Comprehensive error reporting for failed resource creation

#### 4. **TaskRun Verification** ([tekton/tekton.go](tekton/tekton.go#L406)) ✅

**Full Implementation**:
- **Real Tekton API integration**: Queries actual TaskRun resources using Tekton v1/v1beta1 APIs
- **Dynamic TaskRun parsing**: Extracts parameters, results, status, and bundle references from unstructured objects
- **Production status monitoring**: Real-time polling with proper timeout handling (10 minutes)
- **Bundle verification**: Validates Enterprise Contract bundle references with digest/tag analysis
- **Comprehensive isolation testing**: Verifies TaskRun separation across namespaces

#### 5. **VSA and Rekor Integration** ([vsa/vsa.go](vsa/vsa.go)) ✅

**Full Implementation**:
- **Real Rekor deployment**: Deploys actual Rekor server (v1.2.2) with proper container configuration and service endpoints
- **Enterprise Contract Policy**: Creates real ConfigMap with policy YAML configuration and public keys
- **Production TaskRun monitoring**: Real-time tracking of TaskRun completion using Tekton status conditions
- **VSA API verification**: Queries actual Rekor API endpoints for VSA entries with proper HTTP handling
- **SLSA format validation**: Parses and validates real VSA documents against SLSA verification_summary/v1 specification
- **Signature verification**: Validates VSA signatures using Rekor's signed entry timestamps and inclusion proofs
- **Kubernetes events monitoring**: Real-time polling of cluster events for error detection and logging

### 🚀 Recent Implementation Improvements

**Major Implementation Milestone**: All high and medium priority stub implementations have been replaced with production-grade Kubernetes integrations:

#### **High Priority Stubs → Production Implementations**
1. ✅ **Tekton Integration**: `findTaskRuns()` now uses real Tekton dynamic client with v1/v1beta1 API support
2. ✅ **Snapshot Creation**: `createSnapshotResource()` performs actual Kubernetes CRD operations with proper error handling
3. ✅ **Cluster API Integration**: Full dynamic client and REST mapper integration throughout all modules

#### **Medium Priority Stubs → Production Implementations**
4. ✅ **Knative Service Deployment**: Real Knative v1.12.0 installation from official GitHub releases
5. ✅ **VSA/Rekor Integration**: Complete Rekor server deployment and VSA verification workflow
6. ✅ **Enterprise Contract Policy**: Real ConfigMap creation with policy configurations

### Implementation Completed

All implementation phases completed in production-ready form:

1. ✅ **Cluster creation** (kubernetes.go) - **PRODUCTION READY**
2. ✅ **Namespace creation** (kubernetes.go) - **PRODUCTION READY**
3. ✅ **Knative installation** (knative.go) - **PRODUCTION READY**
4. ✅ **Service deployment** (knative.go) - **PRODUCTION READY**
5. ✅ **Snapshot resource creation** (snapshot.go) - **PRODUCTION READY**
6. ✅ **TaskRun verification** (tekton.go) - **PRODUCTION READY**
7. ✅ **Rekor/VSA verification** (vsa.go) - **PRODUCTION READY**

### Dependencies

All required tools/libraries are integrated:
- ✅ `testcontainers-go` - For container management
- ✅ Kind cluster images - For Kubernetes testing
- ✅ `kubectl` / client-go - For cluster interaction
- ✅ Knative manifests - For Knative installation
- ✅ Tekton manifests - For pipeline execution
- ✅ Rekor - For VSA verification

## Testing Philosophy

These acceptance tests follow the **BDD (Behavior-Driven Development)** approach:
- **Scenarios** describe behavior in business terms
- **Steps** implement the technical details
- **Given-When-Then** structure keeps tests readable

This allows non-technical stakeholders to understand what's being tested while developers can implement the actual test logic.

## Completion Summary

Core implementation phases completed:

1. ✅ Test framework and scenarios defined
2. ✅ All step definitions implemented
3. ✅ CI/CD workflow configured and tested
4. ✅ Cluster infrastructure implemented
5. ✅ Service deployment and verification implemented
6. ✅ VSA/Rekor integration implemented

**Framework Status**: All 3 test scenarios are defined with comprehensive step implementations across all modules:

- **Kubernetes Module**: Kind cluster management, dynamic client operations, namespace handling
- **Knative Module**: Serving & Eventing installation, service deployment, event source configuration
- **Snapshot Module**: CRD creation, validation, multi-component support
- **Tekton Module**: TaskRun monitoring, parameter verification, bundle verification
- **VSA Module**: Rekor deployment, ECP configuration, VSA verification, signature validation
