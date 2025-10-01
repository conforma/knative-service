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
	"sync"
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

	// Setup mocks using helper functions
	configData := map[string]string{
		"POLICY_CONFIGURATION":   "test-policy",
		"PUBLIC_KEY_SECRET_NS":   "test-secret-ns",
		"PUBLIC_KEY_SECRET_NAME": "test-secret-name",
		"PUBLIC_KEY_SECRET_KEY":  "test-secret-key",
		"PUBLIC_KEY":             "test-key",
		"VSA_UPLOAD_URL":         "https://example.com/vsa-upload",
		"TASK_BUNDLE":            "quay.io/test/tekton-task:latest",
		"TASK_NAME":              "test-task",
	}
	setupConfigMapMock(mockK8s, "test-namespace", configData)
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-application", "test-namespace", "test-target")
	setupPublicKeySecretMock(mockCrtlClient, "test-secret-ns", "test-secret-name", "test-secret-key", []byte("test-public-key"))
	setupTaskRunCreationMock(mockTekton, "test-namespace")

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
			"POLICY_CONFIGURATION":   "test-policy",
			"IGNORE_REKOR":           "true",
			"PUBLIC_KEY_SECRET_NS":   "test-secret-ns",
			"PUBLIC_KEY_SECRET_NAME": "test-secret-name",
			"PUBLIC_KEY_SECRET_KEY":  "test-secret-key",
			"PUBLIC_KEY":             "test-key",
			"VSA_UPLOAD_URL":         "https://example.com/vsa-upload",
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
			"VSA_UPLOAD_URL":       "https://example.com/vsa-upload",
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
		VsaUploadUrl:        "https://example.com/vsa-upload",
		TaskBundle:          "quay.io/test/tekton-task:latest",
		TaskName:            "test-task",
	}

	// Setup mocks using helper functions
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-app", "test-namespace", "test-target")
	setupPublicKeySecretNotFoundMock(mockCrtlClient, "openshift-pipelines", "public-key")

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.NoError(t, err)
	assert.NotNil(t, taskRun)
	assert.Equal(t, "test-namespace", taskRun.Namespace)
	assert.Contains(t, taskRun.Name, "verify-conforma-test-snapshot-")
	assert.Equal(t, tektonv1.ResolverName("bundles"), taskRun.Spec.TaskRef.Resolver)

	// Check bundle resolver parameters
	resolverParams := make(map[string]string)
	for _, param := range taskRun.Spec.TaskRef.Params {
		resolverParams[param.Name] = param.Value.StringVal
	}
	assert.Equal(t, "quay.io/test/tekton-task:latest", resolverParams["bundle"])
	assert.Equal(t, "test-task", resolverParams["name"])

	// Check parameters
	params := make(map[string]string)
	for _, param := range taskRun.Spec.Params {
		params[param.Name] = param.Value.StringVal
	}

	assert.Equal(t, "test-target/test-ecp-policy", params["POLICY_CONFIGURATION"])
	assert.Equal(t, "test-key", params["PUBLIC_KEY"])
	assert.Equal(t, "true", params["IGNORE_REKOR"])
	assert.Equal(t, "false", params["STRICT"])
	assert.Equal(t, "1", params["WORKERS"])
	assert.Equal(t, "true", params["DEBUG"])
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
		VsaUploadUrl:        "https://example.com/vsa-upload",
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

	// Setup mocks using helper functions
	configData := map[string]string{
		"POLICY_CONFIGURATION":   "test-policy",
		"PUBLIC_KEY_SECRET_NS":   "test-secret-ns",
		"PUBLIC_KEY_SECRET_NAME": "test-secret-name",
		"PUBLIC_KEY_SECRET_KEY":  "test-secret-key",
		"PUBLIC_KEY":             "test-key",
		"VSA_UPLOAD_URL":         "https://example.com/vsa-upload",
		"TASK_BUNDLE":            "quay.io/test/tekton-task:latest",
		"TASK_NAME":              "test-task",
	}
	setupConfigMapMock(mockK8s, "test-namespace", configData)
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-application", "test-namespace", "test-target")
	setupPublicKeySecretMock(mockCrtlClient, "test-secret-ns", "test-secret-name", "test-secret-key", []byte("test-public-key"))
	setupTaskRunCreationMock(mockTekton, "test-namespace")

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

func TestProcessSnapshot_NoECP(t *testing.T) {
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

	// Setup mocks using helper functions
	configData := map[string]string{
		"POLICY_CONFIGURATION": "test-policy",
		"VSA_UPLOAD_URL":       "https://example.com/vsa-upload",
	}
	setupConfigMapMock(mockK8s, "test-namespace", configData)
	setupECPLookupFailureMock(mockCrtlClient)

	err := service.processSnapshot(context.Background(), snapshot)

	// Expect no error since this is normal behavior when no ECP is found
	assert.NoError(t, err)
	mockK8s.AssertExpectations(t)
	// Don't assert Tekton expectations since no TaskRun should be created
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

// Test helper functions to reduce boilerplate

func setupConfigMapMock(mockK8s *mockK8sClient, namespace string, configData map[string]string) {
	mockConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data:       configData,
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(mockConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", namespace).Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)
}

func setupSuccessfulECPLookupMocks(mockCrtlClient *mockControllerRuntimeClient, appName, namespace, target string) {
	// Setup ReleasePlan mock
	releasePlan := &konflux.ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-release-plan",
			Namespace: namespace,
			Labels: map[string]string{
				"release.appstudio.openshift.io/releasePlanAdmission": "test-rpa",
			},
		},
		Spec: konflux.ReleasePlanSpec{
			Application: appName,
			Target:      target,
		},
	}
	releasePlanList := &konflux.ReleasePlanList{
		Items: []konflux.ReleasePlan{*releasePlan},
	}
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Run(func(args mock.Arguments) {
		list := args.Get(1).(*konflux.ReleasePlanList)
		*list = *releasePlanList
	}).Return(nil)

	// Setup ReleasePlanAdmission mock
	releasePlanAdmission := &konflux.ReleasePlanAdmission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rpa",
			Namespace: target,
		},
		Spec: konflux.ReleasePlanAdmissionSpec{
			Policy: "test-ecp-policy",
		},
	}
	mockCrtlClient.On("Get", mock.Anything, mock.MatchedBy(func(key client.ObjectKey) bool {
		return key.Namespace == target && key.Name == "test-rpa"
	}), mock.AnythingOfType("*konflux.ReleasePlanAdmission"), mock.Anything).Run(func(args mock.Arguments) {
		rpa := args.Get(2).(*konflux.ReleasePlanAdmission)
		*rpa = *releasePlanAdmission
	}).Return(nil)
}

func setupPublicKeySecretMock(mockCrtlClient *mockControllerRuntimeClient, secretNs, secretName, secretKey string, publicKey []byte) {
	publicKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNs,
		},
		Data: map[string][]byte{
			secretKey: publicKey,
		},
	}
	mockCrtlClient.On("Get", mock.Anything, mock.MatchedBy(func(key client.ObjectKey) bool {
		return key.Namespace == secretNs && key.Name == secretName
	}), mock.AnythingOfType("*v1.Secret"), mock.Anything).Run(func(args mock.Arguments) {
		secret := args.Get(2).(*corev1.Secret)
		*secret = *publicKeySecret
	}).Return(nil)
}

func setupPublicKeySecretNotFoundMock(mockCrtlClient *mockControllerRuntimeClient, secretNs, secretName string) {
	mockCrtlClient.On("Get", mock.Anything, mock.MatchedBy(func(key client.ObjectKey) bool {
		return key.Namespace == secretNs && key.Name == secretName
	}), mock.AnythingOfType("*v1.Secret"), mock.Anything).Return(fmt.Errorf("secret not found"))
}

func setupTaskRunCreationMock(mockTekton *mockTektonClient, namespace string) *tektonv1.TaskRun {
	expectedTaskRun := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verify-enterprise-contract-test-snapshot-1234567890",
			Namespace: namespace,
		},
	}

	mockTaskRunCreator := &mockTektonTaskRunCreator{}
	mockTaskRunCreator.On("Create", mock.Anything, mock.AnythingOfType("*v1.TaskRun"), metav1.CreateOptions{}).Return(expectedTaskRun, nil)

	mockTektonV1 := &mockTektonV1{}
	mockTektonV1.On("TaskRuns", namespace).Return(mockTaskRunCreator)
	mockTekton.On("TektonV1").Return(mockTektonV1)

	return expectedTaskRun
}

func setupECPLookupFailureMock(mockCrtlClient *mockControllerRuntimeClient) {
	mockCrtlClient.On("List", mock.Anything, mock.AnythingOfType("*konflux.ReleasePlanList"), mock.Anything).Return(fmt.Errorf("no release plans found"))
}

// Additional test cases for better coverage

func TestHandleCloudEvent_InvalidEventData(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	// Create invalid event data
	event := cloudevents.NewEvent()
	event.SetType("dev.knative.apiserver.resource.add")
	// Set invalid JSON data
	if err := event.SetData(cloudevents.ApplicationJSON, "invalid json"); err != nil {
		t.Fatalf("Failed to set event data: %v", err)
	}

	err := service.handleCloudEvent(context.Background(), event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse event data")
	mockK8s.AssertNotCalled(t, "CoreV1")
	mockTekton.AssertNotCalled(t, "TektonV1")
}

func TestProcessSnapshot_TektonAPIFailure(t *testing.T) {
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

	// Setup mocks using helper functions
	configData := map[string]string{
		"POLICY_CONFIGURATION":   "test-policy",
		"PUBLIC_KEY_SECRET_NS":   "test-secret-ns",
		"PUBLIC_KEY_SECRET_NAME": "test-secret-name",
		"PUBLIC_KEY_SECRET_KEY":  "test-secret-key",
		"PUBLIC_KEY":             "test-key",
		"VSA_UPLOAD_URL":         "https://example.com/vsa-upload",
		"TASK_BUNDLE":            "quay.io/test/tekton-task:latest",
		"TASK_NAME":              "test-task",
	}
	setupConfigMapMock(mockK8s, "test-namespace", configData)
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-application", "test-namespace", "test-target")
	setupPublicKeySecretMock(mockCrtlClient, "test-secret-ns", "test-secret-name", "test-secret-key", []byte("test-public-key"))

	// Setup Tekton API failure
	mockTaskRunCreator := &mockTektonTaskRunCreator{}
	mockTaskRunCreator.On("Create", mock.Anything, mock.AnythingOfType("*v1.TaskRun"), metav1.CreateOptions{}).Return((*tektonv1.TaskRun)(nil), fmt.Errorf("tekton API error"))

	mockTektonV1 := &mockTektonV1{}
	mockTektonV1.On("TaskRuns", "test-namespace").Return(mockTaskRunCreator)
	mockTekton.On("TektonV1").Return(mockTektonV1)

	err := service.processSnapshot(context.Background(), snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create taskrun in cluster")
	assert.Contains(t, err.Error(), "tekton API error")
	mockK8s.AssertExpectations(t)
	mockTekton.AssertExpectations(t)
}

func TestCreateTaskRun_NoComponents(t *testing.T) {
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
		Spec: json.RawMessage(`{"application":"test-app","components":[]}`), // No components
	}

	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		VsaUploadUrl:        "https://example.com/vsa-upload",
	}

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "no components found in snapshot spec")
}

func TestCreateTaskRun_EmptyContainerImage(t *testing.T) {
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
		Spec: json.RawMessage(`{"application":"test-app","components":[{"name":"test-component","containerImage":""}]}`), // Empty container image
	}

	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		VsaUploadUrl:        "https://example.com/vsa-upload",
	}

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "no container image found in first component")
}

func TestCreateTaskRun_InvalidJSONSpec(t *testing.T) {
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
		Spec: json.RawMessage(`{"application":"test-app","components":[{"name":"test-component","containerImage":"test-image:latest"}`), // Missing closing brace
	}

	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		VsaUploadUrl:        "https://example.com/vsa-upload",
	}

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "failed to marshal snapshot spec")
}

func TestConfigMapCache_ConcurrentAccess(t *testing.T) {
	cache := newConfigMapCache(5 * time.Minute)

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("namespace-%d", i)
			config := &TaskRunConfig{PolicyConfiguration: fmt.Sprintf("policy-%d", i)}
			cache.set(key, config)

			retrieved, found := cache.get(key)
			assert.True(t, found)
			assert.Equal(t, config.PolicyConfiguration, retrieved.PolicyConfiguration)
		}(i)
	}

	wg.Wait()
}

func TestConfigMapCache_Expiry(t *testing.T) {
	cache := newConfigMapCache(1 * time.Millisecond)

	config := &TaskRunConfig{PolicyConfiguration: "test-policy"}
	cache.set("test-namespace", config)

	// Should be found immediately
	retrieved, found := cache.get("test-namespace")
	assert.True(t, found)
	assert.Equal(t, "test-policy", retrieved.PolicyConfiguration)

	// Wait for expiry
	time.Sleep(2 * time.Millisecond)

	// Should not be found after expiry
	retrieved, found = cache.get("test-namespace")
	assert.False(t, found)
	assert.Nil(t, retrieved)
}

func TestNewService_ErrorHandling(t *testing.T) {
	// This test would require mocking the k8s.NewK8sConfig function
	// For now, we'll test the error handling in NewServiceWithDependencies
	service := NewServiceWithDependencies(nil, nil, nil, nil, ServiceConfig{})

	// Test that default values are set correctly
	assert.Equal(t, "taskrun-config", service.configMapName)
	assert.NotNil(t, service.configCache)
}

func TestReadConfigMap_AllFields(t *testing.T) {
	mockK8s := &mockK8sClient{}
	mockTekton := &mockTektonClient{}
	mockCrtlClient := &mockControllerRuntimeClient{}
	zaplog := &zapLogger{l: zaptest.NewLogger(t)}

	service := NewServiceWithDependencies(mockK8s, mockTekton, mockCrtlClient, zaplog, ServiceConfig{})

	expectedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "taskrun-config"},
		Data: map[string]string{
			"POLICY_CONFIGURATION":                "test-policy",
			"PUBLIC_KEY_SECRET_NS":                "test-secret-ns",
			"PUBLIC_KEY_SECRET_NAME":              "test-secret-name",
			"PUBLIC_KEY_SECRET_KEY":               "test-secret-key",
			"PUBLIC_KEY":                          "test-key",
			"REKOR_HOST":                          "test-rekor",
			"IGNORE_REKOR":                        "true",
			"STRICT":                              "true",
			"INFO":                                "true",
			"TUF_MIRROR":                          "test-tuf",
			"SSL_CERT_DIR":                        "test-ssl",
			"CA_TRUST_CONFIGMAP_NAME":             "test-ca-trust",
			"CA_TRUST_CONFIG_MAP_KEY":             "test-ca-key",
			"EXTRA_RULE_DATA":                     "test-extra",
			"SINGLE_COMPONENT":                    "true",
			"SINGLE_COMPONENT_CUSTOM_RESOURCE":    "test-custom",
			"SINGLE_COMPONENT_CUSTOM_RESOURCE_NS": "test-custom-ns",
			"VSA_SIGNING_KEY_SECRET_NS":           "test-vsa-ns",
			"VSA_SIGNING_KEY_SECRET_NAME":         "test-vsa-name",
			"VSA_SIGNING_KEY_SECRET_KEY":          "test-vsa-key",
			"VSA_UPLOAD_URL":                      "https://example.com/vsa-upload",
			"TASK_BUNDLE":                         "quay.io/test/tekton-task:latest",
			"TASK_NAME":                           "test-task",
		},
	}

	mockConfigMapGetter := &mockK8sConfigMapGetter{}
	mockConfigMapGetter.On("Get", mock.Anything, "taskrun-config", metav1.GetOptions{}).Return(expectedConfigMap, nil)

	mockCoreV1 := &mockK8sCoreV1{}
	mockCoreV1.On("ConfigMaps", "test-namespace").Return(mockConfigMapGetter)
	mockK8s.On("CoreV1").Return(mockCoreV1)

	config, err := service.readConfigMap(context.Background(), "test-namespace")

	assert.NoError(t, err)
	assert.Equal(t, "test-policy", config.PolicyConfiguration)
	assert.Equal(t, "test-secret-ns", config.PublicKeySecretNs)
	assert.Equal(t, "test-secret-name", config.PublicKeySecretName)
	assert.Equal(t, "test-secret-key", config.PublicKeySecretKey)
	assert.Equal(t, "test-key", config.PublicKey)
	assert.Equal(t, "true", config.IgnoreRekor)
	assert.Equal(t, "test-vsa-ns", config.VsaSigningKeySecretNs)
	assert.Equal(t, "test-vsa-name", config.VsaSigningKeySecretName)
	assert.Equal(t, "test-vsa-key", config.VsaSigningKeySecretKey)
	assert.Equal(t, "https://example.com/vsa-upload", config.VsaUploadUrl)
	assert.Equal(t, "quay.io/test/tekton-task:latest", config.TaskBundle)
	assert.Equal(t, "test-task", config.TaskName)
}

func TestCreateTaskRun_MissingTaskBundle(t *testing.T) {
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

	// Config without TaskBundle - should return error
	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		VsaUploadUrl:        "https://example.com/vsa-upload",
		TaskName:            "test-task",
		// TaskBundle is missing
	}

	// Setup mocks using helper functions
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-app", "test-namespace", "test-target")
	setupPublicKeySecretNotFoundMock(mockCrtlClient, "openshift-pipelines", "public-key")

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "TASK_BUNDLE is required but not set in configmap")
}

func TestCreateTaskRun_MissingTaskName(t *testing.T) {
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

	// Config without TaskName - should return error
	config := &TaskRunConfig{
		PolicyConfiguration: "test-policy",
		VsaUploadUrl:        "https://example.com/vsa-upload",
		TaskBundle:          "quay.io/test/tekton-task:latest",
		// TaskName is missing
	}

	// Setup mocks using helper functions
	setupSuccessfulECPLookupMocks(mockCrtlClient, "test-app", "test-namespace", "test-target")
	setupPublicKeySecretNotFoundMock(mockCrtlClient, "openshift-pipelines", "public-key")

	taskRun, err := service.createTaskRun(snapshot, config)

	assert.Error(t, err)
	assert.Nil(t, taskRun)
	assert.Contains(t, err.Error(), "TASK_NAME is required but not set in configmap")
}
