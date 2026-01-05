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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/conforma/knative-service/vsajob/mocks"
)

func TestCreateVSAJob(t *testing.T) {
	tests := []struct {
		name                    string
		snapshot                Snapshot
		setupClientExpectations func(*mocks.ControllerRuntimeClient)
		expectedError           string // Empty string means no error expected
	}{
		{
			name:     "successful job creation with all valid inputs",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-standard"),
				)
				mockReleasePlanList(m, rpList)

				rpa := createMockReleasePlanAdmission("registry-standard", "rhtap-releng-tenant", "default-policy")
				mockReleasePlanAdmissionGet(m, rpa)

				mockJobCreate(m)
			},
			expectedError: "",
		},
		{
			name:     "failure while loading config map",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				// Mock Get for ConfigMap - return error
				m.EXPECT().Get(
					mock.Anything,
					mock.MatchedBy(func(key client.ObjectKey) bool {
						return key.Namespace == "conforma" && key.Name == "vsa-config"
					}),
					mock.AnythingOfType("*v1.ConfigMap"),
					mock.Anything,
				).Return(assert.AnError).Once()
			},
			expectedError: "failed to read job configuration",
		},
		{
			name:     "invalid value in config map: CPU_REQUEST",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["CPU_REQUEST"] = "invalid-cpu"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid CPU_REQUEST value",
		},
		{
			name:     "invalid value in config map: MEMORY_REQUEST",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["MEMORY_REQUEST"] = "invalid-memory"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid MEMORY_REQUEST value",
		},
		{
			name:     "invalid value in config map: MEMORY_LIMIT",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["MEMORY_LIMIT"] = "invalid-memory"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid MEMORY_LIMIT value",
		},
		{
			name:     "invalid value in config map: EPHEMERAL_STORAGE_REQUEST",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["EPHEMERAL_STORAGE_REQUEST"] = "invalid-storage"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid EPHEMERAL_STORAGE_REQUEST value",
		},
		{
			name:     "invalid value in config map: EPHEMERAL_STORAGE_LIMIT",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["EPHEMERAL_STORAGE_LIMIT"] = "invalid-storage"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid EPHEMERAL_STORAGE_LIMIT value",
		},
		{
			name:     "invalid value in config map: BACKOFF_LIMIT",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["BACKOFF_LIMIT"] = "invalid-backoff"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid BACKOFF_LIMIT value",
		},
		{
			name:     "invalid value in config map: WORKERS",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["WORKERS"] = "invalid-workers"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid WORKERS value",
		},
		{
			name:     "invalid value in config map: WORKERS < 0",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["WORKERS"] = "-1"
				mockConfigMapGet(m, cm)
			},
			expectedError: "WORKERS must be greater than 0",
		},
		{
			name:     "invalid value in config map: STRICT",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["STRICT"] = "yes"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid STRICT value",
		},
		{
			name:     "invalid value in config map: IGNORE_REKOR",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["IGNORE_REKOR"] = "yes"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid IGNORE_REKOR value",
		},
		{
			name:     "invalid value in config map: DEBUG",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				cm.Data["DEBUG"] = "yes"
				mockConfigMapGet(m, cm)
			},
			expectedError: "invalid DEBUG value",
		},
		{
			name:     "missing value in config map: PUBLIC_KEY",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				delete(cm.Data, "PUBLIC_KEY")
				mockConfigMapGet(m, cm)
			},
			expectedError: "PUBLIC_KEY is required",
		},
		{
			name:     "missing value in config map: VSA_UPLOAD_URL",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				delete(cm.Data, "VSA_UPLOAD_URL")
				mockConfigMapGet(m, cm)
			},
			expectedError: "VSA_UPLOAD_URL is required",
		},
		{
			name:     "missing value in config map: VSA_SIGNING_KEY_SECRET_NAME",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				delete(cm.Data, "VSA_SIGNING_KEY_SECRET_NAME")
				mockConfigMapGet(m, cm)
			},
			expectedError: "VSA_SIGNING_KEY_SECRET_NAME is required",
		},
		{
			name:     "missing snapshot name in snapshot",
			snapshot: createMockSnapshot("", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				// No expectations - validation happens before any client calls
			},
			expectedError: "snapshot name is required",
		},
		{
			name:     "missing snapshot namespace in snapshot",
			snapshot: createMockSnapshot("test-snapshot", "", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				// No expectations - validation happens before any client calls
			},
			expectedError: "snapshot namespace is required",
		},
		{
			name: "invalid snapshot spec object in snapshot",
			snapshot: Snapshot{
				Name:      "test-snapshot",
				Namespace: "test-namespace",
				Spec:      []byte(`{invalid json`),
			},
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)
			},
			expectedError: "failed to unmarshal snapshot spec to extract application",
		},
		{
			name:     "no release plan found in namespace",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				emptyList := createMockReleasePlanList()
				mockReleasePlanList(m, emptyList)
			},
			expectedError: "", // No error - function returns nil when no ReleasePlan found
		},
		{
			name:     "release plan lookup failure",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				// Mock List for ReleasePlans - return error
				m.EXPECT().List(
					mock.Anything,
					mock.AnythingOfType("*vsajob.ReleasePlanList"),
					mock.Anything,
				).Return(assert.AnError).Once()
			},
			expectedError: "failed to lookup release plan",
		},
		{
			name:     "no matching release plan found for snapshot",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan", "test-namespace", "different-app", "rhtap-releng-tenant", "registry-standard"),
				)
				mockReleasePlanList(m, rpList)
			},
			expectedError: "", // No error - function returns nil when no matching ReleasePlan found
		},
		{
			name:     "multiple release plans found for snapshot, using the first one",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan-1", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-standard"),
					createMockReleasePlan("test-release-plan-2", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-premium"),
				)
				mockReleasePlanList(m, rpList)

				rpa := createMockReleasePlanAdmission("registry-standard", "rhtap-releng-tenant", "default-policy")
				mockReleasePlanAdmissionGet(m, rpa)

				mockJobCreate(m)
			},
			expectedError: "", // No error - uses first plan and completes successfully
		},
		{
			name:     "release plan admission lookup failure",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-standard"),
				)
				mockReleasePlanList(m, rpList)

				// Mock Get for ReleasePlanAdmission - return error
				m.EXPECT().Get(
					mock.Anything,
					mock.MatchedBy(func(key client.ObjectKey) bool {
						return key.Namespace == "rhtap-releng-tenant" && key.Name == "registry-standard"
					}),
					mock.AnythingOfType("*vsajob.ReleasePlanAdmission"),
					mock.Anything,
				).Return(assert.AnError).Once()
			},
			expectedError: "failed to get release plan admission",
		},
		{
			name:     "error creating job",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-standard"),
				)
				mockReleasePlanList(m, rpList)

				rpa := createMockReleasePlanAdmission("registry-standard", "rhtap-releng-tenant", "default-policy")
				mockReleasePlanAdmissionGet(m, rpa)

				mockJobCreateError(m, assert.AnError)
			},
			expectedError: "failed to create job",
		},
		{
			name:     "no policy specified in RPA, use default ECP name",
			snapshot: createMockSnapshot("test-snapshot", "test-namespace", "test-app"),
			setupClientExpectations: func(m *mocks.ControllerRuntimeClient) {
				cm := createMockConfigMap("conforma", "vsa-config")
				mockConfigMapGet(m, cm)

				rpList := createMockReleasePlanList(
					createMockReleasePlan("test-release-plan", "test-namespace", "test-app", "rhtap-releng-tenant", "registry-standard"),
				)
				mockReleasePlanList(m, rpList)

				rpa := createMockReleasePlanAdmission("registry-standard", "rhtap-releng-tenant", "")
				mockReleasePlanAdmissionGet(m, rpa)

				mockJobCreate(m)
			},
			expectedError: "", // No error - should use default policy name and succeed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocks.NewControllerRuntimeClient(t)
			tt.setupClientExpectations(mockClient)

			logger := logr.Discard()

			executor := NewExecutor(mockClient, logger)
			err := executor.CreateVSAJob(context.Background(), tt.snapshot)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectedError)
			}
		})
	}
}

func createMockSnapshot(name, namespace, appName string) Snapshot {
	spec := map[string]interface{}{
		"application": appName,
		"components": []map[string]interface{}{
			{
				"name":           "backend",
				"containerImage": "quay.io/test/backend@sha256:abc123",
			},
		},
	}
	specJSON, _ := json.Marshal(spec)

	return Snapshot{
		Name:      name,
		Namespace: namespace,
		Spec:      specJSON,
	}
}

func createMockConfigMap(namespace, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			// Required fields
			"PUBLIC_KEY":                  "test-public-key",
			"VSA_UPLOAD_URL":              "https://rekor.example.com",
			"VSA_SIGNING_KEY_SECRET_NAME": "signing-key-secret",

			// Optional fields
			"GENERATOR_IMAGE":      "quay.io/conforma/cli:test",
			"SERVICE_ACCOUNT_NAME": "test-service-account",
			"CPU_REQUEST":          "200m",
			"MEMORY_REQUEST":       "512Mi",
			"MEMORY_LIMIT":         "1Gi",
			"BACKOFF_LIMIT":        "3",
			"WORKERS":              "2",
			"STRICT":               "true",
			"IGNORE_REKOR":         "false",
			"DEBUG":                "true",
		},
	}
}

func createMockReleasePlanList(plans ...ReleasePlan) *ReleasePlanList {
	return &ReleasePlanList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "ReleasePlanList",
		},
		Items: plans,
	}
}

func createMockReleasePlan(name, namespace, appName, target, rpaName string) ReleasePlan {
	return ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"release.appstudio.openshift.io/releasePlanAdmission": rpaName,
			},
		},
		Spec: ReleasePlanSpec{
			Application: appName,
			Target:      target,
		},
	}
}

func createMockReleasePlanAdmission(name, namespace, policyName string) *ReleasePlanAdmission {
	return &ReleasePlanAdmission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ReleasePlanAdmissionSpec{
			Policy: policyName,
		},
	}
}

// mockConfigMapGet sets up a mock expectation for getting a ConfigMap
func mockConfigMapGet(m *mocks.ControllerRuntimeClient, cm *corev1.ConfigMap) {
	m.EXPECT().Get(
		mock.Anything,
		mock.MatchedBy(func(key client.ObjectKey) bool {
			return key.Namespace == cm.Namespace && key.Name == cm.Name
		}),
		mock.AnythingOfType("*v1.ConfigMap"),
		mock.Anything,
	).Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
		if configMap, ok := obj.(*corev1.ConfigMap); ok {
			*configMap = *cm
		}
	}).Return(nil).Once()
}

// mockReleasePlanList sets up a mock expectation for listing ReleasePlans
func mockReleasePlanList(m *mocks.ControllerRuntimeClient, rpList *ReleasePlanList) {
	m.EXPECT().List(
		mock.Anything,
		mock.AnythingOfType("*vsajob.ReleasePlanList"),
		mock.Anything,
	).Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
		if rpListObj, ok := list.(*ReleasePlanList); ok {
			*rpListObj = *rpList
		}
	}).Return(nil).Once()
}

// mockReleasePlanAdmissionGet sets up a mock expectation for getting a ReleasePlanAdmission
func mockReleasePlanAdmissionGet(m *mocks.ControllerRuntimeClient, rpa *ReleasePlanAdmission) {
	m.EXPECT().Get(
		mock.Anything,
		mock.MatchedBy(func(key client.ObjectKey) bool {
			return key.Namespace == rpa.Namespace && key.Name == rpa.Name
		}),
		mock.AnythingOfType("*vsajob.ReleasePlanAdmission"),
		mock.Anything,
	).Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
		if rpaObj, ok := obj.(*ReleasePlanAdmission); ok {
			*rpaObj = *rpa
		}
	}).Return(nil).Once()
}

// mockJobCreate sets up a mock expectation for creating a Job (successful)
func mockJobCreate(m *mocks.ControllerRuntimeClient) {
	m.EXPECT().Create(
		mock.Anything,
		mock.AnythingOfType("*v1.Job"),
		mock.Anything,
	).Return(nil).Once()
}

// mockJobCreateError sets up a mock expectation for creating a Job (with error)
func mockJobCreateError(m *mocks.ControllerRuntimeClient, err error) {
	m.EXPECT().Create(
		mock.Anything,
		mock.AnythingOfType("*v1.Job"),
		mock.Anything,
	).Return(err).Once()
}
