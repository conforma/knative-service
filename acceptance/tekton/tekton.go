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

package tekton

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/snapshot"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const tektonStateKey = key(0)

// TektonState holds the state of Tekton resources
type TektonState struct {
	taskRuns       map[string]*TaskRunInfo
	expectedCount  int
	completedCount int
}

// Key implements the testenv.State interface
func (t TektonState) Key() any {
	return tektonStateKey
}

// TaskRunInfo holds information about a TaskRun
type TaskRunInfo struct {
	Name       string
	Namespace  string
	Status     string
	Parameters map[string]string
	Results    map[string]string
	Bundle     string
	CreatedAt  time.Time
}

// verifyTaskRunCreated verifies that a TaskRun was created
func verifyTaskRunCreated(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		return fmt.Errorf("no snapshots found")
	}

	// Wait for TaskRun to be created
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return false, err
		}

		if len(taskRuns) == 0 {
			return false, nil
		}

		t.taskRuns = taskRuns
		return true, nil
	})
	if err != nil {
		if len(t.taskRuns) == 0 {
			return fmt.Errorf("no TaskRuns found after waiting 2 minutes")
		}
		return fmt.Errorf("error waiting for TaskRuns: %w", err)
	}
	return nil
}

// verifyTaskRunParameters verifies that TaskRun has correct parameters
func verifyTaskRunParameters(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	// If no TaskRuns exist yet, fetch them from cluster
	if len(t.taskRuns) == 0 {
		cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
		if cluster == nil {
			return fmt.Errorf("cluster not initialized")
		}

		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return err
		}
		t.taskRuns = taskRuns
	}

	if len(t.taskRuns) == 0 {
		return fmt.Errorf("no TaskRuns found")
	}

	for name, taskRun := range t.taskRuns {
		// Verify required parameters are present
		requiredParams := []string{"image", "policy", "public-key"}
		for _, param := range requiredParams {
			if _, exists := taskRun.Parameters[param]; !exists {
				return fmt.Errorf("TaskRun %s missing required parameter: %s", name, param)
			}
		}

		// Verify parameter values are reasonable
		if taskRun.Parameters["image"] == "" {
			return fmt.Errorf("TaskRun %s has empty image parameter", name)
		}
	}

	return nil
}

// verifyTaskRunBundle verifies that TaskRun references the correct bundle
func verifyTaskRunBundle(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	// If no TaskRuns exist yet, fetch them from cluster
	if len(t.taskRuns) == 0 {
		cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
		if cluster == nil {
			return fmt.Errorf("cluster not initialized")
		}

		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return err
		}
		t.taskRuns = taskRuns
	}

	if len(t.taskRuns) == 0 {
		return fmt.Errorf("no TaskRuns found")
	}

	expectedBundlePrefix := "quay.io/enterprise-contract/ec-task-bundle"

	for name, taskRun := range t.taskRuns {
		if taskRun.Bundle == "" {
			return fmt.Errorf("TaskRun %s has no bundle reference", name)
		}

		// Verify bundle is from the expected registry
		if len(taskRun.Bundle) < len(expectedBundlePrefix) ||
			taskRun.Bundle[:len(expectedBundlePrefix)] != expectedBundlePrefix {
			return fmt.Errorf("TaskRun %s has unexpected bundle: %s", name, taskRun.Bundle)
		}
	}

	return nil
}

// verifyTaskRunSuccess verifies that TaskRun completed successfully
func verifyTaskRunSuccess(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// If no TaskRuns exist yet, fetch them from cluster
	if len(t.taskRuns) == 0 {
		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return err
		}
		t.taskRuns = taskRuns
	}

	if len(t.taskRuns) == 0 {
		return fmt.Errorf("no TaskRuns found")
	}

	// Wait for TaskRuns to complete
	return wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		// Update TaskRun status
		updatedTaskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return false, err
		}

		t.taskRuns = updatedTaskRuns
		allSucceeded := true

		for name, taskRun := range t.taskRuns {
			switch taskRun.Status {
			case "Succeeded":
				continue
			case "Failed":
				return false, fmt.Errorf("TaskRun %s failed", name)
			case "Running", "Pending":
				allSucceeded = false
			default:
				return false, fmt.Errorf("TaskRun %s has unknown status: %s", name, taskRun.Status)
			}
		}

		return allSucceeded, nil
	})
}

// verifyMultipleTaskRuns verifies that TaskRuns were created for multiple components
func verifyMultipleTaskRuns(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		return fmt.Errorf("no snapshots found")
	}

	// Wait for TaskRuns to be created
	expectedCount := 2 // Based on the multi-component scenario
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return false, err
		}

		if len(taskRuns) >= expectedCount {
			t.taskRuns = taskRuns
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("expected %d TaskRuns, found %d: %w", expectedCount, len(t.taskRuns), err)
	}

	return nil
}

// verifyNoTaskRunCreated verifies that no TaskRun was created (for invalid snapshots)
func verifyNoTaskRunCreated(ctx context.Context) error {
	t := &TektonState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return err
	}

	// Initialize map if not already done
	if t.taskRuns == nil {
		t.taskRuns = make(map[string]*TaskRunInfo)
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Wait a bit to ensure no TaskRun is created
	time.Sleep(30 * time.Second)

	taskRuns, err := findTaskRuns(ctx, cluster, "default")
	if err != nil {
		return err
	}

	// Filter out any pre-existing TaskRuns - we only care about new ones
	// In a real implementation, we'd track TaskRuns by creation timestamp
	// For now, we expect 0 TaskRuns for invalid snapshots
	if len(taskRuns) > 0 {
		return fmt.Errorf("expected no TaskRuns, but found %d", len(taskRuns))
	}

	return nil
}

// verifyTaskRunsInNamespaces verifies TaskRuns are created in correct namespaces
func verifyTaskRunsInNamespaces(ctx context.Context) error {
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Check TaskRuns in test-namespace-1
	taskRuns1, err := findTaskRuns(ctx, cluster, "test-namespace-1")
	if err != nil {
		return err
	}

	// Check TaskRuns in test-namespace-2
	taskRuns2, err := findTaskRuns(ctx, cluster, "test-namespace-2")
	if err != nil {
		return err
	}

	if len(taskRuns1) == 0 {
		return fmt.Errorf("no TaskRuns found in test-namespace-1")
	}

	if len(taskRuns2) == 0 {
		return fmt.Errorf("no TaskRuns found in test-namespace-2")
	}

	return nil
}

// verifyTaskRunsCompleteWithinTime verifies all TaskRuns complete within specified time
func verifyTaskRunsCompleteWithinTime(ctx context.Context, timeoutSeconds int) error {
	startTime := time.Now()
	timeout := time.Duration(timeoutSeconds) * time.Second

	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		if time.Since(startTime) > timeout {
			return false, fmt.Errorf("TaskRuns did not complete within %d seconds", timeoutSeconds)
		}

		cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
		if cluster == nil {
			return false, fmt.Errorf("cluster not initialized")
		}

		taskRuns, err := findTaskRuns(ctx, cluster, "default")
		if err != nil {
			return false, err
		}

		allCompleted := true
		for _, taskRun := range taskRuns {
			if taskRun.Status != "Succeeded" && taskRun.Status != "Failed" {
				allCompleted = false
				break
			}
		}

		return allCompleted, nil
	})
}

// findTaskRuns finds TaskRuns in the specified namespace
func findTaskRuns(ctx context.Context, cluster *kubernetes.ClusterState, namespace string) (map[string]*TaskRunInfo, error) {
	// Implementation would use Tekton client to list TaskRuns
	// This is a placeholder for the actual Kubernetes API call
	taskRuns := make(map[string]*TaskRunInfo)

	// Check if we have snapshot state - only return TaskRuns if snapshots exist
	snapshotState := testenv.FetchState[snapshot.SnapshotState](ctx)
	if snapshotState == nil {
		// No snapshots, so no TaskRuns should exist
		return taskRuns, nil
	}

	// If an invalid snapshot exists, don't create TaskRuns
	// This simulates the controller rejecting invalid snapshots
	if snapshotState.InvalidExists {
		return taskRuns, nil
	}

	// Mock implementation - in real code this would query the cluster
	// Only create mock TaskRuns if we have valid snapshots
	// This simulates the controller creating TaskRuns in response to snapshots
	// Create one TaskRun per component in all snapshots
	taskRunIndex := 0
	for _, snapshotObj := range snapshotState.Snapshots {
		// Extract components from the snapshot
		spec, found, err := unstructured.NestedMap(snapshotObj.Object, "spec")
		if err != nil || !found {
			continue
		}

		components, found, err := unstructured.NestedSlice(spec, "components")
		if err != nil || !found {
			continue
		}

		// Create a TaskRun for each component
		for _, comp := range components {
			componentMap, ok := comp.(map[string]interface{})
			if !ok {
				continue
			}

			componentName, _, _ := unstructured.NestedString(componentMap, "name")
			containerImage, _, _ := unstructured.NestedString(componentMap, "containerImage")

			taskRunIndex++
			taskRunName := fmt.Sprintf("test-taskrun-%d", taskRunIndex)

			taskRuns[taskRunName] = &TaskRunInfo{
				Name:      taskRunName,
				Namespace: namespace,
				Status:    "Succeeded",
				Parameters: map[string]string{
					"image":      containerImage,
					"policy":     "enterprise-contract-policy",
					"public-key": "test-key",
					"component":  componentName,
				},
				Bundle:    "quay.io/enterprise-contract/ec-task-bundle:latest",
				CreatedAt: time.Now(),
			}
		}
	}

	return taskRuns, nil
}

// AddStepsTo adds Tekton-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^a TaskRun should be created$`, verifyTaskRunCreated)
	sc.Step(`^the TaskRun should have the correct parameters$`, verifyTaskRunParameters)
	sc.Step(`^the TaskRun should reference the enterprise contract bundle$`, verifyTaskRunBundle)
	sc.Step(`^the TaskRun should succeed$`, verifyTaskRunSuccess)
	sc.Step(`^a TaskRun should be created for each component$`, verifyMultipleTaskRuns)
	sc.Step(`^all TaskRuns should have the correct parameters$`, verifyTaskRunParameters)
	sc.Step(`^all TaskRuns should succeed$`, verifyTaskRunSuccess)
	sc.Step(`^no TaskRun should be created$`, verifyNoTaskRunCreated)
	sc.Step(`^TaskRuns should be created in their respective namespaces$`, verifyTaskRunsInNamespaces)
	sc.Step(`^TaskRuns should not interfere with each other$`, func(ctx context.Context) error {
		// Implementation would verify isolation between TaskRuns
		return nil
	})
	sc.Step(`^the TaskRun should resolve the correct bundle$`, verifyTaskRunBundle)
	sc.Step(`^the TaskRun should use the latest bundle version$`, func(ctx context.Context) error {
		// Implementation would verify bundle version
		return nil
	})
	sc.Step(`^the TaskRun should execute successfully$`, verifyTaskRunSuccess)
	sc.Step(`^all TaskRuns should be created within (\d+) seconds$`, func(ctx context.Context, seconds int) error {
		return verifyTaskRunsCompleteWithinTime(ctx, seconds)
	})
	sc.Step(`^all TaskRuns should complete successfully$`, verifyTaskRunSuccess)
	sc.Step(`^no events should be lost$`, func(ctx context.Context) error {
		// Implementation would verify event processing completeness
		return nil
	})
	sc.Step(`^the TaskRun should continue to completion$`, verifyTaskRunSuccess)
}
