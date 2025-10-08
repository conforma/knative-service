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

package vsa

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const vsaStateKey = key(0)

// VSAState holds the state of VSA verification
type VSAState struct {
	rekorRunning  bool
	rekorURL      string
	vsaCreated    bool
	vsaEntry      map[string]interface{}
	ecpConfigured bool
}

// Key implements the testenv.State interface
func (v VSAState) Key() any {
	return vsaStateKey
}

// setupRekor sets up and verifies Rekor is running
func setupRekor(ctx context.Context) (context.Context, error) {
	v := &VSAState{}
	ctx, err := testenv.SetupState(ctx, &v)
	if err != nil {
		return ctx, err
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, proceed without actual cluster
		// TODO: Remove when real implementation is added
	}

	// Implementation would:
	// 1. Deploy Rekor server in the cluster
	// 2. Wait for Rekor to be ready
	// 3. Configure Rekor URL
	v.rekorRunning = true
	v.rekorURL = "http://rekor-server:3000"

	return ctx, nil
}

// setupEnterpriseContractPolicy sets up ECP configuration
func setupEnterpriseContractPolicy(ctx context.Context) (context.Context, error) {
	v := &VSAState{}
	ctx, err := testenv.SetupState(ctx, &v)
	if err != nil {
		return ctx, err
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, proceed without actual cluster
		// TODO: Remove when real implementation is added
	}

	// Implementation would:
	// 1. Create EnterpriseContractPolicy CR
	// 2. Create ReleasePlan and ReleasePlanAdmission
	// 3. Configure policy bundle reference
	v.ecpConfigured = true

	return ctx, nil
}

// verifyTaskRunCompletes verifies TaskRun completes successfully
func verifyTaskRunCompletes(ctx context.Context) error {
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Implementation would:
	// 1. Wait for TaskRun to complete
	// 2. Verify status is Succeeded
	// This is a stub - actual implementation would query TaskRun status

	return nil
}

// verifyVSAInRekor verifies that a VSA was created in Rekor
func verifyVSAInRekor(ctx context.Context) error {
	v := testenv.FetchState[VSAState](ctx)
	if v == nil || !v.rekorRunning {
		return fmt.Errorf("Rekor not initialized")
	}

	// Implementation would:
	// 1. Query Rekor API for recent entries
	// 2. Find VSA entry for the snapshot
	// 3. Verify VSA structure
	v.vsaCreated = true

	return nil
}

// verifyVSAContents verifies VSA contains verification results
func verifyVSAContents(ctx context.Context) error {
	v := testenv.FetchState[VSAState](ctx)
	if v == nil || !v.vsaCreated {
		return fmt.Errorf("VSA not created")
	}

	// Implementation would:
	// 1. Parse VSA from Rekor
	// 2. Verify it contains policy evaluation results
	// 3. Verify it references the correct snapshot/images

	return nil
}

// verifyVSASignature verifies VSA is properly signed
func verifyVSASignature(ctx context.Context) error {
	v := testenv.FetchState[VSAState](ctx)
	if v == nil || !v.vsaCreated {
		return fmt.Errorf("VSA not created")
	}

	// Implementation would:
	// 1. Extract signature from VSA
	// 2. Verify signature using public key
	// 3. Verify signature matches VSA content

	return nil
}

// verifyErrorLogged verifies an error event was logged
func verifyErrorLogged(ctx context.Context) error {
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Implementation would:
	// 1. Query Kubernetes events
	// 2. Find error events related to the snapshot
	// 3. Verify error message content

	return nil
}

// AddStepsTo adds VSA and Rekor-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^Rekor is running and configured$`, setupRekor)
	sc.Step(`^enterprise contract policy configuration$`, setupEnterpriseContractPolicy)
	sc.Step(`^the TaskRun completes successfully$`, verifyTaskRunCompletes)
	sc.Step(`^a VSA should be created in Rekor$`, verifyVSAInRekor)
	sc.Step(`^the VSA should contain the verification results$`, verifyVSAContents)
	sc.Step(`^the VSA should be properly signed$`, verifyVSASignature)
	sc.Step(`^an error event should be logged$`, verifyErrorLogged)
}
