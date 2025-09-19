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

package konflux

// Define minimal stub types for some Konflux objects we need to access

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// We need to see and use all the data here so it's convenient to leave
	// it as a json.RawMessage rather than create a SnapshotSpec with
	// selected attributes defined. Another approach would be to use the
	// "real" type from "github.com/konflux-ci/application-api/api/v1alpha1"
	Spec json.RawMessage `json:"spec,omitempty"`
}

func (r *Snapshot) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(Snapshot)
	*out = *r
	return out
}

// ---------------------------------------------------------------------------
// ReleasePlan
// ---------------------------------------------------------------------------
type ReleasePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReleasePlanSpec `json:"spec,omitempty"`
}

type ReleasePlanSpec struct {
	Application string `json:"application"`
	Target      string `json:"target"`
}

type ReleasePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleasePlan `json:"items"`
}

func (r *ReleasePlan) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlan)
	*out = *r
	return out
}

func (r *ReleasePlanList) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlanList)
	*out = *r
	return out
}

// ---------------------------------------------------------------------------
// ReleasePlanAdmission
// ---------------------------------------------------------------------------
type ReleasePlanAdmission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReleasePlanAdmissionSpec `json:"spec,omitempty"`
}

type ReleasePlanAdmissionSpec struct {
	Policy string `json:"policy"`
}

func (r *ReleasePlanAdmission) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := new(ReleasePlanAdmission)
	*out = *r
	return out
}

// ---------------------------------------------------------------------------
// Use this to register the stub types defined here
// ---------------------------------------------------------------------------
func AddToScheme(s *runtime.Scheme) error {
	gv := schema.GroupVersion{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
	}
	s.AddKnownTypes(gv,
		&Snapshot{},
		&ReleasePlan{},
		&ReleasePlanList{},
		&ReleasePlanAdmission{},
	)
	metav1.AddToGroupVersion(s, gv)
	return nil
}
