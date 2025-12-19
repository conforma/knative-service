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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const snapshotStateKey = key(0)

// SnapshotState holds the state of snapshot resources
type SnapshotState struct {
	Snapshots map[string]*unstructured.Unstructured
	Namespace string
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

	// The namespace will be set when creating the snapshot in the cluster
	// We don't know the working namespace yet at this point
	s.Namespace = ""

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

	// Generate unique name with nanosecond precision
	snapshot.Metadata.Name = fmt.Sprintf("test-snapshot-%d", time.Now().UnixNano())
	snapshot.Metadata.Namespace = s.Namespace

	// Convert to unstructured for Kubernetes API
	unstructuredSnapshot, err := toUnstructured(snapshot)
	if err != nil {
		return ctx, fmt.Errorf("failed to convert snapshot to unstructured: %w", err)
	}

	s.Snapshots[snapshot.Metadata.Name] = unstructuredSnapshot

	return ctx, nil
}

// createReleasePlanResources creates ReleasePlan and ReleasePlanAdmission resources
// required for the Knative service to find the Enterprise Contract Policy
func createReleasePlanResources(ctx context.Context, cluster *kubernetes.ClusterState, appName, namespace string) error {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return fmt.Errorf("REST mapper not available")
	}

	// Create ReleasePlan
	releasePlanName := fmt.Sprintf("release-plan-%s", appName)
	rpaName := fmt.Sprintf("rpa-%s", appName)

	releasePlan := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "appstudio.redhat.com/v1alpha1",
			"kind":       "ReleasePlan",
			"metadata": map[string]interface{}{
				"name":      releasePlanName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"release.appstudio.openshift.io/releasePlanAdmission": rpaName,
				},
			},
			"spec": map[string]interface{}{
				"application": appName,
				"target":      namespace, // Use the same namespace for simplicity in tests
			},
		},
	}

	rpGVK := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "ReleasePlan",
	}
	releasePlan.SetGroupVersionKind(rpGVK)

	rpMapping, err := mapper.RESTMapping(rpGVK.GroupKind(), rpGVK.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for ReleasePlan: %w", err)
	}

	_, err = dynamicClient.Resource(rpMapping.Resource).Namespace(namespace).Create(ctx, releasePlan, metav1.CreateOptions{})
	if err != nil {
		// If already exists, delete and retry
		if apierrors.IsAlreadyExists(err) {
			deleteErr := dynamicClient.Resource(rpMapping.Resource).Namespace(namespace).Delete(ctx, releasePlanName, metav1.DeleteOptions{})
			if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				return fmt.Errorf("failed to delete existing ReleasePlan: %w", deleteErr)
			}
			time.Sleep(500 * time.Millisecond)
			_, err = dynamicClient.Resource(rpMapping.Resource).Namespace(namespace).Create(ctx, releasePlan, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create ReleasePlan after deletion: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create ReleasePlan: %w", err)
		}
	}

	// Create ReleasePlanAdmission
	releasePlanAdmission := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "appstudio.redhat.com/v1alpha1",
			"kind":       "ReleasePlanAdmission",
			"metadata": map[string]interface{}{
				"name":      rpaName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"policy": "test-ec-policy", // Test policy name
			},
		},
	}

	rpaGVK := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "ReleasePlanAdmission",
	}
	releasePlanAdmission.SetGroupVersionKind(rpaGVK)

	rpaMapping, err := mapper.RESTMapping(rpaGVK.GroupKind(), rpaGVK.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for ReleasePlanAdmission: %w", err)
	}

	_, err = dynamicClient.Resource(rpaMapping.Resource).Namespace(namespace).Create(ctx, releasePlanAdmission, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ReleasePlanAdmission: %w", err)
	}

	return nil
}

// getWorkingNamespaceFromCluster retrieves the working namespace from the cluster
func getWorkingNamespaceFromCluster(ctx context.Context, cluster *kubernetes.ClusterState) (string, error) {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return "", fmt.Errorf("cluster implementation not available")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return "", fmt.Errorf("dynamic client not available")
	}

	// List all namespaces and find the one with name 'conforma'
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	namespaces, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list namespaces: %w", err)
	}

	for _, ns := range namespaces.Items {
		name := ns.GetName()
		if name == "conforma" {
			return name, nil
		}
	}

	return "", fmt.Errorf("no working namespace found with name 'conforma'")
}

// createSnapshotInCluster creates the snapshot resource in the cluster
func createSnapshotInCluster(ctx context.Context) (context.Context, error) {
	s := testenv.FetchState[SnapshotState](ctx)
	if s == nil {
		return ctx, fmt.Errorf("no snapshots to create")
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return ctx, fmt.Errorf("cluster not initialized")
	}

	// Get the working namespace from the cluster
	// This should be the test namespace 'conforma'
	workingNamespace, err := getWorkingNamespaceFromCluster(ctx, cluster)
	if err != nil {
		return ctx, fmt.Errorf("failed to get working namespace: %w", err)
	}

	// Set the namespace in snapshot state so tests can find Jobs in the right namespace
	s.Namespace = workingNamespace

	// Create each snapshot in the cluster
	for name, snapshot := range s.Snapshots {
		// Set the namespace for the snapshot if not already set
		if snapshot.GetNamespace() == "" {
			snapshot.SetNamespace(workingNamespace)
		}

		// Extract application name from snapshot spec
		spec, found, err := unstructured.NestedMap(snapshot.Object, "spec")
		if err != nil || !found {
			return ctx, fmt.Errorf("failed to get snapshot spec for %s: %w", name, err)
		}

		appName, found, err := unstructured.NestedString(spec, "application")
		if err != nil || !found {
			return ctx, fmt.Errorf("failed to get application name from snapshot %s: %w", name, err)
		}

		namespace := snapshot.GetNamespace()

		// Create ReleasePlan and ReleasePlanAdmission before creating the snapshot
		// This ensures the service can find the ECP when processing the snapshot
		err = createReleasePlanResources(ctx, cluster, appName, namespace)
		if err != nil {
			return ctx, fmt.Errorf("failed to create ReleasePlan resources for %s: %w", name, err)
		}

		err = createSnapshotResource(ctx, cluster, snapshot)
		if err != nil {
			return ctx, fmt.Errorf("failed to create snapshot %s: %w", name, err)
		}
	}

	return ctx, nil
}

// createSnapshotResource creates a snapshot resource in Kubernetes
func createSnapshotResource(ctx context.Context, cluster *kubernetes.ClusterState, snapshot *unstructured.Unstructured) error {
	// Get the cluster implementation
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Get the dynamic client and mapper
	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return fmt.Errorf("REST mapper not available")
	}

	// Define the Snapshot GVK (GroupVersionKind)
	gvk := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "Snapshot",
	}

	// Map the GVK to a REST resource
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for Snapshot: %w", err)
	}

	// Ensure the snapshot has the correct GVK set
	snapshot.SetGroupVersionKind(gvk)

	// Get the namespace from the snapshot, or use the default from cluster
	namespace := snapshot.GetNamespace()
	if namespace == "" {
		return fmt.Errorf("snapshot namespace not specified")
	}

	// Get the resource interface for the namespace
	resourceInterface := dynamicClient.Resource(mapping.Resource).Namespace(namespace)

	// Create the snapshot resource
	_, err = resourceInterface.Create(ctx, snapshot, metav1.CreateOptions{})
	if err != nil {
		// If the snapshot already exists, delete it and retry
		if apierrors.IsAlreadyExists(err) {
			// Delete the existing snapshot
			deleteErr := resourceInterface.Delete(ctx, snapshot.GetName(), metav1.DeleteOptions{})
			if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				return fmt.Errorf("failed to delete existing snapshot %s in namespace %s: %w", snapshot.GetName(), namespace, deleteErr)
			}

			// Wait a moment for deletion to complete
			time.Sleep(1 * time.Second)

			// Retry creation
			_, err = resourceInterface.Create(ctx, snapshot, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create snapshot %s in namespace %s after deletion: %w", snapshot.GetName(), namespace, err)
			}
		} else {
			return fmt.Errorf("failed to create snapshot %s in namespace %s: %w", snapshot.GetName(), namespace, err)
		}
	}

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
	s.Namespace = "conforma"

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
	sc.Step(`^(\d+) snapshots are created simultaneously$`, func(ctx context.Context, count int) (context.Context, error) {
		return createMultipleSnapshots(ctx, count)
	})
	sc.Step(`^the snapshot is created in the cluster$`, createSnapshotInCluster)
	sc.Step(`^the snapshot is created$`, createSnapshotSimple)
	sc.Step(`^all snapshots are processed$`, createSnapshotInCluster)
}
