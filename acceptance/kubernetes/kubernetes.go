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

package kubernetes

import (
	"context"
	"errors"

	"github.com/cucumber/godog"

	"github.com/conforma/knative-service/acceptance/kubernetes/kind"
	"github.com/conforma/knative-service/acceptance/kubernetes/types"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const (
	clusterStateKey = key(0) // we store the ClusterState struct under this key in Context and when persisted
	stopStateKey    = key(iota)
)

// ClusterState holds the Cluster used in the current Context
type ClusterState struct {
	cluster types.Cluster
}

func (c ClusterState) Key() any {
	return clusterStateKey
}

func (c ClusterState) Persist() bool {
	return false
}

func (c ClusterState) Up(ctx context.Context) bool {
	// if the cluster implementation has been initialized and it claims the
	// cluster to be up
	return c.cluster != nil && c.cluster.Up(ctx)
}

func (c ClusterState) KubeConfig(ctx context.Context) (string, error) {
	if err := mustBeUp(ctx, c); err != nil {
		return "", err
	}

	return c.cluster.KubeConfig(ctx)
}

type startFunc func(context.Context) (context.Context, types.Cluster, error)

// startAndSetupState starts the cluster via the provided startFunc. The
// crosscutting concern of setting up the ClusterState in the Context and making
// sure we don't start the cluster multiple times per Context is handled here
func startAndSetupState(start startFunc) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		c := &ClusterState{}
		ctx, err := testenv.SetupState(ctx, &c)
		if err != nil {
			return ctx, err
		}

		if c.Up(ctx) {
			return ctx, nil
		}

		ctx, c.cluster, err = start(ctx)

		return ctx, err
	}
}

// mustBeUp makes sure that the cluster is up, used by functions that require
// the cluster to be up
func mustBeUp(ctx context.Context, c ClusterState) error {
	if !c.Up(ctx) {
		return errors.New("cluster has not been started, use `Given a cluster running`")
	}

	return nil
}

func createNamespace(ctx context.Context) (context.Context, error) {
	c := testenv.FetchState[ClusterState](ctx)

	if err := mustBeUp(ctx, *c); err != nil {
		return ctx, err
	}

	return c.cluster.CreateNamespace(ctx)
}

// AddStepsTo adds cluster-related steps to the context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^a cluster running$`, startAndSetupState(kind.Start))
	sc.Step(`^a working namespace$`, createNamespace)

	// stop usage of the cluster once a test is done, godog will call this
	// function on failure and on the last step, so more than once if the
	// failure is not on the last step and once if there was no failure or the
	// failure was on the last step
	sc.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if ctx.Value(stopStateKey) == nil {
			ctx = context.WithValue(ctx, stopStateKey, true)
		} else {
			// we did this already
			return ctx, nil
		}

		if !testenv.HasState[ClusterState](ctx) {
			return ctx, nil
		}

		c := testenv.FetchState[ClusterState](ctx)
		if c == nil {
			return ctx, nil
		}

		if c.cluster == nil || !c.cluster.Up(ctx) {
			return ctx, nil
		}

		return c.cluster.Stop(ctx)
	})
}

func InitializeSuite(ctx context.Context, tsc *godog.TestSuiteContext) {
	tsc.AfterSuite(func() {
		if !testenv.Persisted(ctx) {
			kind.Destroy(ctx)
		}
	})
}
