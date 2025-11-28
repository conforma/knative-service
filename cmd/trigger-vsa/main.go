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
	"log"
	"net/http"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceclient "github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/go-logr/logr"
	"k8s.io/klog/v2"

	"github.com/conforma/knative-service/cmd/trigger-vsa/k8s"
	"github.com/conforma/knative-service/vsajob"
)

//go:generate mockery --name CloudEventsClient --structname CloudEventsClient --with-expecter

// --- CloudEvents client abstraction ---
type CloudEventsClient interface {
	StartReceiver(ctx context.Context, fn interface{}) error
}

type realCloudEventsClient struct {
	client cloudevents.Client
}

func (r *realCloudEventsClient) StartReceiver(ctx context.Context, fn interface{}) error {
	return r.client.StartReceiver(ctx, fn)
}

// --- Service and business logic ---

type CloudEventData struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

type Service struct {
	logger         logr.Logger
	vsaJobExecutor vsajob.Executor
}

func NewService() (*Service, error) {
	crtlClient, err := k8s.NewControllerRuntimeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	logger := klog.NewKlogr()

	vsaJobExecutor := vsajob.NewExecutor(crtlClient, logger)

	// Use POD_NAMESPACE to find the ConfigMap in the same namespace as the service
	if podNamespace := os.Getenv("POD_NAMESPACE"); podNamespace != "" {
		vsaJobExecutor = vsaJobExecutor.WithNamespace(podNamespace)
	}

	return &Service{
		logger:         logger,
		vsaJobExecutor: vsaJobExecutor,
	}, nil
}

func (s *Service) handleCloudEvent(ctx context.Context, event cloudevents.Event) error {
	s.logger.Info("Received CloudEvent", "type", event.Type())
	var eventData CloudEventData
	if err := event.DataAs(&eventData); err != nil {
		return fmt.Errorf("failed to parse event data: %w", err)
	}
	if eventData.Kind != "Snapshot" || eventData.APIVersion != "appstudio.redhat.com/v1alpha1" {
		s.logger.Info("Ignoring resource", "apiVersion", eventData.APIVersion, "kind", eventData.Kind)
		return nil
	}
	s.logger.Info("Processing Snapshot", "name", eventData.Metadata.Name, "namespace", eventData.Metadata.Namespace)
	snapshot := vsajob.Snapshot{
		Name:      eventData.Metadata.Name,
		Namespace: eventData.Metadata.Namespace,
		Spec:      eventData.Spec,
	}

	if err := s.vsaJobExecutor.CreateVSAJob(ctx, snapshot); err != nil {
		s.logger.Error(err, "Failed to create VSA Job", "snapshot", eventData.Metadata.Name, "namespace", eventData.Metadata.Namespace)
		return err
	}
	s.logger.Info("VSA Job processing triggered", "snapshot", eventData.Metadata.Name, "namespace", eventData.Metadata.Namespace)
	return nil
}

// --- HTTP server ---
type Server struct {
	service  *Service
	port     string
	ceClient CloudEventsClient
}

func NewServer(service *Service, port string, ceClient CloudEventsClient) *Server {
	return &Server{service: service, port: port, ceClient: ceClient}
}

func (s *Server) Start() error {
	s.service.logger.Info("Starting server", "port", s.port)
	return s.ceClient.StartReceiver(context.Background(), s.service.handleCloudEvent)
}

func main() {
	service, err := NewService()
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	protocol, err := cehttp.New(
		cehttp.WithPath("/"),
		cehttp.WithMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Health check endpoint for observability
				if r.URL.Path == "/health" && r.Method == "GET" {
					w.WriteHeader(http.StatusOK)
					if _, writeErr := w.Write([]byte("OK")); writeErr != nil {
						// Log but don't fail - health check should be resilient
						log.Printf("Health check response write failed: %v", writeErr)
					}
					return
				}

				if r.Header.Get("Ce-Type") != "dev.knative.apiserver.resource.add" {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				next.ServeHTTP(w, r)
			})
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create protocol: %v", err)
	}
	ceClient, err := ceclient.New(protocol)
	if err != nil {
		log.Fatalf("Failed to create CloudEvents client: %v", err)
	}
	server := NewServer(service, port, &realCloudEventsClient{client: ceClient})
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
