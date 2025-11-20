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

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AddToScheme registers the Konflux custom resource types with the given runtime scheme.
//
// IMPORTANT: You must call this function to register Konflux types (ReleasePlan, ReleasePlanAdmission)
// with your controller-runtime client's scheme before creating an Executor. Without this registration,
// the Executor will fail to query Konflux resources and policy discovery will not work.
//
// Example:
//
//	scheme := runtime.NewScheme()
//	_ = clientgoscheme.AddToScheme(scheme)  // Register standard Kubernetes types
//	if err := vsajob.AddToScheme(scheme); err != nil {
//	    return fmt.Errorf("failed to register Konflux types: %w", err)
//	}
//	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
//
// Registered types:
//   - ReleasePlan (appstudio.redhat.com/v1alpha1)
//   - ReleasePlanList (appstudio.redhat.com/v1alpha1)
//   - ReleasePlanAdmission (appstudio.redhat.com/v1alpha1)
func AddToScheme(s *runtime.Scheme) error {
	gv := schema.GroupVersion{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
	}
	s.AddKnownTypes(gv,
		&ReleasePlan{},
		&ReleasePlanList{},
		&ReleasePlanAdmission{},
	)
	metav1.AddToGroupVersion(s, gv)
	return nil
}

// controllerRuntimeClient defines the minimal interface required from a controller-runtime client.
// This interface allows the library to interact with Kubernetes API while remaining testable
// and loosely coupled to the specific controller-runtime version.
type controllerRuntimeClient interface {
	client.Reader // Provides Get, List operations
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Scheme() *runtime.Scheme
}

// ---------------------------------------------------------------------------
// Job execution
// ---------------------------------------------------------------------------

// Snapshot represents an application snapshot that needs VSA generation.
//
// A snapshot captures a specific point-in-time state of an application's components
// (container images) that have been built and are ready for verification and release.
//
// The Spec field contains the full snapshot specification as raw JSON, which includes:
//   - application: Name of the application this snapshot belongs to
//   - components: List of component images with their digests and metadata
//
// Example Spec JSON structure:
//
//	{
//	  "application": "my-app",
//	  "components": [
//	    {
//	      "name": "backend",
//	      "containerImage": "quay.io/myorg/backend@sha256:abc123...",
//	      "source": {...}
//	    }
//	  ]
//	}
//
// This type is intentionally minimal to avoid tight coupling with the full Konflux Snapshot CRD.
type Snapshot struct {
	Name      string          // Kubernetes object name of the snapshot
	Namespace string          // Kubernetes namespace where the snapshot exists
	Spec      json.RawMessage // Snapshot specification as raw JSON
}

// ---------------------------------------------------------------------------
// ReleasePlan - Konflux custom resource (appstudio.redhat.com/v1alpha1)
// ---------------------------------------------------------------------------

// ReleasePlanList contains a list of ReleasePlan resources.
// This type is required for listing ReleasePlans via the Kubernetes API.
type ReleasePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ReleasePlan `json:"items"`
}

// DeepCopyObject implements runtime.Object interface for ReleasePlanList.
func (r *ReleasePlanList) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlanList)
	*out = *r
	return out
}

// ReleasePlan is a Konflux custom resource that defines how an application should be released.
//
// A ReleasePlan connects an application to a ReleasePlanAdmission (RPA) in a target namespace,
// which in turn specifies the EnterpriseContractPolicy to use for compliance verification.
//
// This is a minimal stub type containing only the fields needed for policy discovery.
// The full Konflux ReleasePlan CRD contains additional fields not represented here.
type ReleasePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReleasePlanSpec `json:"spec,omitempty"`
}

// DeepCopyObject implements runtime.Object interface for ReleasePlan.
func (r *ReleasePlan) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlan)
	*out = *r
	return out
}

// rpaNamespace returns the namespace where the associated ReleasePlanAdmission exists.
// This is typically a centralized tenant namespace (e.g., "rhtap-releng-tenant").
func (rp *ReleasePlan) rpaNamespace() string {
	return rp.Spec.Target
}

// rpaName returns the name of the associated ReleasePlanAdmission.
// The name is stored as a label rather than in the spec for historical reasons
// in the Konflux architecture.
func (rp *ReleasePlan) rpaName() string {
	return rp.Labels["release.appstudio.openshift.io/releasePlanAdmission"]
}

// ReleasePlanSpec contains the core specification for a ReleasePlan.
type ReleasePlanSpec struct {
	Application string `json:"application"` // Name of the application this plan applies to
	Target      string `json:"target"`      // Namespace containing the ReleasePlanAdmission
}

// ---------------------------------------------------------------------------
// ReleasePlanAdmission - Konflux custom resource (appstudio.redhat.com/v1alpha1)
// ---------------------------------------------------------------------------

// ReleasePlanAdmission is a Konflux custom resource that governs release approval and policy enforcement.
//
// A ReleasePlanAdmission (RPA) is typically created in a centralized tenant/admin namespace and specifies:
//   - Which EnterpriseContractPolicy to use for compliance verification
//   - Release approval workflows and automation settings
//   - Target environments for releases
//
// This is a minimal stub type containing only the fields needed for policy discovery.
// The full Konflux ReleasePlanAdmission CRD contains additional fields not represented here.
type ReleasePlanAdmission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReleasePlanAdmissionSpec `json:"spec,omitempty"`
}

// ReleasePlanAdmissionSpec contains the core specification for a ReleasePlanAdmission.
type ReleasePlanAdmissionSpec struct {
	Policy string `json:"policy"` // Name of the EnterpriseContractPolicy in the same namespace
}

// DeepCopyObject implements runtime.Object interface for ReleasePlanAdmission.
func (r *ReleasePlanAdmission) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlanAdmission)
	*out = *r
	return out
}
