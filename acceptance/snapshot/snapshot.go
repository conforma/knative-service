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

package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const snapshotStateKey = key(0)

// SnapshotState holds the state of snapshot resources
type SnapshotState struct {
	Snapshots     map[string]*unstructured.Unstructured
	Namespace     string
	InvalidExists bool // tracks if any invalid snapshots were created
}

// Key implements the testenv.State interface
func (s SnapshotState) Key() any {
	return snapshotStateKey
}

// Snapshot represents the structure of a Snapshot resource
type Snapshot struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
	} `json:"metadata"`
	Spec SnapshotSpec `json:"spec"`
}

// SnapshotSpec represents the spec of a Snapshot resource
type SnapshotSpec struct {
	Application        string      `json:"application"`
	DisplayName        string      `json:"displayName"`
	DisplayDescription string      `json:"displayDescription,omitempty"`
	Components         []Component `json:"components"`
}

// Component represents a component in a snapshot
type Component struct {
	Name           string `json:"name"`
	ContainerImage string `json:"containerImage"`
}

// createValidSnapshot creates a valid snapshot from specification
func createValidSnapshot(ctx context.Context, specification *godog.DocString) (context.Context, error) {
	s := &SnapshotState{}
	ctx, err := testenv.SetupState(ctx, &s)
	if err != nil {
		return ctx, err
	}

	// Initialize map if not already done
	if s.Snapshots == nil {
		s.Snapshots = make(map[string]*unstructured.Unstructured)
	}

	// Use default namespace for now
	// In a real implementation, this would come from the cluster's working namespace
	s.Namespace = "default"

	// Parse the specification
	var spec SnapshotSpec
	err = json.Unmarshal([]byte(specification.Content), &spec)
	if err != nil {
		return ctx, fmt.Errorf("failed to parse snapshot specification: %w", err)
	}

	// Create the snapshot resource
	snapshot := &Snapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Spec:       spec,
	}

	// Generate unique name
	snapshot.Metadata.Name = fmt.Sprintf("test-snapshot-%d", time.Now().Unix())
	snapshot.Metadata.Namespace = s.Namespace

	// Convert to unstructured for Kubernetes API
	unstructuredSnapshot, err := toUnstructured(snapshot)
	if err != nil {
		return ctx, fmt.Errorf("failed to convert snapshot to unstructured: %w", err)
	}

	s.Snapshots[snapshot.Metadata.Name] = unstructuredSnapshot

	return ctx, nil
}

// createInvalidSnapshot creates an invalid snapshot from specification
func createInvalidSnapshot(ctx context.Context, specification *godog.DocString) (context.Context, error) {
	s := &SnapshotState{}
	ctx, err := testenv.SetupState(ctx, &s)
	if err != nil {
		return ctx, err
	}

	// Initialize map if not already done
	if s.Snapshots == nil {
		s.Snapshots = make(map[string]*unstructured.Unstructured)
	}

	// Use default namespace for now
	// In a real implementation, this would come from the cluster's working namespace
	s.Namespace = "default"

	// Parse the specification (which should be invalid)
	var spec SnapshotSpec
	err = json.Unmarshal([]byte(specification.Content), &spec)
	if err != nil {
		return ctx, fmt.Errorf("failed to parse snapshot specification: %w", err)
	}

	// Create the invalid snapshot resource
	snapshot := &Snapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Spec:       spec,
	}

	// Generate unique name
	snapshot.Metadata.Name = fmt.Sprintf("invalid-snapshot-%d", time.Now().Unix())
	snapshot.Metadata.Namespace = s.Namespace

	// Convert to unstructured for Kubernetes API
	unstructuredSnapshot, err := toUnstructured(snapshot)
	if err != nil {
		return ctx, fmt.Errorf("failed to convert snapshot to unstructured: %w", err)
	}

	s.Snapshots[snapshot.Metadata.Name] = unstructuredSnapshot
	s.InvalidExists = true // mark that an invalid snapshot exists

	return ctx, nil
}

// createSnapshotInCluster creates the snapshot resource in the cluster
func createSnapshotInCluster(ctx context.Context) (context.Context, error) {
	s := testenv.FetchState[SnapshotState](ctx)
	if s == nil {
		return ctx, fmt.Errorf("no snapshots to create")
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, proceed without actual cluster
		// TODO: Remove when real implementation is added
		return ctx, nil
	}

	// Create each snapshot in the cluster
	for name, snapshot := range s.Snapshots {
		err := createSnapshotResource(ctx, cluster, snapshot)
		if err != nil {
			return ctx, fmt.Errorf("failed to create snapshot %s: %w", name, err)
		}
	}

	return ctx, nil
}

// createSnapshotResource creates a snapshot resource in Kubernetes
func createSnapshotResource(ctx context.Context, cluster *kubernetes.ClusterState, snapshot *unstructured.Unstructured) error {
	// Implementation would use dynamic client to create the snapshot resource
	// This is a placeholder for the actual Kubernetes API call
	return nil
}

// createMultipleSnapshots creates multiple snapshots simultaneously
func createMultipleSnapshots(ctx context.Context, count int) (context.Context, error) {
	s := &SnapshotState{}
	ctx, err := testenv.SetupState(ctx, &s)
	if err != nil {
		return ctx, err
	}

	// Initialize map if not already done
	if s.Snapshots == nil {
		s.Snapshots = make(map[string]*unstructured.Unstructured)
	}

	// Use default namespace for now
	// In a real implementation, this would come from the cluster's working namespace
	s.Namespace = "default"

	// Create multiple snapshots
	for i := 0; i < count; i++ {
		spec := SnapshotSpec{
			Application:        fmt.Sprintf("test-app-%d", i),
			DisplayName:        fmt.Sprintf("test-snapshot-%d", i),
			DisplayDescription: fmt.Sprintf("Test snapshot %d for performance testing", i),
			Components: []Component{
				{
					Name:           fmt.Sprintf("component-%d", i),
					ContainerImage: "quay.io/redhat-user-workloads/test/component@sha256:abc123",
				},
			},
		}

		snapshot := &Snapshot{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Snapshot",
			Spec:       spec,
		}

		snapshot.Metadata.Name = fmt.Sprintf("perf-test-snapshot-%d-%d", i, time.Now().Unix())
		snapshot.Metadata.Namespace = s.Namespace

		unstructuredSnapshot, err := toUnstructured(snapshot)
		if err != nil {
			return ctx, fmt.Errorf("failed to convert snapshot %d to unstructured: %w", i, err)
		}

		s.Snapshots[snapshot.Metadata.Name] = unstructuredSnapshot
	}

	return ctx, nil
}

// toUnstructured converts a typed object to unstructured
func toUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var unstructuredObj unstructured.Unstructured
	err = json.Unmarshal(data, &unstructuredObj)
	if err != nil {
		return nil, err
	}

	// Set GVK
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "Snapshot",
	})

	return &unstructuredObj, nil
}

// createSimpleValidSnapshot creates a valid snapshot without docstring specification
func createSimpleValidSnapshot(ctx context.Context) (context.Context, error) {
	// Create a default valid snapshot specification
	defaultSpec := `{
		"application": "default-app",
		"displayName": "default-snapshot",
		"displayDescription": "Default snapshot for testing",
		"components": [
			{
				"name": "default-component",
				"containerImage": "quay.io/redhat-user-workloads/test/component@sha256:abc123"
			}
		]
	}`

	docString := &godog.DocString{
		Content: defaultSpec,
	}

	return createValidSnapshot(ctx, docString)
}

// createSnapshotSimple creates a snapshot without docstring (alias for compatibility)
func createSnapshotSimple(ctx context.Context) (context.Context, error) {
	return createSnapshotInCluster(ctx)
}

// AddStepsTo adds snapshot-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^a valid snapshot with specification$`, createValidSnapshot)
	sc.Step(`^a valid snapshot with multiple components$`, createValidSnapshot)
	sc.Step(`^a valid snapshot$`, createSimpleValidSnapshot)
	sc.Step(`^an invalid snapshot with specification$`, createInvalidSnapshot)
	sc.Step(`^a snapshot in namespace "([^"]*)"$`, func(ctx context.Context, namespace string) (context.Context, error) {
		s := &SnapshotState{}
		ctx, err := testenv.SetupState(ctx, &s)
		if err != nil {
			return ctx, err
		}

		// Initialize map if not already done
		if s.Snapshots == nil {
			s.Snapshots = make(map[string]*unstructured.Unstructured)
		}

		s.Namespace = namespace

		// Create a default valid snapshot specification for this namespace
		spec := SnapshotSpec{
			Application:        fmt.Sprintf("app-%s", namespace),
			DisplayName:        fmt.Sprintf("snapshot-%s", namespace),
			DisplayDescription: fmt.Sprintf("Test snapshot for %s", namespace),
			Components: []Component{
				{
					Name:           fmt.Sprintf("component-%s", namespace),
					ContainerImage: "quay.io/redhat-user-workloads/test/component@sha256:abc123",
				},
			},
		}

		snapshot := &Snapshot{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Snapshot",
			Spec:       spec,
		}

		snapshot.Metadata.Name = fmt.Sprintf("snapshot-%s-%d", namespace, time.Now().Unix())
		snapshot.Metadata.Namespace = namespace

		unstructuredSnapshot, err := toUnstructured(snapshot)
		if err != nil {
			return ctx, fmt.Errorf("failed to convert snapshot to unstructured: %w", err)
		}

		s.Snapshots[snapshot.Metadata.Name] = unstructuredSnapshot

		return ctx, nil
	})
	sc.Step(`^(\d+) snapshots are created simultaneously$`, func(ctx context.Context, count int) (context.Context, error) {
		return createMultipleSnapshots(ctx, count)
	})
	sc.Step(`^the snapshot is created in the cluster$`, createSnapshotInCluster)
	sc.Step(`^the snapshot is created$`, createSnapshotSimple)
	sc.Step(`^both snapshots are created$`, createSnapshotInCluster)
	sc.Step(`^all snapshots are processed$`, createSnapshotInCluster)
}
