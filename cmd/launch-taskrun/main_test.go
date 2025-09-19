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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/conforma/knative-service/cmd/launch-taskrun/konflux"
)

// --- Mock implementations ---
type mockK8sClient struct{ mock.Mock }

func (m *mockK8sClient) CoreV1() K8sCoreV1 { return m.Called().Get(0).(K8sCoreV1) }

type mockK8sCoreV1 struct{ mock.Mock }

func (m *mockK8sCoreV1) ConfigMaps(ns string) K8sConfigMapGetter {
	return m.Called(ns).Get(0).(K8sConfigMapGetter)
}

type mockK8sConfigMapGetter struct{ mock.Mock }

func (m *mockK8sConfigMapGetter) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	args := m.Called(ctx, name, opts)
	return args.Get(0).(*corev1.ConfigMap), args.Error(1)
}

type mockTektonClient struct{ mock.Mock }

func (m *mockTektonClient) TektonV1() TektonV1 { return m.Called().Get(0).(TektonV1) }

type mockTektonV1 struct{ mock.Mock }

func (m *mockTektonV1) TaskRuns(ns string) TektonTaskRunCreator {
	return m.Called(ns).Get(0).(TektonTaskRunCreator)
}

type mockTektonTaskRunCreator struct{ mock.Mock }

func (m *mockTektonTaskRunCreator) Create(ctx context.Context, taskRun *tektonv1.TaskRun, opts metav1.CreateOptions) (*tektonv1.TaskRun, error) {
	args := m.Called(ctx, taskRun, opts)
	return args.Get(0).(*tektonv1.TaskRun), args.Error(1)
}

// mockLogger is kept for potential future use
// type mockLogger struct{ mock.Mock }
//
// func (m *mockLogger) Printf(format string, args ...interface{}) {
// 	m.Called(append([]interface{}{format}, args...)...)
// }

type mockControllerRuntimeClient struct {
	mock.Mock
}

func (m *mockControllerRuntimeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *mockControllerRuntimeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

type mockCloudEventsClient struct {
	mock.Mock
}

func (m *mockCloudEventsClient) StartReceiver(ctx context.Context, fn interface{}) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

func TestHandleCloudEvent_ValidSnapshot(t *testing.T) {
	// Setup mocks
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	// Create test data
	snapshotSpec := map[string]interface{}{
		"application": "test-application",
		"components": []map[string]interface{}{
			{"name": "test-component", "containerImage": "test-image:latest"},
		},
	}
	specJSON, _ := json.Marshal(snapshotSpec)

	eventData := CloudEventData{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Metadata: struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		}{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: specJSON,
	}

	eventJSON, _ := json.Marshal(eventData)
	event := cloudevents.NewEvent()
	event.SetType("dev.knative.apiserver.resource.add")
	if err := event.SetData(cloudevents.ApplicationJSON, eventJSON); err != nil {
		t.Fatalf("Failed to set event data: %v", err)
	}

	// Setup mock expectations
	mockConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION": "test-policy",
			"PUBLIC_KEY":           "test-key",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(mockConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	// Setup ECP lookup mocks - return empty lists to trigger fallback to config
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Return(fmt.Errorf("no release plans found"))

	expectedTaskRun := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verify-enterprise-contract-test-snapshot-1234567890",
			Namespace: "test-namespace",
		},
	}

	mockTaskRunCreator := &mockTektonTaskRunCreator{}
	mockTaskRunCreator.On("Create", mock.Anything, mock.AnythingOfType("*v1.TaskRun"), metav1.CreateOptions{}).Return(expectedTaskRun, nil)

	mockTektonV1 := &mockTektonV1{}
	mockTektonV1.On("TaskRuns", "test-namespace").Return(mockTaskRunCreator)
	mockTekton.On("TektonV1").Return(mockTektonV1)

	// Execute
	err := service.handleCloudEvent(context.Background(), event)

	// Assert
	assert.NoError(t, err)
	mockK8s.AssertExpectations(t)
	mockTekton.AssertExpectations(t)
}

func TestHandleCloudEvent_InvalidResource(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	eventData := CloudEventData{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Component", // Wrong resource type
	}

	eventJSON, _ := json.Marshal(eventData)
	event := cloudevents.NewEvent()
	event.SetType("dev.knative.apiserver.resource.add")
	if err := event.SetData(cloudevents.ApplicationJSON, eventJSON); err != nil {
		t.Fatalf("Failed to set event data: %v", err)
	}

	err := service.handleCloudEvent(context.Background(), event)

	assert.NoError(t, err)
	mockK8s.AssertNotCalled(t, "CoreV1")
	mockTekton.AssertNotCalled(t, "TektonV1")
}

func TestReadConfigMap_Success(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	expectedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION": "test-policy",
			"PUBLIC_KEY":           "test-key",
			"IGNORE_REKOR":         "true",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(expectedConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	// First call should fetch from K8s
	config, err := service.readConfigMap(context.Background(), "test-namespace")

	assert.NoError(t, err)
	assert.Equal(t, "test-policy", config.PolicyConfiguration)
	assert.Equal(t, "test-key", config.PublicKey)
	assert.Equal(t, "true", config.IgnoreRekor)

	// Second call should use cache (no additional K8s calls)
	config2, err := service.readConfigMap(context.Background(), "test-namespace")

	assert.NoError(t, err)
	assert.Equal(t, "test-policy", config2.PolicyConfiguration)
	assert.Equal(t, "test-key", config2.PublicKey)
	assert.Equal(t, "true", config2.IgnoreRekor)

	// Verify K8s was only called once (for the first request)
	mockK8s.AssertNumberOfCalls(t, "CoreV1", 1)
}

func TestReadConfigMap_CacheExpiry(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	// Create service with very short TTL for testing
	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{
		CacheTTL: 1 * time.Millisecond,
	})

	expectedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION": "test-policy",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(expectedConfigMap, nil).Times(2)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	// First call
	config, err := service.readConfigMap(context.Background(), "test-namespace")
	assert.NoError(t, err)
	assert.Equal(t, "test-policy", config.PolicyConfiguration)

	// Wait for cache to expire
	time.Sleep(2 * time.Millisecond)

	// Second call should fetch from K8s again due to expiry
	config2, err := service.readConfigMap(context.Background(), "test-namespace")
	assert.NoError(t, err)
	assert.Equal(t, "test-policy", config2.PolicyConfiguration)

	// Verify K8s was called twice (once for each request due to expiry)
	mockK8s.AssertNumberOfCalls(t, "CoreV1", 2)
}

func TestReadConfigMap_Error(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return((*corev1.ConfigMap)(nil), fmt.Errorf("configmap not found"))

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	config, err := service.readConfigMap(context.Background(), "test-namespace")

	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "configmap not found")
}

func TestCreateTaskRun_Success(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	snapshot := &konflux.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: json.RawMessage(`{"application":"test-app","components":[{"name":"test-component","containerImage":"test-image:latest"}]}`),
	}

	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		PublicKey:           "test-key",
		RekorHost:           "test-rekor",
	}

	// Setup ECP lookup mocks - return error to trigger fallback to config
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Return(fmt.Errorf("no release plans found"))

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.NoError(t, err)
	assert.NotNil(t, taskRun)
	assert.Equal(t, "test-namespace", taskRun.Namespace)
	assert.Contains(t, taskRun.Name, "verify-enterprise-contract-test-snapshot-")
	assert.Equal(t, tektonv1.ResolverName("bundles"), taskRun.Spec.TaskRef.Resolver)

	// Check bundle resolver parameters
	resolverParams := make(map[string]string)
	for _, param := range taskRun.Spec.TaskRef.Params {
		resolverParams[param.Name] = param.Value.StringVal
	}
	assert.Equal(t, "quay.io/conforma/tekton-task:latest", resolverParams["bundle"])
	assert.Equal(t, "verify-enterprise-contract", resolverParams["name"])

	// Check parameters
	params := make(map[string]string)
	for _, param := range taskRun.Spec.Params {
		params[param.Name] = param.Value.StringVal
	}

	assert.Equal(t, "test-policy", params["POLICY_CONFIGURATION"])
	assert.Equal(t, "test-key", params["PUBLIC_KEY"])
	assert.Equal(t, "true", params["IGNORE_REKOR"])
	assert.Equal(t, "true", params["STRICT"])
	assert.Equal(t, "true", params["INFO"])
	assert.Equal(t, "true", params["show-successes"])
	assert.Equal(t, "1", params["WORKERS"])
	assert.Contains(t, params["IMAGES"], "test-app")
	assert.Contains(t, params["IMAGES"], "test-component")
}

func TestCreateTaskRun_InvalidSpec(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	snapshot := &konflux.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: json.RawMessage(`invalid json`), // Invalid JSON
	}

	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
	}

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "failed to marshal snapshot spec")
}

func TestProcessSnapshot_Success(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	snapshot := &konflux.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: json.RawMessage(`{"application":"test-application","components":[{"name":"test-component","containerImage":"test-image:latest"}]}`),
	}

	// Setup configmap mock
	mockConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION": "test-policy",
			"PUBLIC_KEY":           "test-key",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(mockConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	// Setup ECP lookup mocks - return error to trigger fallback to config
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Return(fmt.Errorf("no release plans found"))

	// Setup taskrun creation mock
	expectedTaskRun := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verify-enterprise-contract-test-snapshot-1234567890",
			Namespace: "test-namespace",
		},
	}

	mockTaskRunCreator := &mockTektonTaskRunCreator{}
	mockTaskRunCreator.On("Create", mock.Anything, mock.AnythingOfType("*v1.TaskRun"), metav1.CreateOptions{}).Return(expectedTaskRun, nil)

	mockTektonV1 := &mockTektonV1{}
	mockTektonV1.On("TaskRuns", "test-namespace").Return(mockTaskRunCreator)
	mockTekton.On("TektonV1").Return(mockTektonV1)

	err := service.processSnapshot(context.Background(), snapshot)

	assert.NoError(t, err)
	mockK8s.AssertExpectations(t)
	mockTekton.AssertExpectations(t)
}

func TestProcessSnapshot_ConfigMapError(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	snapshot := &konflux.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: json.RawMessage(`{"application":"test-application","components":[{"name":"test-component","containerImage":"test-image:latest"}]}`),
	}

	// Setup configmap error
	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return((*corev1.ConfigMap)(nil), fmt.Errorf("configmap not found"))

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	err := service.processSnapshot(context.Background(), snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read configmap")
	assert.Contains(t, err.Error(), "configmap not found")
	mockTekton.AssertNotCalled(t, "TektonV1")
}

func TestProcessSnapshot_TaskRunCreationError(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	snapshot := &konflux.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-namespace",
		},
		Spec: json.RawMessage(`{"application":"test-application","components":[{"name":"test-component","containerImage":"test-image:latest"}]}`),
	}

	// Setup configmap mock
	mockConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION": "test-policy",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(mockConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	// Setup ECP lookup mocks - return error to trigger fallback to config
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Return(fmt.Errorf("no release plans found"))

	// Setup taskrun creation error
	mockTaskRunCreator := &mockTektonTaskRunCreator{}
	mockTaskRunCreator.On("Create", mock.Anything, mock.AnythingOfType("*v1.TaskRun"), metav1.CreateOptions{}).Return((*tektonv1.TaskRun)(nil), fmt.Errorf("taskrun creation failed"))

	mockTektonV1 := &mockTektonV1{}
	mockTektonV1.On("TaskRuns", "test-namespace").Return(mockTaskRunCreator)
	mockTekton.On("TektonV1").Return(mockTektonV1)

	err := service.processSnapshot(context.Background(), snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create taskrun in cluster")
	assert.Contains(t, err.Error(), "taskrun creation failed")
}

func TestNewServiceWithDependencies(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, nil, zaplog, ServiceConfig{ConfigMapName: "custom-config"})

	assert.Equal(t, mockK8s, service.k8sClient)
	assert.Equal(t, mockTekton, service.tektonClient)
	assert.Equal(t, zaplog, service.logger)
	assert.Equal(t, "custom-config", service.configMapName)
}

func TestNewServiceWithDependencies_DefaultConfigMapName(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	assert.Equal(t, "taskrun-config", service.configMapName)
}

func TestServer_Start(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}
	ceClient := &mockCloudEventsClient{}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})
	server := NewServer(service, "8080", ceClient)

	// Test that server can be created (we can't easily test the actual HTTP server in unit tests)
	assert.NotNil(t, server)
	assert.Equal(t, service, server.service)
	assert.Equal(t, "8080", server.port)
}

func TestServer_Start_UsesCloudEventsClient(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}
	ceClient := &mockCloudEventsClient{}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})
	server := NewServer(service, "8080", ceClient)

	ceClient.On("StartReceiver", mock.Anything, mock.Anything).Return(nil).Once()

	err := server.Start()
	assert.NoError(t, err)
	ceClient.AssertExpectations(t)
}
