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
	"errors"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/conforma/knative-service/cmd/launch-taskrun/mocks"
	"github.com/conforma/knative-service/vsajob"
	"github.com/conforma/knative-service/vsajob/vsajobtest"
)

func TestHandleCloudEvent_ValidSnapshot(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

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
	mockExecutor.EXPECT().CreateVSAJob(mock.Anything, mock.MatchedBy(func(s vsajob.Snapshot) bool {
		return s.Name == "test-snapshot" && s.Namespace == "test-namespace"
	})).Return(nil).Once()

	// Execute
	err := service.handleCloudEvent(context.Background(), event)

	// Assert
	assert.NoError(t, err)
}

func TestHandleCloudEvent_InvalidResource(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

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

	// Execute
	err := service.handleCloudEvent(context.Background(), event)

	// Assert
	assert.NoError(t, err)
	mockExecutor.AssertNotCalled(t, "CreateVSAJob")
}

func TestHandleCloudEvent_ExecutorError(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

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
	testError := errors.New("test error")
	mockExecutor.EXPECT().CreateVSAJob(mock.Anything, mock.Anything).Return(testError).Once()

	// Execute
	err := service.handleCloudEvent(context.Background(), event)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, testError, err)
}

func TestHandleCloudEvent_InvalidEventData(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

	event := cloudevents.NewEvent()
	event.SetType("dev.knative.apiserver.resource.add")
	// Set invalid data (not JSON)
	if err := event.SetData(cloudevents.ApplicationJSON, "invalid-data"); err != nil {
		t.Fatalf("Failed to set event data: %v", err)
	}

	// Execute
	err := service.handleCloudEvent(context.Background(), event)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse event data")
	mockExecutor.AssertNotCalled(t, "CreateVSAJob")
}

func TestNewServer(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)
	mockCEClient := mocks.NewCloudEventsClient(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

	server := NewServer(service, "8080", mockCEClient)

	assert.NotNil(t, server)
	assert.Equal(t, service, server.service)
	assert.Equal(t, "8080", server.port)
	assert.Equal(t, mockCEClient, server.ceClient)
}

func TestServer_Start(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)
	mockCEClient := mocks.NewCloudEventsClient(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

	server := NewServer(service, "8080", mockCEClient)

	// Setup mock expectations
	mockCEClient.EXPECT().StartReceiver(mock.Anything, mock.Anything).Return(nil).Once()

	// Execute
	err := server.Start()

	// Assert
	assert.NoError(t, err)
}

func TestServer_Start_Error(t *testing.T) {
	mockExecutor := vsajobtest.NewMockExecutor(t)
	testLogger := testr.New(t)
	mockCEClient := mocks.NewCloudEventsClient(t)

	service := &Service{
		logger:         testLogger,
		vsaJobExecutor: mockExecutor,
	}

	server := NewServer(service, "8080", mockCEClient)

	// Setup mock expectations
	testError := errors.New("test error")
	mockCEClient.EXPECT().StartReceiver(mock.Anything, mock.Anything).Return(testError).Once()

	// Execute
	err := server.Start()

	// Assert
	assert.Error(t, err)
	assert.Equal(t, testError, err)
}
