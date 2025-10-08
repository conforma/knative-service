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

// Package registry provides registry state management for acceptance tests
package registry

import (
	"context"
	"fmt"

	"github.com/conforma/knative-service/acceptance/testenv"
	"github.com/pkg/errors"
)

type key int

// key to store the host:port of the registry in Context and persisted environment
const registryStateKey = key(0)

type registryState struct {
	HostAndPort string
}

func (g registryState) Key() any {
	return registryStateKey
}

func (g registryState) Up() bool {
	return g.HostAndPort != ""
}

// IsRunning returns true if a registry has been registered in the context
func IsRunning(ctx context.Context) bool {
	if !testenv.HasState[registryState](ctx) {
		return false
	}

	state := testenv.FetchState[registryState](ctx)
	return state.Up()
}

// Url returns the host:port needed to interact with the registry
func Url(ctx context.Context) (string, error) {
	if !testenv.HasState[registryState](ctx) {
		return "", errors.New("no state setup, did you start the registry?")
	}

	state := testenv.FetchState[registryState](ctx)
	if !state.Up() {
		return "", errors.New("registry not running")
	}
	return state.HostAndPort, nil
}

// Register registers a registry host:port in the context
func Register(ctx context.Context, hostAndPort string) (context.Context, error) {
	var state *registryState
	ctx, err := testenv.SetupState(ctx, &state)
	if err != nil {
		return ctx, err
	}

	if state.Up() {
		return ctx, fmt.Errorf("a registry has already been registered in this context: %s", state.HostAndPort)
	}

	state.HostAndPort = hostAndPort

	return ctx, nil
}
