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

package testenv

import (
	"context"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

// Context keys for test environment state
type contextKey string

const (
	TestingT                 contextKey = "testing.T"
	PersistStubEnvironment   contextKey = "persist"
	RestoreStubEnvironment   contextKey = "restore"
	NoColors                 contextKey = "no-colors"
	Scenario                 contextKey = "scenario"
)

// State represents the interface for test state management
type State interface {
	Persist() bool
}

// SetupState initializes state in the context
func SetupState[T State](ctx context.Context, state *T) (context.Context, error) {
	existing := FetchState[T](ctx)
	if existing != nil {
		*state = *existing
		return ctx, nil
	}

	return context.WithValue(ctx, fmt.Sprintf("state-%T", *state), state), nil
}

// FetchState retrieves state from the context
func FetchState[T State](ctx context.Context) *T {
	if state, ok := ctx.Value(fmt.Sprintf("state-%T", *new(T))).(*T); ok {
		return state
	}
	return nil
}

// Persist handles test environment persistence for debugging
func Persist(ctx context.Context) (context.Context, error) {
	if !ShouldPersist(ctx) {
		return ctx, nil
	}

	// Implementation would handle persisting test environment
	// This is a placeholder for the actual persistence logic
	return ctx, nil
}

// ShouldPersist checks if the test environment should be persisted
func ShouldPersist(ctx context.Context) bool {
	if persist, ok := ctx.Value(PersistStubEnvironment).(bool); ok {
		return persist
	}
	return false
}

// ShouldRestore checks if the test environment should be restored
func ShouldRestore(ctx context.Context) bool {
	if restore, ok := ctx.Value(RestoreStubEnvironment).(bool); ok {
		return restore
	}
	return false
}

// GetTestingT retrieves the testing.T instance from context
func GetTestingT(ctx context.Context) *testing.T {
	if t, ok := ctx.Value(TestingT).(*testing.T); ok {
		return t
	}
	return nil
}

// GetScenario retrieves the current scenario from context
func GetScenario(ctx context.Context) *godog.Scenario {
	if scenario, ok := ctx.Value(Scenario).(*godog.Scenario); ok {
		return scenario
	}
	return nil
}

