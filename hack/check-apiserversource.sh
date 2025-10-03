#!/bin/bash
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


# Script to check if ApiServerSource is available in a Kubernetes cluster
# Usage: ./check-apiserversource.sh

set -e

echo "🔍 Checking ApiServerSource availability in cluster..."
echo

# Method 1: Check CRD exists
echo "1. Checking for ApiServerSource CRD..."
if kubectl get crd apiserversources.sources.knative.dev >/dev/null 2>&1; then
    echo "   ✅ ApiServerSource CRD found"
    kubectl get crd apiserversources.sources.knative.dev --no-headers | awk '{print "   📅 Created:", $2}'
else
    echo "   ❌ ApiServerSource CRD not found"
    exit 1
fi
echo

# Method 2: Check API resource
echo "2. Checking API resource availability..."
if kubectl api-resources | grep -q apiserversource; then
    echo "   ✅ ApiServerSource API resource available"
    kubectl api-resources | grep apiserversource | awk '{print "   📋 Resource:", $1, "Group:", $3}'
else
    echo "   ❌ ApiServerSource API resource not available"
    exit 1
fi
echo

# Method 3: Check Knative Eventing namespace
echo "3. Checking Knative Eventing installation..."
if kubectl get namespace knative-eventing >/dev/null 2>&1; then
    echo "   ✅ knative-eventing namespace found"
    
    # Check eventing pods
    echo "   📊 Knative Eventing pods:"
    kubectl get pods -n knative-eventing --no-headers | while read line; do
        name=$(echo $line | awk '{print $1}')
        status=$(echo $line | awk '{print $3}')
        if [ "$status" = "Running" ]; then
            echo "      ✅ $name"
        else
            echo "      ⚠️  $name ($status)"
        fi
    done
else
    echo "   ❌ knative-eventing namespace not found"
    exit 1
fi
echo

# Method 4: Test schema access
echo "4. Testing ApiServerSource schema access..."
if kubectl explain apiserversource >/dev/null 2>&1; then
    echo "   ✅ Can access ApiServerSource schema"
    version=$(kubectl explain apiserversource | grep "VERSION:" | awk '{print $2}')
    echo "   📋 Version: $version"
else
    echo "   ❌ Cannot access ApiServerSource schema"
    exit 1
fi
echo

# Method 5: Test dry-run creation
echo "5. Testing dry-run creation..."
cat <<EOF | kubectl apply --dry-run=client -f - >/dev/null 2>&1
apiVersion: sources.knative.dev/v1
kind: ApiServerSource
metadata:
  name: test-apiserversource
spec:
  serviceAccountName: default
  mode: Resource
  resources:
    - apiVersion: v1
      kind: Pod
  sink:
    ref:
      apiVersion: v1
      kind: Service
      name: test-service
EOF

if [ $? -eq 0 ]; then
    echo "   ✅ Can create ApiServerSource (dry-run successful)"
else
    echo "   ❌ Cannot create ApiServerSource (dry-run failed)"
    exit 1
fi
echo

# Method 6: List existing ApiServerSources
echo "6. Listing existing ApiServerSources..."
if kubectl get apiserversources --all-namespaces --no-headers 2>/dev/null | wc -l | grep -q "^0$"; then
    echo "   📋 No existing ApiServerSources found"
else
    echo "   📋 Existing ApiServerSources:"
    kubectl get apiserversources --all-namespaces --no-headers | while read line; do
        namespace=$(echo $line | awk '{print $1}')
        name=$(echo $line | awk '{print $2}')
        ready=$(echo $line | awk '{print $5}')
        echo "      📌 $namespace/$name (Ready: $ready)"
    done
fi
echo

echo "🎉 ApiServerSource is fully available and functional in this cluster!"
echo
echo "💡 You can now use ApiServerSource to:"
echo "   • Monitor Kubernetes resource changes"
echo "   • Convert API events to CloudEvents"
echo "   • Trigger event-driven workflows"
echo
echo "📖 Example usage:"
echo "   kubectl apply -f config/base/event-source.yaml"
