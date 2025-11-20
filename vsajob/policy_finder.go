// Copyright The Conforma Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package vsajob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultEcpName is the fallback EnterpriseContractPolicy name when a ReleasePlanAdmission
// doesn't explicitly specify a policy. This ensures VSA generation can proceed with a
// reasonable default policy for standard container registry compliance checks.
//
// TODO: Investigate if there's a way to discover this default dynamically from cluster
// configuration rather than hard-coding it.
const defaultEcpName = "registry-standard"

// ReleasePlanNotFoundError is returned when no ReleasePlan exists for a snapshot's application.
// This is not necessarily an error condition - it indicates the snapshot is not intended for
// release and doesn't need VSA generation.
var ReleasePlanNotFoundError = errors.New("no ReleasePlan found for snapshot")

// policyFinder discovers the appropriate EnterpriseContractPolicy for a snapshot
// by traversing the Konflux resource hierarchy: Snapshot → ReleasePlan → ReleasePlanAdmission → Policy.
type policyFinder struct {
	client controllerRuntimeClient
	logger logr.Logger
}

// newPolicyFinder creates a new policy finder instance.
func newPolicyFinder(client controllerRuntimeClient, logger logr.Logger) policyFinder {
	return policyFinder{
		client: client,
		logger: logger,
	}
}

// FindPolicy discovers the EnterpriseContractPolicy configuration for a snapshot.
//
// The policy discovery process:
//  1. Extract the application name from the snapshot's spec JSON
//  2. Find the ReleasePlan for that application in the snapshot's namespace
//  3. Extract the ReleasePlanAdmission reference from the ReleasePlan
//  4. Retrieve the ReleasePlanAdmission from its namespace
//  5. Extract the EnterpriseContractPolicy name from the RPA spec
//
// Returns:
//   - A policy reference string in "namespace/name" format (e.g., "rhtap-releng-tenant/registry-standard")
//   - An error if the discovery process fails at any step
//
// Note: If the ReleasePlanAdmission doesn't specify a policy, this falls back to the
// default policy name "registry-standard" in the RPA's namespace.
//
// Error cases:
//   - Snapshot spec is invalid JSON
//   - No ReleasePlan found for the application
//   - Multiple ReleasePlans found (uses first, logs warning)
//   - ReleasePlanAdmission not found
func (pf *policyFinder) FindPolicy(ctx context.Context, snapshot Snapshot) (string, error) {
	pf.logger.Info("Looking up EnterpriseContractPolicy", "snapshot", snapshot.Name, "namespace", snapshot.Namespace)

	// Extract the application name from the raw JSON spec
	var spec struct {
		Application string `json:"application"`
	}
	if err := json.Unmarshal(snapshot.Spec, &spec); err != nil {
		return "", fmt.Errorf("failed to unmarshal snapshot spec to extract application: %w", err)
	}

	// Find the applicable ReleasePlan for this application
	rp, err := pf.findReleasePlan(ctx, spec.Application, snapshot.Namespace)
	if err != nil {
		return "", err
	}
	pf.logger.Info("Found ReleasePlan", "name", rp.Name, "namespace", rp.Namespace)

	// Use the ReleasePlan to find the relevant ReleasePlanAdmission
	rpa, err := pf.findReleasePlanAdmission(ctx, rp)
	if err != nil {
		return "", err
	}
	pf.logger.Info("Found ReleasePlanAdmission", "name", rpa.Name, "namespace", rpa.Namespace)

	// Read the ECP name from the ReleasePlanAdmission
	ecpName := rpa.Spec.Policy

	// TODO: It is safe to assume the RPA and the ECP are always in the same namespace?
	ecpNamespace := rpa.Namespace

	// Fall back to the default value if the RPA doesn't set a policy
	var logMsg string
	if ecpName == "" {
		ecpName = defaultEcpName
		logMsg = "No policy specified in RPA, using default"
	} else {
		logMsg = "Using policy specified in RPA"
	}
	pf.logger.Info(logMsg, "name", ecpName, "namespace", ecpNamespace)

	policyConfig := fmt.Sprintf("%s/%s", ecpNamespace, ecpName)
	pf.logger.Info("Found policy configuration", "policy", policyConfig)
	// Example value: rhtap-releng-tenant/registry-rhtap-contract
	// Conforma can use this directly with its --policy flag
	return policyConfig, nil
}

// findReleasePlan searches for a ReleasePlan matching the given application name in the specified namespace.
//
// If multiple ReleasePlans exist for the same application (unusual but possible), this method:
//   - Logs a warning with details about all matching plans
//   - Returns the first matching plan found
//
// Returns:
//   - The matching ReleasePlan
//   - An error if no plans exist in the namespace or none match the application name
func (pf *policyFinder) findReleasePlan(ctx context.Context, appName string, ns string) (ReleasePlan, error) {
	var rp ReleasePlan

	// Get all release plans in the namespace
	planList := &ReleasePlanList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "ReleasePlanList",
		},
	}
	err := pf.client.List(ctx, planList, client.InNamespace(ns))
	if err != nil {
		return rp, fmt.Errorf("failed to lookup release plan in namespace %s: %w", ns, err)
	}
	if len(planList.Items) == 0 {
		return rp, ReleasePlanNotFoundError
	}

	// Filter to find just the release plans for the given application
	var matchingPlans []ReleasePlan
	for _, plan := range planList.Items {
		if plan.Spec.Application == appName {
			matchingPlans = append(matchingPlans, plan)
		}
	}
	if len(matchingPlans) == 0 {
		return rp, ReleasePlanNotFoundError
	}

	if len(matchingPlans) > 1 {
		// TODO: I'm expecting most of the time there will be only one releasePlan, but
		// I'm not sure how correct that is. Could there be more than one? If there was
		// more than one, how would we know which one to choose? For now we'll log a
		// warning with the details, and proceed with the first one found.
		for _, plan := range matchingPlans {
			rpa := fmt.Sprintf("%s/%s", plan.Spec.Target, plan.Labels["release.appstudio.openshift.io/releasePlanAdmission"])
			pf.logger.V(1).Info("Warning: Found multiple ReleasePlans", "RP", plan.Name, "Related RPA", rpa)
		}
	}
	rp = matchingPlans[0]

	return rp, nil
}

// findReleasePlanAdmission retrieves the ReleasePlanAdmission referenced by a ReleasePlan.
//
// The ReleasePlan contains two pieces of information to locate the RPA:
//   - spec.target: The namespace where the RPA exists
//   - labels["release.appstudio.openshift.io/releasePlanAdmission"]: The RPA name
//
// Returns:
//   - The referenced ReleasePlanAdmission
//   - An error if the RPA doesn't exist or cannot be retrieved
func (pf *policyFinder) findReleasePlanAdmission(ctx context.Context, rp ReleasePlan) (ReleasePlanAdmission, error) {
	var rpa ReleasePlanAdmission
	rpaKey := client.ObjectKey{
		Namespace: rp.rpaNamespace(),
		Name:      rp.rpaName(),
	}
	err := pf.client.Get(ctx, rpaKey, &rpa)
	if err != nil {
		return rpa, fmt.Errorf("failed to get release plan admission %s/%s: %w", rpaKey.Namespace, rpaKey.Name, err)
	}
	return rpa, nil
}
