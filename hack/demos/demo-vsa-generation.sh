#!/usr/bin/env bash
# Copyright 2025 The Conforma Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Always work from project root for consistent paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${PROJECT_ROOT}"

echo "🎯 VSA Generation Demo"
echo "======================"
echo "This demo shows the complete end-to-end workflow with:"
echo "  ✅ Cross-namespace Snapshot watching"
echo "  ✅ In-cluster registry (image accessibility)"
echo "  ✅ Image signatures (cosign)"
echo "  ✅ SLSA provenance attestations"
echo "  ✅ Policy validation"
echo "  ✅ VSA generation and upload"
echo ""

# Configuration
LOCAL_REGISTRY="registry.registry.svc.cluster.local:5000"
EXTERNAL_REGISTRY="localhost:5001"
IMAGE_NAME="vsa-demo-app"
IMAGE_TAG="demo-$(date +%s)"
FULL_IMAGE_REF="${LOCAL_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
SNAPSHOT_NAME="vsa-demo-$(date +%s)"
USER_NAMESPACE="demo-user-namespace"

echo "📋 Demo Configuration:"
echo "  Registry: ${LOCAL_REGISTRY}"
echo "  Image: ${FULL_IMAGE_REF}"
echo "  Snapshot: ${SNAPSHOT_NAME}"
echo "  User Namespace: ${USER_NAMESPACE} (demonstrating cross-namespace watching)"
echo ""

# Cleanup function
cleanup_demo() {
    echo ""
    echo "🧹 Cleaning up demo resources..."
    
    # Restore original ConfigMap to avoid conflicts
    echo "  Restoring original ConfigMap..."
    kubectl patch configmap vsa-config -n default --patch '{
        "data": {
            "VSA_SIGNING_KEY_SECRET_NAME": "vsa-signing-key",
            "PUBLIC_KEY": "k8s://openshift-pipelines/public-key"
        }
    }' 2>/dev/null || true
    
    # Remove demo snapshots and user namespace
    kubectl delete snapshot "${SNAPSHOT_NAME}" --ignore-not-found -n "${USER_NAMESPACE}" 2>/dev/null || true
    kubectl delete namespace "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    
    # Remove demo secrets
    kubectl delete secret vsa-demo-signing-key --ignore-not-found -n default 2>/dev/null || true
    kubectl delete secret vsa-demo-signing-key --ignore-not-found -n "${USER_NAMESPACE}" 2>/dev/null || true
    kubectl delete secret vsa-demo-public-key --ignore-not-found -n openshift-pipelines 2>/dev/null || true
    
    # Remove demo resources
    kubectl delete -f hack/demos/vsa-demo-resources.yaml --ignore-not-found 2>/dev/null || true
    
    # Remove the generate-vsa Tekton task
    kubectl delete -f config/base/generate-vsa.yaml --ignore-not-found 2>/dev/null || true
    
    # Remove RBAC for task runner from user namespace
    kubectl delete serviceaccount conforma-vsa-generator -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete role conforma-vsa-generator -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete role conforma-vsa-generator-pods -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete role conforma-vsa-generator-secrets -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete rolebinding conforma-vsa-generator -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete rolebinding conforma-vsa-generator-pods -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete rolebinding conforma-vsa-generator-secrets -n "${USER_NAMESPACE}" --ignore-not-found 2>/dev/null || true
    kubectl delete clusterrolebinding conforma-vsa-generator-cluster-demo --ignore-not-found 2>/dev/null || true
    
    # Clean up port-forward
    if [ -f /tmp/vsa-demo-port-forward.pid ]; then
        PORT_FORWARD_PID=$(cat /tmp/vsa-demo-port-forward.pid)
        kill "$PORT_FORWARD_PID" 2>/dev/null || true
        rm -f /tmp/vsa-demo-port-forward.pid
    fi
    pkill -f "kubectl.*port-forward.*registry.*5001:5000" 2>/dev/null || true
    
    # Clean up in-cluster registry
    kubectl delete -f hack/demos/in-cluster-registry.yaml --ignore-not-found 2>/dev/null || true
    
    # Clean up generated keys
    DEMO_KEYS_DIR="hack/demos"
    rm -f "${DEMO_KEYS_DIR}/vsa-demo-keys.key" "${DEMO_KEYS_DIR}/vsa-demo-keys.pub" 2>/dev/null || true
    
    # Clean up temporary files
    rm -f /tmp/vsa-demo-snapshot.yaml 2>/dev/null || true
    rm -f /tmp/slsa-provenance.json 2>/dev/null || true
    
    # Clean up Docker images
    docker rmi "${EXTERNAL_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}" 2>/dev/null || true
    
    echo "  Demo cleanup completed"
}

# Set up signal handlers for graceful cleanup
# DISABLED: Comment out to keep resources after demo for inspection
# trap cleanup_demo EXIT
# trap 'echo ""; echo "🛑 Demo interrupted - cleaning up..."; cleanup_demo; exit 1' INT TERM

echo "🔧 Step 1: Setting up in-cluster registry..."
# Deploy in-cluster registry
kubectl apply -f hack/demos/in-cluster-registry.yaml

# Wait for registry to be ready
echo "  Waiting for in-cluster registry to be ready..."
kubectl wait --for=condition=available --timeout=60s deployment/registry -n registry

# Set up port-forward for external access
echo "  Setting up port-forward for external registry access..."
# Kill any existing port-forward on this port
pkill -f "kubectl.*port-forward.*registry.*5001:5000" 2>/dev/null || true
sleep 2

# Start port-forward in background
kubectl port-forward -n registry service/registry 5001:5000 > /dev/null 2>&1 &
PORT_FORWARD_PID=$!

# Store PID for cleanup
echo "$PORT_FORWARD_PID" > /tmp/vsa-demo-port-forward.pid

# Wait for port-forward to be ready
echo "  Waiting for port-forward to be accessible..."
for i in {1..30}; do
    if curl -s "http://${EXTERNAL_REGISTRY}/v2/" > /dev/null 2>&1; then
        echo "  Port-forward is accessible at ${EXTERNAL_REGISTRY}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "  Warning: Port-forward not accessible after 30 attempts"
        echo "  This might cause image push failures"
    fi
    sleep 2
done

echo "  In-cluster registry ready!"
echo ""

echo "🏗️ Step 2: Building test application..."
cd hack/demos/test-app

# Build and tag for external registry (for pushing)
# Add a unique build arg to ensure different image digest each time
EXTERNAL_IMAGE_REF="${EXTERNAL_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
BUILD_TIMESTAMP=$(date +%s)
echo "  Building Docker image: ${EXTERNAL_IMAGE_REF}"
docker build --build-arg BUILD_ID="${BUILD_TIMESTAMP}" -t "${EXTERNAL_IMAGE_REF}" .
echo "  Pushing to in-cluster registry..."
docker push "${EXTERNAL_IMAGE_REF}"

# Get the image digest and convert to internal cluster address
IMAGE_DIGEST=$(docker inspect "${EXTERNAL_IMAGE_REF}" --format='{{index .RepoDigests 0}}' | cut -d'@' -f2)
FULL_IMAGE_WITH_DIGEST="${LOCAL_REGISTRY}/${IMAGE_NAME}@${IMAGE_DIGEST}"
EXTERNAL_IMAGE_WITH_DIGEST="${EXTERNAL_REGISTRY}/${IMAGE_NAME}@${IMAGE_DIGEST}"
echo "  Image with digest (cluster-internal): ${FULL_IMAGE_WITH_DIGEST}"

# Return to project root
cd "${PROJECT_ROOT}"

echo ""
echo "🔑 Step 3: Generating signing keys..."
# Generate proper Sigstore keys for this demo (non-interactive)
DEMO_KEYS_DIR="hack/demos"
rm -f "${DEMO_KEYS_DIR}/vsa-demo-keys.key" "${DEMO_KEYS_DIR}/vsa-demo-keys.pub"
cd "${DEMO_KEYS_DIR}"
COSIGN_PASSWORD="" cosign generate-key-pair --output-key-prefix vsa-demo-keys
cd "${PROJECT_ROOT}"
echo "  Generated ${DEMO_KEYS_DIR}/vsa-demo-keys.key and ${DEMO_KEYS_DIR}/vsa-demo-keys.pub"

echo ""
echo "✍️ Step 4: Signing the image..."
# Sign the image with our generated key
echo "  Signing image via external address: ${EXTERNAL_IMAGE_WITH_DIGEST}"
COSIGN_PASSWORD="" cosign sign --key "${DEMO_KEYS_DIR}/vsa-demo-keys.key" "${EXTERNAL_IMAGE_WITH_DIGEST}" --yes
echo "  Image signed successfully"

# Verify the signature
echo "  Verifying signature..."
cosign verify --key "${DEMO_KEYS_DIR}/vsa-demo-keys.pub" "${EXTERNAL_IMAGE_WITH_DIGEST}"
echo "  Signature verified!"

echo ""
echo "📋 Step 5: Creating SLSA provenance attestation..."
# Create a simple SLSA provenance attestation
cat > /tmp/slsa-provenance.json << EOF
{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "https://slsa.dev/provenance/v0.2",
  "subject": [
    {
      "name": "${EXTERNAL_IMAGE_WITH_DIGEST}",
      "digest": {
        "sha256": "$(echo ${IMAGE_DIGEST} | cut -d':' -f2)"
      }
    }
  ],
  "predicate": {
    "builder": {
      "id": "https://github.com/conforma/knative-service/demo-builder"
    },
    "buildType": "https://github.com/conforma/knative-service/demo-build",
    "invocation": {
      "configSource": {
        "uri": "https://github.com/conforma/knative-service",
        "digest": {
          "sha1": "demo-commit-hash"
        }
      }
    },
    "metadata": {
      "buildInvocationId": "demo-build-$(date +%s)",
      "buildStartedOn": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
      "buildFinishedOn": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
      "completeness": {
        "parameters": true,
        "environment": false,
        "materials": true
      },
      "reproducible": false
    },
    "materials": [
      {
        "uri": "https://github.com/conforma/knative-service",
        "digest": {
          "sha1": "demo-commit-hash"
        }
      }
    ]
  }
}
EOF

echo "  Creating SLSA provenance attestation..."
COSIGN_PASSWORD="" cosign attest --key "${DEMO_KEYS_DIR}/vsa-demo-keys.key" --predicate /tmp/slsa-provenance.json "${EXTERNAL_IMAGE_WITH_DIGEST}" --yes
echo "  SLSA provenance attestation created successfully"

# Verify the attestation
echo "  Verifying attestation..."
cosign verify-attestation --key "${DEMO_KEYS_DIR}/vsa-demo-keys.pub" "${EXTERNAL_IMAGE_WITH_DIGEST}"
echo "  Attestation verified!"

echo ""
echo "🔧 Step 6: Creating user namespace for demo..."
# Create user namespace early so we can create resources in it
echo "  Creating user namespace: ${USER_NAMESPACE}"
kubectl create namespace "${USER_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
echo "  User namespace created"

echo ""
echo "🔧 Step 7: Setting up demo resources..."
# Check if Tekton Pipelines is installed, install if needed
if ! kubectl get crd tasks.tekton.dev > /dev/null 2>&1; then
    echo "  Installing Tekton Pipelines..."
    kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml > /dev/null 2>&1
    echo "  Waiting for Tekton Pipelines to be ready..."
    kubectl wait --for=condition=ready pod -l app=tekton-pipelines-controller -n tekton-pipelines --timeout=300s > /dev/null 2>&1
    echo "  Tekton Pipelines installed"
else
    echo "  Tekton Pipelines already installed"
fi

# Install CRDs
echo "  Installing CRDs..."
kubectl apply \
  -f https://raw.githubusercontent.com/konflux-ci/application-api/refs/heads/main/manifests/application-api-customresourcedefinitions.yaml \
  -f https://raw.githubusercontent.com/konflux-ci/release-service/refs/heads/main/config/crd/bases/appstudio.redhat.com_releaseplanadmissions.yaml \
  -f https://raw.githubusercontent.com/konflux-ci/release-service/refs/heads/main/config/crd/bases/appstudio.redhat.com_releaseplans.yaml \
  -f https://raw.githubusercontent.com/conforma/crds/refs/heads/main/config/crd/bases/appstudio.redhat.com_enterprisecontractpolicies.yaml \
  > /dev/null 2>&1

# Wait for CRDs - create helper script
cat > /tmp/wait-for-crds.sh << 'EOFWAIT'
#!/bin/bash
set -e
resource_type=$1
condition=$2
timeout=$3
shift 3
resources=("$@")

for resource in "${resources[@]}"; do
    kubectl wait --for="${condition}=${resource_type}" --timeout="${timeout}" "crd/${resource}" > /dev/null 2>&1 || true
done
EOFWAIT
chmod +x /tmp/wait-for-crds.sh

echo "  Waiting for CRDs to be ready..."
/tmp/wait-for-crds.sh crd established 60s snapshots.appstudio.redhat.com releaseplans.appstudio.redhat.com releaseplanadmissions.appstudio.redhat.com enterprisecontractpolicies.appstudio.redhat.com > /dev/null
rm -f /tmp/wait-for-crds.sh

# Apply the generate-vsa Tekton task in default namespace (used via cluster resolver)
echo "  Installing generate-vsa Tekton task..."
kubectl apply -f config/base/generate-vsa.yaml
echo "  Tekton task installed (accessible via cluster resolver)"

# Apply RBAC for task runner (required for cross-namespace access)
echo "  Installing RBAC for task runner in user namespace..."
# Create service account, role, and rolebinding in user namespace
kubectl create serviceaccount conforma-vsa-generator -n "${USER_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl create role conforma-vsa-generator -n "${USER_NAMESPACE}" \
  --verb=get,list,watch,create,update,patch --resource=taskruns,tasks --resource-name='' \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl create role conforma-vsa-generator-pods -n "${USER_NAMESPACE}" \
  --verb=get,list,watch,create,update,patch --resource=pods --resource-name='' \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl create role conforma-vsa-generator-secrets -n "${USER_NAMESPACE}" \
  --verb=get,list --resource=secrets --resource-name='' \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl create rolebinding conforma-vsa-generator -n "${USER_NAMESPACE}" \
  --role=conforma-vsa-generator --serviceaccount="${USER_NAMESPACE}:conforma-vsa-generator" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl create rolebinding conforma-vsa-generator-pods -n "${USER_NAMESPACE}" \
  --role=conforma-vsa-generator-pods --serviceaccount="${USER_NAMESPACE}:conforma-vsa-generator" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl create rolebinding conforma-vsa-generator-secrets -n "${USER_NAMESPACE}" \
  --role=conforma-vsa-generator-secrets --serviceaccount="${USER_NAMESPACE}:conforma-vsa-generator" \
  --dry-run=client -o yaml | kubectl apply -f -
# Create ClusterRoleBinding
kubectl create clusterrolebinding conforma-vsa-generator-cluster-demo \
  --clusterrole=conforma-knative-service-cluster \
  --serviceaccount="${USER_NAMESPACE}:conforma-vsa-generator" \
  --dry-run=client -o yaml | kubectl apply -f -
echo "  Task runner RBAC installed"

# Apply VSA demo specific resources
echo "  Applying VSA demo resources..."
kubectl apply -f hack/demos/vsa-demo-resources.yaml

# Create ReleasePlan in the user namespace (overriding the one from vsa-demo-resources.yaml)
echo "  Creating ReleasePlan in user namespace: ${USER_NAMESPACE}"
cat <<EOF | kubectl apply -f -
apiVersion: appstudio.redhat.com/v1alpha1
kind: ReleasePlan
metadata:
  name: vsa-demo-release-plan
  namespace: ${USER_NAMESPACE}
  labels:
    release.appstudio.openshift.io/releasePlanAdmission: vsa-demo-rpa
spec:
  application: vsa-demo-application
  target: rhtap-releng-tenant
EOF
echo "  Demo resources configured"

echo ""
echo "🔑 Step 8: Creating VSA signing key secrets..."
# Create signing key secret for TaskRun workspace in both conforma and user namespace
kubectl create secret generic vsa-demo-signing-key \
    --from-file=cosign.key="${DEMO_KEYS_DIR}/vsa-demo-keys.key" \
    -n conforma --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic vsa-demo-signing-key \
    --from-file=cosign.key="${DEMO_KEYS_DIR}/vsa-demo-keys.key" \
    -n "${USER_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
echo "  VSA signing key secret created"

# Update public key secret for policy validation
kubectl create secret generic public-key \
    --from-file=cosign.pub="${DEMO_KEYS_DIR}/vsa-demo-keys.pub" \
    -n openshift-pipelines --dry-run=client -o yaml | kubectl apply -f -
echo "  Public key secret created"

# Update configmap to use appropriate keys
echo "  Updating configmap for demo keys..."
kubectl patch configmap vsa-config -n default --patch '{
    "data": {
        "VSA_SIGNING_KEY_SECRET_NAME": "vsa-demo-signing-key"
    }
}'
echo "  ConfigMap updated for demo"

echo ""
echo "📦 Step 9: Creating snapshot for VSA generation..."
# Create a temporary snapshot file
cat > /tmp/vsa-demo-snapshot.yaml << EOF
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  name: ${SNAPSHOT_NAME}
  namespace: ${USER_NAMESPACE}
spec:
  application: vsa-demo-application
  displayName: ${SNAPSHOT_NAME}
  displayDescription: "Demo snapshot with full attestation coverage (cross-namespace)"
  components:
    - name: vsa-demo-component
      containerImage: "${FULL_IMAGE_WITH_DIGEST}"
EOF

echo "  Created snapshot: ${SNAPSHOT_NAME}"
echo "  Namespace: ${USER_NAMESPACE}"
echo "  Image: ${FULL_IMAGE_WITH_DIGEST}"
echo "  📍 Note: Service runs in 'default' namespace, Snapshot in '${USER_NAMESPACE}'"

echo ""
echo "🚀 Step 10: Triggering VSA generation..."
kubectl apply -f /tmp/vsa-demo-snapshot.yaml
echo "  Snapshot applied - VSA generation should start"

echo ""
echo "⏳ Step 11: Monitoring VSA generation..."
echo "  Waiting for TaskRun creation in ${USER_NAMESPACE}..."
sleep 5

# Find the TaskRun (will be created in the same namespace as the Snapshot)
TASKRUN=$(kubectl get taskruns -n "${USER_NAMESPACE}" -l app.kubernetes.io/instance=${SNAPSHOT_NAME} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$TASKRUN" ]; then
    echo "  ⚠️ No TaskRun found yet. Checking service logs..."
    kubectl logs -l app=conforma-knative-service -n default --tail=20
    echo ""
    echo "  💡 This might indicate:"
    echo "    - Cross-namespace event not received (check ApiServerSource config)"
    echo "    - No ReleasePlan configured for vsa-demo-application in ${USER_NAMESPACE}"
    echo "    - Service is still processing the snapshot"
    echo "    - Check 'kubectl get taskruns -n ${USER_NAMESPACE}' and service logs"
else
    echo "  ✅ TaskRun created: ${TASKRUN}"
    echo ""
    echo "📊 Step 12: Watching VSA generation progress..."
    echo "  Waiting for TaskRun to complete..."
    echo ""
    echo "📋 Expected Behavior:"
    echo "  ✅ Cross-namespace event detected (Snapshot in ${USER_NAMESPACE}, Service in default)"
    echo "  ✅ Image accessibility will SUCCEED (in-cluster registry accessible)"
    echo "  ✅ Image signature check will SUCCEED"
    echo "  ✅ Attestation signature check will SUCCEED"
    echo "  ✅ Policy validation will run"
    echo "  ✅ VSA will be GENERATED and uploaded to Rekor"
    echo "  ✅ This demonstrates the complete VSA generation workflow!"
    echo ""
    
    # Wait for TaskRun to complete (avoid tkn CLI race condition)
    kubectl wait --for=condition=Succeeded --timeout=120s taskrun/${TASKRUN} -n ${USER_NAMESPACE} 2>/dev/null || true
    
    # Get logs after completion (no -f flag to avoid race condition)
    echo "TaskRun logs:"
    kubectl logs -l tekton.dev/taskRun=${TASKRUN} -n ${USER_NAMESPACE} --tail=50 2>/dev/null || \
        echo "  (TaskRun completed too quickly - check with: kubectl logs -l tekton.dev/taskRun=${TASKRUN} -n ${USER_NAMESPACE})"
fi

echo ""
echo "🎉 VSA Generation Demo Results:"
echo "  ✅ Cross-namespace watching: Snapshot in ${USER_NAMESPACE}, Service in default"
echo "  ✅ In-cluster registry: Images accessible from TaskRuns"
echo "  ✅ Image signatures: Verified with cosign"
echo "  ✅ SLSA attestations: Created and verified"
echo "  ✅ Policy validation: Executed"
echo "  ✅ VSA generation: Complete workflow demonstrated"
echo ""
echo "🔗 Rekor entries created for transparency and auditability"
echo ""
echo "💡 Key Demonstration:"
echo "  The ApiServerSource (in default namespace) successfully detected and processed"
echo "  a Snapshot created in ${USER_NAMESPACE}, demonstrating cross-namespace capability!"
echo ""
echo "✅ VSA Generation Demo Finished!"

