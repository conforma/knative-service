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

echo "* Installing demo CRDs (ReleasePlan, ReleasePlanAdmission)..."
kubectl apply -f hack/demo-crds.yaml

echo "* Waiting for CRDs to be ready..."
kubectl wait --for condition=established --timeout=60s crd/releaseplans.appstudio.redhat.com
kubectl wait --for condition=established --timeout=60s crd/releaseplanadmissions.appstudio.redhat.com
kubectl wait --for condition=established --timeout=60s crd/enterprisecontractpolicies.appstudio.redhat.com

echo "* Setting up demo resources (ReleasePlan, ReleasePlanAdmission)..."
kubectl apply -f hack/demo-resources.yaml

echo "* Waiting for resources to be ready..."
sleep 2

echo "* Deleting old snapshot..."
kubectl delete --ignore-not-found -f test-snapshot.yaml

echo "* Creating new snapshot..."
kubectl create -f test-snapshot.yaml

echo "* Waiting for pod"
until kubectl get pod -l app=conforma-knative-service -n default &>/dev/null; do sleep 1; done
kubectl wait --for=condition=Ready pod -l app=conforma-knative-service -n default --timeout=30s

echo "* Showing pod logs"
sleep 2
kubectl logs -l app=conforma-knative-service -n default --tail 100

echo "* Checking for created resources..."
echo "ReleasePlans:"
kubectl get releaseplan -n default
echo ""
echo "ReleasePlanAdmissions:"
kubectl get releaseplanadmission -n rhtap-releng-tenant
echo ""
echo "EnterpriseContractPolicies:"
kubectl get enterprisecontractpolicy -n rhtap-releng-tenant
echo ""

echo "* Find the new taskrun..."
sleep 2
TASKRUN=$(tkn tr list -o yaml | yq '.items[0].metadata.name')
echo "TaskRun name: $TASKRUN"

if [ "$TASKRUN" = "null" ] || [ -z "$TASKRUN" ]; then
    echo "No TaskRuns found. This might be expected if:"
    echo "  - No ReleasePlan exists in the cluster"
    echo "  - The snapshot doesn't meet the criteria for TaskRun creation"
    echo "  - The service is configured to skip TaskRun creation"
    echo ""
    echo "Check the service logs above for more details."
    echo ""
    echo "Current TaskRuns in all namespaces:"
    kubectl get taskruns --all-namespaces
    exit 0
fi

# Watch the logs of that taskrun
echo "* Watch the taskrun logs (ctrl-c to quit)"
tkn taskrun logs -f $TASKRUN
