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

package vsajob

import "context"

//go:generate mockery --name Executor --structname MockExecutor --filename mock_executor.go --outpkg vsajobtest --output vsajobtest --with-expecter

// Executor is the main interface for creating VSA (Verification Summary Attestation) generation jobs.
//
// This interface provides a high-level abstraction for triggering enterprise contract verification
// and VSA generation for Konflux application snapshots. It handles all the complexity of:
//   - Configuration management (reading from ConfigMaps)
//   - Policy discovery (traversing ReleasePlan → ReleasePlanAdmission → EnterpriseContractPolicy)
//   - Kubernetes Job creation with proper resource limits, secrets, and service accounts
//
// Typical workflow:
//  1. Create an Executor using NewExecutor()
//  2. Configure it with WithNamespace() and WithConfigMapName()
//  3. Call CreateVSAJob() when a new Snapshot is created
//  4. The Job runs asynchronously; monitor it via standard Kubernetes Job status
//
// Example usage in a Kubernetes controller:
//
//	// Setup (once during controller initialization)
//	scheme := runtime.NewScheme()
//	_ = clientgoscheme.AddToScheme(scheme)
//	_ = vsajob.AddToScheme(scheme)  // IMPORTANT: Register Konflux types
//	k8sClient, _ := client.New(config, client.Options{Scheme: scheme})
//	executor := vsajob.NewExecutor(k8sClient, logger).
//	    WithNamespace(os.Getenv("POD_NAMESPACE")).
//	    WithConfigMapName("vsa-config")
//
//	// Event handler (called for each new Snapshot)
//	func (r *SnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
//	    snapshot := vsajob.Snapshot{
//	        Name:      req.Name,
//	        Namespace: req.Namespace,
//	        Spec:      getSnapshotSpec(),  // Extract from actual Snapshot resource
//	    }
//	    if err := executor.CreateVSAJob(ctx, snapshot); err != nil {
//	        return ctrl.Result{}, err
//	    }
//	    return ctrl.Result{}, nil
//	}
type Executor interface {
	// CreateVSAJob creates and launches a VSA generation Job for the given snapshot.
	//
	// Workflow:
	//  1. Validates the snapshot has required fields (Name, Namespace)
	//  2. Reads Job configuration from the ConfigMap (container image, resources, etc.)
	//  3. Reads VSA generation settings from the ConfigMap (keys, upload URL, etc.)
	//  4. Discovers the EnterpriseContractPolicy by querying ReleasePlan and ReleasePlanAdmission
	//  5. Creates a Kubernetes Job in the snapshot's namespace with all necessary configuration
	//
	// The Job runs asynchronously and this method returns immediately (fire-and-forget pattern).
	// To monitor Job status, watch the Job resource using standard Kubernetes APIs.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - snapshot: The snapshot to generate VSA for (must have Name, Namespace, and Spec)
	//
	// Returns:
	//   - nil on success (Job was created successfully)
	//   - error if:
	//       * ConfigMap is missing or has invalid configuration
	//       * Policy discovery fails (ReleasePlanAdmission not found, etc.)
	//       * Job creation fails (permissions, quota, etc.)
	//
	// Special case: If no ReleasePlan is found for the snapshot's application, this method
	// returns nil without creating a Job. This is NOT an error - it means the snapshot is
	// not intended for release and doesn't need VSA generation.
	CreateVSAJob(ctx context.Context, snapshot Snapshot) error

	// WithConfigMapName sets a custom ConfigMap name for reading configuration.
	//
	// The ConfigMap must exist in the namespace specified by WithNamespace() and contain
	// all required fields for VSA generation.
	//
	// Default: "vsa-config"
	//
	// Returns the Executor for method chaining.
	WithConfigMapName(configMapName string) Executor

	// WithNamespace sets the namespace where the executor's service is running.
	//
	// This is used to read the ConfigMap containing VSA generation configuration.
	// It is NOT the namespace where Jobs are created - Jobs are always created in the
	// snapshot's namespace.
	//
	// Default: "conforma"
	//
	// Tip: Use os.Getenv("POD_NAMESPACE") or the Kubernetes downward API to automatically
	// detect the namespace in containerized environments.
	//
	// Returns the Executor for method chaining.
	WithNamespace(namespace string) Executor
}

// Ensure JobExecutor implements Executor interface
var _ Executor = (*executor)(nil)
