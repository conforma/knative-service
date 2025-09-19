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

# This is a convenience wrapper for running hack/test_ecp_lookup.go to
# test/debug RPA/ECP lookups in a real cluster.

# You need to be logged in to our Konflux cluster. If not then go here
# to get an `oc login` command with a token then run it:
#   https://oauth-openshift.apps.stone-prd-rh01.pg1f.p1.openshiftapps.com/oauth/token/request

# Find a recent snapshot that starts with "ec-v".
# We're dog-fooding our own Conforma cli Konflux builds here, and I know the
# ec-v0{5,6,7} applications definitely have an RPA.
NAMESPACE=rhtap-contract-tenant
SNAPSHOT=$(oc get snapshot -n $NAMESPACE \
  --sort-by=.metadata.creationTimestamp --no-headers \
  --output=custom-columns="NAME:.metadata.name" |
  (grep ^ec-v || true) |
  tail -1)

if [ -z "$SNAPSHOT" ]; then
  echo "Snapshot not found. Maybe you need to login here:"
  echo "  https://oauth-openshift.apps.stone-prd-rh01.pg1f.p1.openshiftapps.com/oauth/token/request"
  exit 1
else
  # Use the test go program to look up the ECP for that snapshot
  go run hack/test_ecp_lookup.go $SNAPSHOT $NAMESPACE
fi
