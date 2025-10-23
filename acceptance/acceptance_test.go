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

package acceptance

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/cucumber/godog"
	"github.com/gkampitakis/go-snaps/snaps"

	"github.com/conforma/knative-service/acceptance/knative"
	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/log"
	"github.com/conforma/knative-service/acceptance/snapshot"
	"github.com/conforma/knative-service/acceptance/tekton"
	"github.com/conforma/knative-service/acceptance/testenv"
	"github.com/conforma/knative-service/acceptance/vsa"
)

// NOTE: flags need to be initialized with the package in order to be recognized
// a flag that can be set by running the test with "-args -persist" command line options
var persist = flag.Bool("persist", false, "persist the stubbed environment to facilitate debugging")

// run acceptance tests with the persisted environment
var restore = flag.Bool("restore", false, "restore last persisted environment")

var noColors = flag.Bool("no-colors", false, "disable colored output")

// specify a subset of scenarios to run filtering by given tags
var tags = flag.String("tags", "", "select scenarios to run based on tags")

// random seed to use
var seed = flag.Int64("seed", -1, "random seed to use for the tests")

// initializeScenario adds all steps and registers all hooks to the
// provided godog.ScenarioContext
func initializeScenario(sc *godog.ScenarioContext) {
	knative.AddStepsTo(sc)
	kubernetes.AddStepsTo(sc)
	snapshot.AddStepsTo(sc)
	tekton.AddStepsTo(sc)
	vsa.AddStepsTo(sc)

	sc.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		logger, ctx := log.LoggerFor(ctx)
		logger.Name(sc.Name)

		return context.WithValue(ctx, testenv.Scenario, sc), nil
	})

	sc.After(func(ctx context.Context, scenario *godog.Scenario, scenarioErr error) (context.Context, error) {
		_, err := testenv.Persist(ctx)
		return ctx, err
	})
}

func initializeSuite(ctx context.Context) func(*godog.TestSuiteContext) {
	return func(tsc *godog.TestSuiteContext) {
		kubernetes.InitializeSuite(ctx, tsc)
	}
}

// setupContext creates a Context prepopulated with the *testing.T and *persist
// values
func setupContext(t *testing.T) context.Context {
	ctx := context.WithValue(context.Background(), testenv.TestingT, t)
	ctx = context.WithValue(ctx, testenv.PersistStubEnvironment, *persist)
	ctx = context.WithValue(ctx, testenv.RestoreStubEnvironment, *restore)
	ctx = context.WithValue(ctx, testenv.NoColors, *noColors)

	return ctx
}

// TestFeatures launches all acceptance test scenarios running them
// in random order in parallel threads equal to the number of available
// cores
func TestFeatures(t *testing.T) {
	// change the directory to repository root, makes for easier paths
	if err := os.Chdir(".."); err != nil {
		t.Error(err)
	}

	featuresDir, err := filepath.Abs("features")
	if err != nil {
		t.Error(err)
	}

	ctx := setupContext(t)

	// Determine concurrency level
	// Default to number of CPUs, but allow override via environment
	concurrency := runtime.NumCPU()
	if concEnv := os.Getenv("ACCEPTANCE_CONCURRENCY"); concEnv != "" {
		if c, err := strconv.Atoi(concEnv); err == nil && c > 0 {
			concurrency = c
		}
	}

	opts := godog.Options{
		Format:         "pretty",
		Paths:          []string{featuresDir},
		Randomize:      *seed,
		Concurrency:    concurrency,
		TestingT:       t,
		DefaultContext: ctx,
		Tags:           *tags,
		NoColors:       *noColors,
	}

	suite := godog.TestSuite{
		ScenarioInitializer:  initializeScenario,
		TestSuiteInitializer: initializeSuite(ctx),
		Options:              &opts,
	}

	if suite.Run() != 0 {
		t.Fatal("failure in acceptance tests")
	}
}

func TestMain(t *testing.M) {
	v := t.Run()

	// After all tests have run `go-snaps` can check for not used snapshots
	snaps.Clean(t)

	os.Exit(v)
}
