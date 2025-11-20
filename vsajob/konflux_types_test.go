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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()

	// Add the types to the scheme
	err := AddToScheme(scheme)
	assert.NoError(t, err)

	// Verify that ReleasePlan was registered
	gvk := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "ReleasePlan",
	}
	obj, err := scheme.New(gvk)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	_, ok := obj.(*ReleasePlan)
	assert.True(t, ok, "Expected object to be of type *ReleasePlan")

	// Verify that ReleasePlanList was registered
	gvkList := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "ReleasePlanList",
	}
	objList, err := scheme.New(gvkList)
	assert.NoError(t, err)
	assert.NotNil(t, objList)
	_, ok = objList.(*ReleasePlanList)
	assert.True(t, ok, "Expected object to be of type *ReleasePlanList")

	// Verify that ReleasePlanAdmission was registered
	gvkRPA := schema.GroupVersionKind{
		Group:   "appstudio.redhat.com",
		Version: "v1alpha1",
		Kind:    "ReleasePlanAdmission",
	}
	objRPA, err := scheme.New(gvkRPA)
	assert.NoError(t, err)
	assert.NotNil(t, objRPA)
	_, ok = objRPA.(*ReleasePlanAdmission)
	assert.True(t, ok, "Expected object to be of type *ReleasePlanAdmission")
}
