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

// ---------------------------------------------------------------------------
//
// This is meant to be a thin wrapper for the FindECP function in
// cmd/launch-taskrun/konflux/ecp_lookup so you can run it directly.
// It maybe doesn't have a lot of long term value, and it has no test
// coverage, but it's useful when developing and debugging the FindECP
// function from cmd/launch-taskrun/konflux/ecp_lookup.go.
//
// Example usage:
//   $ go run hack/ecp_lookup.go ec-v07-psx6j rhtap-contract-tenant
//
// Or use this bash script:
//   $ ./hack/test_ecp_lookup.sh
//
// ---------------------------------------------------------------------------

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	gozap "go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/conforma/knative-service/cmd/launch-taskrun/k8s"
	"github.com/conforma/knative-service/cmd/launch-taskrun/konflux"
)

// zapLogger wraps a zap logger to implement our Logger interface
type zapLogger struct {
	l *gozap.Logger
}

func (z *zapLogger) Info(msg string, fields ...gozap.Field) { z.l.Info(msg, fields...) }
func (z *zapLogger) Warn(msg string, fields ...gozap.Field) { z.l.Warn(msg, fields...) }
func (z *zapLogger) Error(err error, msg string, fields ...gozap.Field) {
	z.l.Error(msg, append(fields, gozap.Error(err))...)
}

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: go run test_ecp_lookup.go <snapshot-name> <namespace>")
	}

	snapshotName := os.Args[1]
	namespace := os.Args[2]

	// Create client
	cli, err := k8s.NewControllerRuntimeClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get the snapshot
	snapshot := &konflux.Snapshot{}
	err = cli.Get(context.Background(), client.ObjectKey{
		Name:      snapshotName,
		Namespace: namespace,
	}, snapshot)
	if err != nil {
		log.Fatalf("Failed to get snapshot %s/%s: %v", namespace, snapshotName, err)
	}

	fmt.Printf("Found snapshot: %s\n", snapshot.Name)

	// Extract application name from raw JSON spec
	var spec struct {
		Application string `json:"application"`
	}
	if err = json.Unmarshal(snapshot.Spec, &spec); err != nil {
		log.Fatalf("Failed to extract application from spec: %v", err)
	}
	fmt.Printf("Application name: %s\n", spec.Application)

	// Create a development logger for nice console output
	zapLog, err := gozap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		_ = zapLog.Sync() // Ignore sync errors (common with stderr/stdout)
	}()
	logger := &zapLogger{l: zapLog}

	// Call FindEnterpriseContractPolicy
	policyResult, err := konflux.FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)
	if err != nil {
		log.Fatalf("Failed to get enterprise contract policy: %v", err)
	}

	fmt.Printf("Found ECP name: %s\n", policyResult)
}
