# Test Cluster Setup

This directory contains Kustomize manifests for setting up a complete test cluster with all dependencies required for running the knative-service acceptance tests.

## Components

The kustomization installs the following components:

### 1. Tekton Pipelines (v0.56.0)
- Provides TaskRun and Pipeline CRDs
- Required for executing enterprise contract verification tasks
- Source: https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.56.0/release.yaml

### 2. Knative Eventing (v1.12.0)
- Core eventing components for event-driven architecture
- In-Memory Channel for simple event routing
- Sources:
  - https://github.com/knative/eventing/releases/download/knative-v1.12.0/eventing-crds.yaml
  - https://github.com/knative/eventing/releases/download/knative-v1.12.0/eventing-core.yaml
  - https://github.com/knative/eventing/releases/download/knative-v1.12.0/in-memory-channel.yaml

### 3. Snapshot CRD (appstudio.redhat.com/v1alpha1)
- Custom Resource Definition for Snapshot resources
- Defines the schema for application snapshots
- Includes component specifications with container images

### 4. Image Registry
- In-cluster container registry for test images
- Configured via `../registry` kustomization
- Uses a generator plugin for dynamic port allocation

## Usage

### Apply to Cluster

The acceptance tests automatically apply these manifests when creating the test cluster. To manually apply:

```bash
# From repository root
kubectl apply -k hack/test/ --load-restrictor=LoadRestrictionsNone

# Wait for all components to be ready
kubectl wait --for=condition=ready pod --all -n tekton-pipelines --timeout=5m
kubectl wait --for=condition=ready pod --all -n knative-eventing --timeout=5m
```

### Verify Installation

Check that all components are running:

```bash
# Tekton
kubectl get pods -n tekton-pipelines

# Knative Eventing
kubectl get pods -n knative-eventing

# Snapshot CRD
kubectl get crds snapshots.appstudio.redhat.com
```

### Test a Snapshot

Create a test snapshot to verify the setup:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  name: test-snapshot
  namespace: conforma
spec:
  application: test-app
  displayName: Test Snapshot
  components:
    - name: test-component
      containerImage: "quay.io/example/image:latest"
EOF
```

## Architecture

```
┌─────────────────┐
│  Tekton         │  TaskRun execution
│  Pipelines      │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│  Knative        │
│  Eventing       │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│  ApiServerSource│
└─────────────────┘
         │
         ▼
┌─────────────────┐    ┌─────────────────┐
│  Service        │◀───│  Snapshot       │
│  (under test)   │    │  Resources      │
└─────────────────┘    └─────────────────┘
```

## Configuration



## Versions

All component versions are pinned for reproducibility:

- **Tekton Pipelines**: v0.56.0
- **Knative Eventing**: v1.12.0
- **Snapshot CRD**: v1alpha1

To update versions, modify the URLs in `kustomization.yaml`.

## Troubleshooting

### Pods Not Starting

If pods fail to start, check resource availability:

```bash
kubectl describe pod <pod-name> -n <namespace>
```


### CRD Issues

If Snapshot CRD is not recognized:

```bash
kubectl get crd snapshots.appstudio.redhat.com
kubectl describe crd snapshots.appstudio.redhat.com
```


## Development

When adding new components:

1. Add the resource URL to `kustomization.yaml` under `resources:`
2. Add any required patches under `patches:`
3. Update this README with component information
4. Test with `kubectl apply -k hack/test/`

## Related

- [Acceptance Tests](../../acceptance/README.md) - Full test suite documentation
- [Registry Setup](../registry/README.md) - In-cluster registry configuration
- [Main README](../../README.md) - Project overview
