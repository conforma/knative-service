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

package knative

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const knativeStateKey = key(0)

// KnativeState holds the state of Knative components
type KnativeState struct {
	servingInstalled  bool
	eventingInstalled bool
	serviceDeployed   bool
	serviceURL        string
}

// Key implements the testenv.State interface
func (k KnativeState) Key() any {
	return knativeStateKey
}

// installKnative installs Knative Serving and Eventing
func installKnative(ctx context.Context) (context.Context, error) {
	k := &KnativeState{}
	ctx, err := testenv.SetupState(ctx, &k)
	if err != nil {
		return ctx, err
	}

	if k.servingInstalled && k.eventingInstalled {
		return ctx, nil
	}

	// Get cluster state
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, allow nil cluster
		// TODO: Remove when real implementation is added
		k.servingInstalled = true
		k.eventingInstalled = true
		return ctx, nil
	}

	// Install Knative Serving
	if !k.servingInstalled {
		err = installKnativeServing(ctx, cluster)
		if err != nil {
			return ctx, fmt.Errorf("failed to install Knative Serving: %w", err)
		}
		k.servingInstalled = true
	}

	// Install Knative Eventing
	if !k.eventingInstalled {
		err = installKnativeEventing(ctx, cluster)
		if err != nil {
			return ctx, fmt.Errorf("failed to install Knative Eventing: %w", err)
		}
		k.eventingInstalled = true
	}

	return ctx, nil
}

// installKnativeServing installs Knative Serving components
func installKnativeServing(ctx context.Context, cluster *kubernetes.ClusterState) error {
	// Implementation would:
	// 1. Apply Knative Serving CRDs
	// 2. Apply Knative Serving core components
	// 3. Wait for components to be ready
	// 4. Configure networking (Kourier or Istio)

	return nil
}

// installKnativeEventing installs Knative Eventing components
func installKnativeEventing(ctx context.Context, cluster *kubernetes.ClusterState) error {
	// Implementation would:
	// 1. Apply Knative Eventing CRDs
	// 2. Apply Knative Eventing core components
	// 3. Wait for components to be ready
	// 4. Configure event sources and brokers

	return nil
}

// deployKnativeService deploys the knative service under test
func deployKnativeService(ctx context.Context) (context.Context, error) {
	k := testenv.FetchState[KnativeState](ctx)
	if k == nil {
		// For stub testing, initialize if not found
		// TODO: Remove when real implementation is added
		k = &KnativeState{
			servingInstalled:  true,
			eventingInstalled: true,
			serviceDeployed:   true,
		}
		var err error
		ctx, err = testenv.SetupState(ctx, &k)
		if err != nil {
			return ctx, err
		}
		return ctx, nil
	}

	if k.serviceDeployed {
		return ctx, nil
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, mark as deployed even without cluster
		// TODO: Remove when real implementation is added
		k.serviceDeployed = true
		return ctx, nil
	}

	// Deploy the knative service
	err := deployService(ctx, cluster)
	if err != nil {
		return ctx, fmt.Errorf("failed to deploy knative service: %w", err)
	}

	// Wait for service to be ready
	err = waitForServiceReady(ctx, cluster)
	if err != nil {
		return ctx, fmt.Errorf("knative service not ready: %w", err)
	}

	k.serviceDeployed = true
	return ctx, nil
}

// deployService deploys the knative service using ko or kubectl
func deployService(ctx context.Context, cluster *kubernetes.ClusterState) error {
	// Implementation would:
	// 1. Build the service image using ko
	// 2. Apply Knative Service manifest
	// 3. Apply ApiServerSource for Snapshot events
	// 4. Apply Trigger for event routing
	// 5. Configure RBAC permissions

	return nil
}

// waitForServiceReady waits for the knative service to be ready
func waitForServiceReady(ctx context.Context, cluster *kubernetes.ClusterState) error {
	return wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		// Check if service is ready
		// Implementation would check the Knative Service status
		return true, nil
	})
}

// checkServiceHealth verifies the service is responding to health checks
func checkServiceHealth(ctx context.Context) error {
	k := testenv.FetchState[KnativeState](ctx)
	if k == nil || !k.serviceDeployed {
		return fmt.Errorf("knative service not deployed")
	}

	// Implementation would make HTTP request to service health endpoint
	return nil
}

// AddStepsTo adds Knative-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^Knative is installed and configured$`, installKnative)
	sc.Step(`^the knative service is deployed$`, deployKnativeService)
	sc.Step(`^the knative service is healthy$`, checkServiceHealth)
}
