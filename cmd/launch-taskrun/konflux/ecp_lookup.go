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

package konflux

import (
	"context"
	"encoding/json"
	"fmt"

	gozap "go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientReader interface captures only the read operations we need for testability
type ClientReader interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

// Logger interface for logging in the konflux package
type Logger interface {
	Info(msg string, fields ...gozap.Field)
	Warn(msg string, fields ...gozap.Field)
	Error(err error, msg string, fields ...gozap.Field)
}

// findReleasePlan looks for a release plan applicable for a given application
func FindReleasePlan(ctx context.Context, cli ClientReader, logger Logger, appName string, ns string) (ReleasePlan, error) {
	var rp ReleasePlan

	// Get all release plans in the namespace
	planList := &ReleasePlanList{}
	err := cli.List(ctx, planList, client.InNamespace(ns))
	if err != nil {
		return rp, fmt.Errorf("failed to lookup release plan in namespace %s: %w", ns, err)
	}
	if len(planList.Items) == 0 {
		return rp, fmt.Errorf("no release plans found in namespace %s", ns)
	}

	// Filter to find just the release plans for the given application
	var matchingPlans []ReleasePlan
	for _, plan := range planList.Items {
		if plan.Spec.Application == appName {
			matchingPlans = append(matchingPlans, plan)
		}
	}
	if len(matchingPlans) == 0 {
		return rp, fmt.Errorf("no release plans found for application name: %s", appName)
	}

	if len(matchingPlans) > 1 {
		// TODO: I'm expecting most of the time there will be only one ReleasePlan, but
		// I'm not sure how correct that is. Could there be more than one? If there was
		// more than one, how would we know which one to choose? For now we'll log a
		// warning with the details, and proceed with the first one found.
		for _, plan := range matchingPlans {
			rpa := fmt.Sprintf("%s/%s", plan.Spec.Target, plan.Labels["release.appstudio.openshift.io/releasePlanAdmission"])
			logger.Warn("Found multiple ReleasePlans", gozap.String("RP", plan.Name), gozap.String("Related RPA", rpa))
		}
	}
	rp = matchingPlans[0]

	return rp, nil
}

// Two methods to extract the information we need from the ReleasePlan
func (rp *ReleasePlan) RpaNamespace() string {
	// Usually "rhtap-releng-tenant"
	return rp.Spec.Target
}

func (rp *ReleasePlan) RpaName() string {
	return rp.Labels["release.appstudio.openshift.io/releasePlanAdmission"]
}

func FindReleasePlanAdmission(ctx context.Context, cli ClientReader, logger Logger, rp ReleasePlan) (ReleasePlanAdmission, error) {
	// The RP points to a one specific RPA. Look it up using the target and a label value:
	var rpa ReleasePlanAdmission
	rpaKey := client.ObjectKey{
		Namespace: rp.RpaNamespace(),
		Name:      rp.RpaName(),
	}
	err := cli.Get(ctx, rpaKey, &rpa)
	if err != nil {
		return rpa, fmt.Errorf("failed to get release plan admission %s/%s: %w", rpaKey.Namespace, rpaKey.Name, err)
	}
	return rpa, nil
}

// FindECP takes a snapshot and tries to find the ECP that would be applicable in the
// Konflux release pipeline if that snapshot was released by looking up the relevant RPA
func FindEnterpriseContractPolicy(ctx context.Context, cli ClientReader, logger Logger, snapshot *Snapshot) (string, error) {
	// TODO: There might be a way to look this up which would be preferable to hard-coding it here
	const defaultEcpName = "registry-standard"

	// Extract the application name from the raw JSON spec
	var spec struct {
		Application string `json:"application"`
	}
	if err := json.Unmarshal(snapshot.Spec, &spec); err != nil {
		return "", fmt.Errorf("failed to unmarshal snapshot spec to extract application: %w", err)
	}

	appName := spec.Application
	ns := snapshot.Namespace

	// Find the applicable ReleasePlan for this application
	rp, err := FindReleasePlan(ctx, cli, logger, appName, ns)
	if err != nil {
		return "", err
	}
	logger.Info("Found ReleasePlan", gozap.String("name", rp.Name), gozap.String("namespace", rp.Namespace))

	// Use the ReleasePlan to find the relevant ReleasePlanAdmission
	rpa, err := FindReleasePlanAdmission(ctx, cli, logger, rp)
	if err != nil {
		return "", err
	}
	logger.Info("Found ReleasePlanAdmission", gozap.String("name", rpa.Name), gozap.String("namespace", rpa.Namespace))

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

	logger.Info(logMsg, gozap.String("name", ecpName), gozap.String("namespace", ecpNamespace))

	// Example value: rhtap-releng-tenant/registry-rhtap-contract
	// Conforma can use this directly with its --policy flag
	return fmt.Sprintf("%s/%s", ecpNamespace, ecpName), nil
}
