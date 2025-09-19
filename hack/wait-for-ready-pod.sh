#!/usr/bin/env bash
# Copyright The Conforma Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

LABEL="$1"
NAMESPACE="$2"

# Wait for pod to be created
while ! kubectl get pod -l "$LABEL" -n "$NAMESPACE" --no-headers 2>/dev/null | grep -q .; do
  echo -n "."
  sleep 2
done

# Wait for pod to be ready
kubectl wait --for=condition=ready pod -l "$LABEL" -n "$NAMESPACE" --timeout 300s
