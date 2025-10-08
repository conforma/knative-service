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

# Generates the registry deployment, service with dynamic port from environment

set -o errexit
set -o pipefail
set -o nounset

PORT="${REGISTRY_PORT:-5000}"

printf -- '---
apiVersion: v1
kind: ConfigMap
metadata:
  name: registry-port-number
  namespace: image-registry
data:
  PORT: "%d"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: registry
  namespace: image-registry
  labels:
    app: registry
spec:
  replicas: 1
  selector:
    matchLabels:
      app: registry
  template:
    metadata:
      labels:
        app: registry
    spec:
      containers:
        - name: registry
          image: registry:2
          ports:
            - name: registry
              containerPort: %d
          env:
            - name: REGISTRY_STORAGE_DELETE_ENABLED
              value: "true"
            - name: REGISTRY_HTTP_ADDR
              value: ":%d"
          volumeMounts:
            - name: registry-storage
              mountPath: /var/lib/registry
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "100m"
      volumes:
        - name: registry-storage
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: registry
  name: registry
  namespace: image-registry
spec:
  ports:
  - name: registry
    nodePort: %d
    port: %d
    protocol: TCP
    targetPort: %d
  selector:
    app: registry
  type: NodePort
' "${PORT}" "${PORT}" "${PORT}" "${PORT}" "${PORT}" "${PORT}"
