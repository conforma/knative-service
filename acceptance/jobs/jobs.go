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

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/snapshot"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const jobsStateKey = key(0)

// JobsState holds the state of Job resources
type JobsState struct {
	jobs           map[string]*JobInfo
	expectedCount  int
	completedCount int
}

// Key implements the testenv.State interface
func (j JobsState) Key() any {
	return jobsStateKey
}

// JobInfo holds information about a Job
type JobInfo struct {
	Name         string
	Namespace    string
	Status       string
	Args         []string
	Env          map[string]string
	CreatedAt    time.Time
	SnapshotName string // From annotation conforma.dev/snapshot-name
}

// verifyJobCreated verifies that a Job was created
func verifyJobCreated(ctx context.Context) error {
	j := &JobsState{}
	ctx, err := testenv.SetupState(ctx, &j)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if j.jobs == nil {
		j.jobs = make(map[string]*JobInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		return fmt.Errorf("no snapshots found")
	}

	// Get the namespace from snapshot state
	namespace := snapshotState.Namespace
	if namespace == "" {
		// Try to get namespace from first snapshot if available
		for _, snap := range snapshotState.Snapshots {
			namespace = snap.GetNamespace()
			break
		}
	}
	if namespace == "" {
		namespace = "default"
	}

	// Wait for Job to be created
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		jobs, err := findJobs(ctx, cluster, namespace)
		if err != nil {
			return false, err
		}

		if len(jobs) == 0 {
			return false, nil
		}

		j.jobs = jobs
		return true, nil
	})
	if err != nil {
		if len(j.jobs) == 0 {
			return fmt.Errorf("no Jobs found after waiting 2 minutes")
		}
		return fmt.Errorf("error waiting for Jobs: %w", err)
	}
	return nil
}

// verifyJobParameters verifies that Job has correct parameters
func verifyJobParameters(ctx context.Context) error {
	j := &JobsState{}
	ctx, err := testenv.SetupState(ctx, &j)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if j.jobs == nil {
		j.jobs = make(map[string]*JobInfo)
	}

	// If no Jobs exist yet, fetch them from cluster
	if len(j.jobs) == 0 {
		cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
		if cluster == nil {
			return fmt.Errorf("cluster not initialized")
		}

		snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
		namespace := getNamespaceFromSnapshotState(snapshotState)

		jobs, err := findJobs(ctx, cluster, namespace)
		if err != nil {
			return err
		}
		j.jobs = jobs
	}

	if len(j.jobs) == 0 {
		return fmt.Errorf("no Jobs found")
	}

	for name, job := range j.jobs {
		// Verify the job has the expected conforma command
		if len(job.Args) == 0 {
			return fmt.Errorf("Job %s has no arguments", name)
		}

		// Check for mandatory arguments according to vsajob.buildJob implementation
		// Based on executor.go lines 477-501
		hasValidate := false
		hasImage := false
		hasImages := false
		hasPolicy := false
		hasPublicKey := false
		hasVSA := false
		hasVSASigningKey := false
		hasVSAUpload := false

		for i, arg := range job.Args {
			switch arg {
			case "validate":
				hasValidate = true
			case "image":
				hasImage = true
			case "--images":
				hasImages = i+1 < len(job.Args) && job.Args[i+1] != ""
			case "--policy":
				hasPolicy = i+1 < len(job.Args) && job.Args[i+1] != ""
			case "--public-key":
				hasPublicKey = i+1 < len(job.Args) && job.Args[i+1] != ""
			case "--vsa":
				hasVSA = true
			case "--vsa-signing-key":
				hasVSASigningKey = i+1 < len(job.Args) && job.Args[i+1] != ""
			case "--vsa-upload":
				hasVSAUpload = i+1 < len(job.Args) && job.Args[i+1] != ""
			}
		}

		if !hasValidate {
			return fmt.Errorf("Job %s missing 'validate' command", name)
		}
		if !hasImage {
			return fmt.Errorf("Job %s missing 'image' subcommand", name)
		}
		if !hasImages {
			return fmt.Errorf("Job %s missing '--images' parameter with snapshot spec", name)
		}
		if !hasPolicy {
			return fmt.Errorf("Job %s missing '--policy' parameter", name)
		}
		if !hasPublicKey {
			return fmt.Errorf("Job %s missing '--public-key' parameter", name)
		}
		if !hasVSA {
			return fmt.Errorf("Job %s missing '--vsa' flag for VSA generation", name)
		}
		if !hasVSASigningKey {
			return fmt.Errorf("Job %s missing '--vsa-signing-key' parameter", name)
		}
		if !hasVSAUpload {
			return fmt.Errorf("Job %s missing '--vsa-upload' parameter", name)
		}
	}

	return nil
}

// verifyJobParameterValues verifies that Job parameters have exact correct values extracted from the environment
// This checks that the snapshot spec, policy reference, and other critical values match what was configured
func verifyJobParameterValues(ctx context.Context) error {
	j := &JobsState{}
	ctx, err := testenv.SetupState(ctx, &j)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if j.jobs == nil {
		j.jobs = make(map[string]*JobInfo)
	}

	// If no Jobs exist yet, fetch them from cluster
	if len(j.jobs) == 0 {
		cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
		if cluster == nil {
			return fmt.Errorf("cluster not initialized")
		}

		snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
		namespace := getNamespaceFromSnapshotState(snapshotState)

		jobs, err := findJobs(ctx, cluster, namespace)
		if err != nil {
			return err
		}
		j.jobs = jobs
	}

	if len(j.jobs) == 0 {
		return fmt.Errorf("no Jobs found")
	}

	// Get the snapshot state to get the namespace
	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		return fmt.Errorf("snapshot state not found")
	}

	// Get the cluster to fetch actual snapshot from API
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	for name, job := range j.jobs {
		// Extract parameter values from Job args
		imagesValue := ""
		policyValue := ""
		publicKeyValue := ""

		for i, arg := range job.Args {
			switch arg {
			case "--images":
				if i+1 < len(job.Args) {
					imagesValue = job.Args[i+1]
				}
			case "--policy":
				if i+1 < len(job.Args) {
					policyValue = job.Args[i+1]
				}
			case "--public-key":
				if i+1 < len(job.Args) {
					publicKeyValue = job.Args[i+1]
				}
			}
		}

		// Fetch the snapshot from the cluster API using the annotation
		// This is more reliable than relying on in-memory state
		if job.SnapshotName == "" {
			return fmt.Errorf("Job %s has no snapshot name annotation", name)
		}

		snapshotObj, err := fetchSnapshotFromCluster(ctx, cluster, job.Namespace, job.SnapshotName)
		if err != nil {
			return fmt.Errorf("failed to fetch snapshot %s for Job %s: %w", job.SnapshotName, name, err)
		}

		// Get the snapshot spec
		spec, found, err := unstructured.NestedMap(snapshotObj.Object, "spec")
		if err != nil || !found {
			return fmt.Errorf("failed to get snapshot spec: %w", err)
		}

		// Convert spec to JSON to compare with --images parameter
		specBytes, err := json.Marshal(spec)
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot spec: %w", err)
		}
		expectedImagesValue := string(specBytes)

		// Verify --images parameter matches the snapshot spec exactly
		if imagesValue != expectedImagesValue {
			return fmt.Errorf("Job %s --images parameter doesn't match snapshot spec.\nExpected: %s\nGot: %s",
				name, expectedImagesValue, imagesValue)
		}

		// Verify --policy is in correct namespace/name format
		// The policy should reference test-ec-policy in the snapshot's namespace
		if policyValue == "" {
			return fmt.Errorf("Job %s has empty --policy parameter", name)
		}
		if !strings.Contains(policyValue, "/") {
			return fmt.Errorf("Job %s --policy parameter %q is not in 'namespace/name' format", name, policyValue)
		}
		policyParts := strings.Split(policyValue, "/")
		if len(policyParts) != 2 {
			return fmt.Errorf("Job %s --policy parameter %q has invalid 'namespace/name' format", name, policyValue)
		}
		// The namespace should match the snapshot namespace and policy name should be test-ec-policy
		expectedPolicyNamespace := snapshotState.Namespace
		expectedPolicyName := "test-ec-policy"
		if policyParts[0] != expectedPolicyNamespace {
			return fmt.Errorf("Job %s --policy namespace is %q, expected %q", name, policyParts[0], expectedPolicyNamespace)
		}
		if policyParts[1] != expectedPolicyName {
			return fmt.Errorf("Job %s --policy name is %q, expected %q", name, policyParts[1], expectedPolicyName)
		}

		// Verify --public-key matches the configured value from ConfigMap
		// In tests, this should be "k8s://openshift-pipelines/public-key"
		expectedPublicKey := "k8s://openshift-pipelines/public-key"
		if publicKeyValue != expectedPublicKey {
			return fmt.Errorf("Job %s --public-key is %q, expected %q", name, publicKeyValue, expectedPublicKey)
		}
	}

	return nil
}

// verifyMultipleJobs verifies that Jobs were created for multiple components
func verifyMultipleJobs(ctx context.Context) error {
	j := &JobsState{}
	ctx, err := testenv.SetupState(ctx, &j)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if j.jobs == nil {
		j.jobs = make(map[string]*JobInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		return fmt.Errorf("no snapshots found")
	}

	namespace := getNamespaceFromSnapshotState(snapshotState)

	// Wait for Jobs to be created
	expectedCount := 2 // Based on the multi-component scenario
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		jobs, err := findJobs(ctx, cluster, namespace)
		if err != nil {
			return false, err
		}

		if len(jobs) >= expectedCount {
			j.jobs = jobs
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("expected %d Jobs, found %d: %w", expectedCount, len(j.jobs), err)
	}

	return nil
}

// getNamespaceFromSnapshotState retrieves the namespace from snapshot state
func getNamespaceFromSnapshotState(snapshotState *snapshot.SnapshotState) string {
	if snapshotState == nil {
		return "default"
	}

	namespace := snapshotState.Namespace
	if namespace == "" {
		// Try to get namespace from first snapshot if available
		for _, snap := range snapshotState.Snapshots {
			namespace = snap.GetNamespace()
			break
		}
	}
	if namespace == "" {
		namespace = "default"
	}
	return namespace
}

// findJobs finds Jobs in the specified namespace with the vsa-generator label
func findJobs(ctx context.Context, cluster *kubernetes.ClusterState, namespace string) (map[string]*JobInfo, error) {
	jobs := make(map[string]*JobInfo)

	// Get the cluster implementation
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return jobs, fmt.Errorf("cluster not initialized")
	}

	// Get the dynamic client and mapper
	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return jobs, fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return jobs, fmt.Errorf("REST mapper not available")
	}

	// Define the Job GVK (GroupVersionKind)
	gvk := schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1",
		Kind:    "Job",
	}

	// Map the GVK to a REST resource
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return jobs, fmt.Errorf("failed to get REST mapping for Job: %w", err)
	}

	// Get the resource interface for the namespace
	resourceInterface := dynamicClient.Resource(mapping.Resource).Namespace(namespace)

	// List Jobs with the vsa-generator label
	list, err := resourceInterface.List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=vsa-generator",
	})
	if err != nil {
		return jobs, fmt.Errorf("failed to list Jobs: %w", err)
	}

	// Parse each Job
	for _, item := range list.Items {
		jobInfo, err := parseJob(&item)
		if err != nil {
			// Log error but continue processing other Jobs
			continue
		}
		jobs[jobInfo.Name] = jobInfo
	}

	return jobs, nil
}

// fetchSnapshotFromCluster retrieves a snapshot from the Kubernetes API
func fetchSnapshotFromCluster(ctx context.Context, cluster *kubernetes.ClusterState, namespace, name string) (*unstructured.Unstructured, error) {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return nil, fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return nil, fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return nil, fmt.Errorf("REST mapper not available")
	}

	// Define the Snapshot GVK
	gvk := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "Snapshot",
	}

	// Map the GVK to a REST resource
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to get REST mapping for Snapshot: %w", err)
	}

	// Get the snapshot
	snapshot, err := dynamicClient.Resource(mapping.Resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Snapshot %s in namespace %s: %w", name, namespace, err)
	}

	return snapshot, nil
}

// parseJob extracts JobInfo from an unstructured Job object
func parseJob(obj *unstructured.Unstructured) (*JobInfo, error) {
	info := &JobInfo{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Env:       make(map[string]string),
	}

	// Get creation timestamp
	info.CreatedAt = obj.GetCreationTimestamp().Time

	// Get snapshot name from annotation
	annotations := obj.GetAnnotations()
	if snapshotName, found := annotations["conforma.dev/snapshot-name"]; found {
		info.SnapshotName = snapshotName
	}

	// Get status - default to Pending if no status is available
	info.Status = "Pending"

	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if err == nil && found {
		// Check for completion conditions
		conditions, found, err := unstructured.NestedSlice(status, "conditions")
		if err == nil && found && len(conditions) > 0 {
			for _, cond := range conditions {
				if condition, ok := cond.(map[string]interface{}); ok {
					condType, typeFound, _ := unstructured.NestedString(condition, "type")
					condStatus, statusFound, _ := unstructured.NestedString(condition, "status")

					if typeFound && statusFound && condStatus == "True" {
						if condType == "Complete" {
							info.Status = "Succeeded"
						} else if condType == "Failed" {
							info.Status = "Failed"
						}
					}
				}
			}
		}

		// Check if job is active (running)
		active, found, err := unstructured.NestedInt64(status, "active")
		if err == nil && found && active > 0 && info.Status == "Pending" {
			info.Status = "Running"
		}
	}

	// Get spec to extract container args
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return info, nil
	}

	template, found, err := unstructured.NestedMap(spec, "template")
	if err != nil || !found {
		return info, nil
	}

	podSpec, found, err := unstructured.NestedMap(template, "spec")
	if err != nil || !found {
		return info, nil
	}

	containers, found, err := unstructured.NestedSlice(podSpec, "containers")
	if err != nil || !found || len(containers) == 0 {
		return info, nil
	}

	// Get the first container
	if container, ok := containers[0].(map[string]interface{}); ok {
		// Get args
		args, found, err := unstructured.NestedStringSlice(container, "args")
		if err == nil && found {
			info.Args = args
		}

		// Get env variables
		envVars, found, err := unstructured.NestedSlice(container, "env")
		if err == nil && found {
			for _, envVar := range envVars {
				if envMap, ok := envVar.(map[string]interface{}); ok {
					name, _, _ := unstructured.NestedString(envMap, "name")
					value, _, _ := unstructured.NestedString(envMap, "value")
					if name != "" {
						info.Env[name] = value
					}
				}
			}
		}
	}

	return info, nil
}

// AddStepsTo adds Job-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^a Job should be created$`, verifyJobCreated)
	sc.Step(`^the Job should have the correct parameters$`, verifyJobParameters)
	sc.Step(`^the Job parameters should have correct values$`, verifyJobParameterValues)
	sc.Step(`^a Job should be created for each component$`, verifyMultipleJobs)
	sc.Step(`^all Jobs should have the correct parameters$`, verifyJobParameters)
	sc.Step(`^all Jobs parameters should have correct values$`, verifyJobParameterValues)
}
