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

package knative

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/cucumber/godog"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	util "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/conforma/knative-service/acceptance/kubernetes"
	"github.com/conforma/knative-service/acceptance/kustomize"
	"github.com/conforma/knative-service/acceptance/log"
	"github.com/conforma/knative-service/acceptance/registry"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const knativeStateKey = key(0)

// KnativeState holds the state of Knative components
type KnativeState struct {
	servingInstalled  bool
	eventingInstalled bool
	serviceDeployed   bool
	serviceURL        string
}

// Key implements the testenv.State interface
func (k KnativeState) Key() any {
	return knativeStateKey
}

// installKnative verifies Knative Serving and Eventing are installed and ready
// Note: The actual installation happens during cluster setup via hack/test/kustomization.yaml
func installKnative(ctx context.Context) (context.Context, error) {
	k := &KnativeState{}
	ctx, err := testenv.SetupState(ctx, &k)
	if err != nil {
		return ctx, err
	}

	if k.servingInstalled && k.eventingInstalled {
		return ctx, nil
	}

	logger, ctx := log.LoggerFor(ctx)
	logger.Info("Verifying Knative installation...")

	// Get cluster state
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		// For stub testing, allow nil cluster
		k.servingInstalled = true
		k.eventingInstalled = true
		return ctx, nil
	}

	// Verify Knative Serving is installed (should already be installed by cluster setup)
	if !k.servingInstalled {
		logger.Info("Verifying Knative Serving installation...")
		if err := verifyKnativeServing(ctx, cluster); err != nil {
			return ctx, fmt.Errorf("Knative Serving not properly installed: %w", err)
		}
		k.servingInstalled = true
	}

	// Verify Knative Eventing is installed (should already be installed by cluster setup)
	if !k.eventingInstalled {
		logger.Info("Verifying Knative Eventing installation...")
		if err := verifyKnativeEventing(ctx, cluster); err != nil {
			return ctx, fmt.Errorf("Knative Eventing not properly installed: %w", err)
		}
		k.eventingInstalled = true
	}

	logger.Info("Knative installation verified successfully")
	return ctx, nil
}

// verifyKnativeServing verifies that Knative Serving components are installed and ready
func verifyKnativeServing(ctx context.Context, cluster *kubernetes.ClusterState) error {
	logger, ctx := log.LoggerFor(ctx)

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Wait for Knative Serving components to be ready
	logger.Info("Waiting for Knative Serving components to be ready...")
	if err := waitForDeployments(ctx, cluster, "knative-serving", 5*time.Minute); err != nil {
		return fmt.Errorf("Knative Serving components not ready: %w", err)
	}

	// Wait for Kourier networking to be ready
	logger.Info("Waiting for Kourier networking to be ready...")
	if err := waitForDeployments(ctx, cluster, "kourier-system", 5*time.Minute); err != nil {
		return fmt.Errorf("Kourier components not ready: %w", err)
	}

	logger.Info("Knative Serving is ready")
	return nil
}

// verifyKnativeEventing verifies that Knative Eventing components are installed and ready
func verifyKnativeEventing(ctx context.Context, cluster *kubernetes.ClusterState) error {
	logger, ctx := log.LoggerFor(ctx)

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Wait for Knative Eventing components to be ready
	logger.Info("Waiting for Knative Eventing components to be ready...")
	if err := waitForDeployments(ctx, cluster, "knative-eventing", 5*time.Minute); err != nil {
		return fmt.Errorf("Knative Eventing components not ready: %w", err)
	}

	logger.Info("Knative Eventing is ready")
	return nil
}

// deployKnativeService deploys the knative service under test
func deployKnativeService(ctx context.Context) (context.Context, error) {
	logger, ctx := log.LoggerFor(ctx)

	k := testenv.FetchState[KnativeState](ctx)
	if k == nil {
		// Initialize knative state if not found
		k = &KnativeState{
			servingInstalled:  true,
			eventingInstalled: true,
			serviceDeployed:   false,
		}
		var err error
		ctx, err = testenv.SetupState(ctx, &k)
		if err != nil {
			return ctx, err
		}
	}

	if k.serviceDeployed {
		logger.Info("Knative service already deployed")
		return ctx, nil
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return ctx, fmt.Errorf("cluster not initialized")
	}

	// Get the working namespace
	namespace, err := getWorkingNamespace(ctx)
	if err != nil {
		return ctx, fmt.Errorf("failed to get working namespace: %w", err)
	}

	logger.Infof("Deploying knative service to namespace %s", namespace)

	// Deploy the knative service
	err = deployService(ctx, cluster, namespace)
	if err != nil {
		return ctx, fmt.Errorf("failed to deploy knative service: %w", err)
	}

	// Wait for service to be ready
	err = waitForServiceReady(ctx, cluster, namespace)
	if err != nil {
		return ctx, fmt.Errorf("knative service not ready: %w", err)
	}

	k.serviceDeployed = true
	k.serviceURL = fmt.Sprintf("http://conforma-knative-service.%s.svc.cluster.local", namespace)

	logger.Infof("Knative service deployed successfully to %s", k.serviceURL)
	return ctx, nil
}

// createDefaultBroker creates a default Broker in the specified namespace for event routing
func createDefaultBroker(ctx context.Context, cluster *kubernetes.ClusterState, namespace string) error {
	logger, ctx := log.LoggerFor(ctx)
	logger.Infof("Creating default Broker in namespace %s", namespace)

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return fmt.Errorf("REST mapper not available")
	}

	// Create the Broker resource
	broker := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "eventing.knative.dev/v1",
			"kind":       "Broker",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"config": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"name":       "config-br-default-channel",
					"namespace":  "knative-eventing",
				},
			},
		},
	}

	// Set the GVK
	gvk := schema.GroupVersionKind{
		Group:   "eventing.knative.dev",
		Version: "v1",
		Kind:    "Broker",
	}
	broker.SetGroupVersionKind(gvk)

	// Map the GVK to a REST resource
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for Broker: %w", err)
	}

	// Create the broker
	_, err = dynamicClient.Resource(mapping.Resource).Namespace(namespace).Apply(
		ctx,
		"default",
		broker,
		metav1.ApplyOptions{
			FieldManager: "knative-acceptance-test",
			Force:        true,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create Broker: %w", err)
	}

	logger.Infof("Default Broker created successfully in namespace %s", namespace)

	// Wait for the Broker to be ready
	logger.Info("Waiting for Broker to be ready...")
	err = wait.PollImmediate(2*time.Second, 2*time.Minute, func() (bool, error) {
		brokerObj, err := dynamicClient.Resource(mapping.Resource).Namespace(namespace).Get(ctx, "default", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		// Check status conditions
		conditions, found, err := unstructured.NestedSlice(brokerObj.Object, "status", "conditions")
		if err != nil || !found {
			return false, nil
		}

		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(condMap, "type")
			condStatus, _, _ := unstructured.NestedString(condMap, "status")
			if condType == "Ready" && condStatus == "True" {
				logger.Info("Broker is ready")
				return true, nil
			}
		}

		return false, nil
	})

	if err != nil {
		logger.Warn("Broker may not be ready yet, continuing anyway")
		// Don't fail if broker isn't ready immediately - the service can still be deployed
		// and the broker may become ready shortly after
	}

	return nil
}

// deployService deploys the knative service using ko and kustomize
func deployService(ctx context.Context, cluster *kubernetes.ClusterState, namespace string) error {
	logger, ctx := log.LoggerFor(ctx)
	logger.Info("Deploying knative service...")

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// 1. Build and push the image using ko
	logger.Info("Building and pushing image with ko...")
	imageRef, err := buildAndPushImage(ctx)
	if err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}
	logger.Infof("Image built and pushed: %s", imageRef)

	// 2. Render the configuration using kustomize (config/base)
	logger.Info("Rendering service configuration with kustomize...")
	yamlData, err := kustomize.RenderPath(path.Join("config", "base"))
	if err != nil {
		return fmt.Errorf("failed to render kustomize configuration: %w", err)
	}

	// 3. Replace ko:// image reference with the actual built image and set namespace
	yamlData, err = replaceImageAndNamespace(yamlData, imageRef, namespace)
	if err != nil {
		return fmt.Errorf("failed to replace image reference: %w", err)
	}

	// 4. Create a default Broker in the namespace for event routing
	logger.Info("Creating default Broker for event routing...")
	if err := createDefaultBroker(ctx, cluster, namespace); err != nil {
		return fmt.Errorf("failed to create default broker: %w", err)
	}

	// 5. Apply the configuration to the cluster
	logger.Info("Applying service configuration to cluster...")
	if err := applyYAMLData(ctx, cluster, yamlData); err != nil {
		return fmt.Errorf("failed to apply service configuration: %w", err)
	}

	logger.Info("Service deployed successfully")
	return nil
}

// waitForServiceReady waits for the knative service to be ready
func waitForServiceReady(ctx context.Context, cluster *kubernetes.ClusterState, namespace string) error {
	logger, ctx := log.LoggerFor(ctx)
	logger.Info("Waiting for service to be ready...")

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	deploymentName := "conforma-knative-service"

	return wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		// Check if the Deployment is ready
		client := clusterImpl.Dynamic()
		if client == nil {
			return false, fmt.Errorf("dynamic client not available")
		}

		deployment, err := getDeployment(ctx, cluster, namespace, deploymentName)
		if err != nil {
			logger.Infof("Deployment not found yet: %v", err)
			return false, nil
		}

		// Log detailed status information
		logger.Infof("Deployment status: replicas=%d, readyReplicas=%d, availableReplicas=%d, unavailableReplicas=%d",
			deployment.Status.Replicas,
			deployment.Status.ReadyReplicas,
			deployment.Status.AvailableReplicas,
			deployment.Status.UnavailableReplicas)

		// Log deployment conditions
		for _, condition := range deployment.Status.Conditions {
			logger.Infof("Deployment condition: type=%s, status=%s, reason=%s, message=%s",
				condition.Type, condition.Status, condition.Reason, condition.Message)
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == v1.ConditionTrue {
				logger.Info("Service deployment is ready")
				return true, nil
			}
		}

		// Get pod status to understand why deployment isn't ready
		pods, err := getPodsForDeployment(ctx, cluster, namespace, deploymentName)
		if err != nil {
			logger.Infof("Failed to get pods: %v", err)
		} else {
			for _, pod := range pods {
				logger.Infof("Pod %s: phase=%s", pod.Name, pod.Status.Phase)
				for _, containerStatus := range pod.Status.ContainerStatuses {
					logger.Infof("  Container %s: ready=%t, restartCount=%d",
						containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)
					if containerStatus.State.Waiting != nil {
						logger.Infof("    Waiting: reason=%s, message=%s",
							containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
					}
					if containerStatus.State.Terminated != nil {
						logger.Infof("    Terminated: reason=%s, exitCode=%d, message=%s",
							containerStatus.State.Terminated.Reason,
							containerStatus.State.Terminated.ExitCode,
							containerStatus.State.Terminated.Message)
					}
				}
			}
		}

		logger.Info("Service deployment not ready yet...")
		return false, nil
	})
}

// checkServiceHealth verifies the service is responding to health checks
func checkServiceHealth(ctx context.Context) error {
	logger, ctx := log.LoggerFor(ctx)

	k := testenv.FetchState[KnativeState](ctx)
	if k == nil || !k.serviceDeployed {
		return fmt.Errorf("knative service not deployed")
	}

	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return fmt.Errorf("cluster not initialized")
	}

	// Get the working namespace
	namespace, err := getWorkingNamespace(ctx)
	if err != nil {
		return fmt.Errorf("failed to get working namespace: %w", err)
	}

	// Get the service deployment
	serviceName := "conforma-knative-service"
	deployment, err := getDeployment(ctx, cluster, namespace, serviceName)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check if all replicas are ready
	if deployment.Status.ReadyReplicas == 0 {
		return fmt.Errorf("no ready replicas found")
	}

	if deployment.Status.ReadyReplicas < deployment.Status.Replicas {
		return fmt.Errorf("not all replicas are ready: %d/%d",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
	}

	// Verify pods have passed readiness probe (which checks /health endpoint)
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == v1.ConditionTrue {
			logger.Info("Service health check passed")
			return nil
		}
	}

	return fmt.Errorf("service not healthy")
}

// Helper functions

// applyYAMLFromURL downloads YAML from a URL and applies it to the cluster
func applyYAMLFromURL(ctx context.Context, cluster *kubernetes.ClusterState, manifestURL string) error {
	logger, ctx := log.LoggerFor(ctx)
	logger.Infof("Downloading YAML from %s", manifestURL)

	// Validate URL before request to satisfy gosec G107
	parsedURL, err := url.Parse(manifestURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("invalid URL scheme: %s", parsedURL.Scheme)
	}

	// Download YAML content
	// #nosec G107 -- URL validated for scheme, used for downloading official Knative manifests
	resp, err := http.Get(manifestURL)
	if err != nil {
		return fmt.Errorf("failed to download YAML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download YAML: HTTP %d", resp.StatusCode)
	}

	yamlData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read YAML: %w", err)
	}

	return applyYAMLData(ctx, cluster, yamlData)
}

// applyYAMLData applies YAML data to the cluster
func applyYAMLData(ctx context.Context, cluster *kubernetes.ClusterState, yamlData []byte) error {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	mapper := clusterImpl.Mapper()
	if mapper == nil {
		return fmt.Errorf("REST mapper not available")
	}

	// Parse and apply each document in the YAML
	reader := util.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlData)))
	for {
		definition, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read YAML document: %w", err)
		}

		var obj unstructured.Unstructured
		if err = yaml.Unmarshal(definition, &obj); err != nil {
			return fmt.Errorf("failed to unmarshal YAML: %w", err)
		}

		// Skip empty documents
		if obj.Object == nil || len(obj.Object) == 0 {
			continue
		}

		// Get the REST mapping for this resource
		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
		if err != nil {
			return fmt.Errorf("failed to get REST mapping for %s: %w", obj.GroupVersionKind(), err)
		}

		// Get the resource interface
		var resourceInterface dynamic.ResourceInterface = dynamicClient.Resource(mapping.Resource)
		if mapping.Scope.Name() == "namespace" {
			namespace := obj.GetNamespace()
			if namespace == "" {
				namespace = "default"
			}
			resourceInterface = dynamicClient.Resource(mapping.Resource).Namespace(namespace)
		}

		// Apply the resource
		_, err = resourceInterface.Apply(ctx, obj.GetName(), &obj, metav1.ApplyOptions{
			FieldManager: "knative-acceptance-test",
			Force:        true,
		})
		if err != nil {
			return fmt.Errorf("failed to apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}

	return nil
}

// patchConfigMap patches a ConfigMap with new data
func patchConfigMap(ctx context.Context, cluster *kubernetes.ClusterState, namespace, name string, data map[string]string) error {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	// Get the ConfigMap
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	configMap, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update the data
	existingData, found, err := unstructured.NestedStringMap(configMap.Object, "data")
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap data: %w", err)
	}
	if !found {
		existingData = make(map[string]string)
	}

	// Merge new data
	for k, v := range data {
		existingData[k] = v
	}

	if err := unstructured.SetNestedStringMap(configMap.Object, existingData, "data"); err != nil {
		return fmt.Errorf("failed to set ConfigMap data: %w", err)
	}

	// Apply the updated ConfigMap
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	return nil
}

// waitForDeployments waits for all deployments in a namespace to be ready
func waitForDeployments(ctx context.Context, cluster *kubernetes.ClusterState, namespace string, timeout time.Duration) error {
	logger, ctx := log.LoggerFor(ctx)
	logger.Infof("Waiting for deployments in namespace %s", namespace)

	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		// List all deployments in the namespace
		deploymentList, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			logger.Infof("Failed to list deployments: %v", err)
			return false, nil
		}

		if len(deploymentList.Items) == 0 {
			logger.Infof("No deployments found in namespace %s yet", namespace)
			return false, nil
		}

		allReady := true
		for _, item := range deploymentList.Items {
			deployment := &appsv1.Deployment{}
			err := convertUnstructuredToDeployment(&item, deployment)
			if err != nil {
				logger.Infof("Failed to convert deployment: %v", err)
				continue
			}

			ready := false
			for _, condition := range deployment.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable && condition.Status == v1.ConditionTrue {
					ready = true
					break
				}
			}

			if !ready {
				logger.Infof("Deployment %s not ready yet", deployment.Name)
				allReady = false
			}
		}

		return allReady, nil
	})
}

// getDeployment retrieves a deployment from the cluster
func getDeployment(ctx context.Context, cluster *kubernetes.ClusterState, namespace, name string) (*appsv1.Deployment, error) {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return nil, fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return nil, fmt.Errorf("dynamic client not available")
	}

	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	deployment := &appsv1.Deployment{}
	if err := convertUnstructuredToDeployment(obj, deployment); err != nil {
		return nil, fmt.Errorf("failed to convert deployment: %w", err)
	}

	return deployment, nil
}

// convertUnstructuredToDeployment converts an unstructured object to a Deployment
func convertUnstructuredToDeployment(obj *unstructured.Unstructured, deployment *appsv1.Deployment) error {
	data, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, deployment)
}

// getPodsForDeployment retrieves pods for a deployment
func getPodsForDeployment(ctx context.Context, cluster *kubernetes.ClusterState, namespace, deploymentName string) ([]v1.Pod, error) {
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return nil, fmt.Errorf("cluster not initialized")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return nil, fmt.Errorf("dynamic client not available")
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	// List pods with label selector matching the deployment
	labelSelector := fmt.Sprintf("app=%s", deploymentName)
	podList, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pods := make([]v1.Pod, 0, len(podList.Items))
	for _, item := range podList.Items {
		pod := &v1.Pod{}
		data, err := item.MarshalJSON()
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, pod); err != nil {
			continue
		}
		pods = append(pods, *pod)
	}

	return pods, nil
}

// getWorkingNamespace retrieves the working namespace from the test context
func getWorkingNamespace(ctx context.Context) (string, error) {
	// The testState is stored in the kind package, we need to use a type assertion
	// to access it. First, try to get the value using reflection on the context.
	// For now, we'll use a simpler approach: check the testenv package for the state.

	// Import the kind package's testState indirectly through the cluster
	cluster := testenv.FetchState[kubernetes.ClusterState](ctx)
	if cluster == nil {
		return "", fmt.Errorf("cluster not initialized")
	}

	// The working namespace is created by calling CreateNamespace
	// It's stored in the kind.testState, but we can't access it directly from here
	// We'll need to add a method to the Cluster interface to get the namespace

	// For now, try to find a namespace with the knative-test- prefix
	// This is a workaround until we can properly expose the namespace
	clusterImpl := cluster.Cluster()
	if clusterImpl == nil {
		return "", fmt.Errorf("cluster implementation not available")
	}

	dynamicClient := clusterImpl.Dynamic()
	if dynamicClient == nil {
		return "", fmt.Errorf("dynamic client not available")
	}

	// List all namespaces and find the one with knative-test- prefix
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	namespaces, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list namespaces: %w", err)
	}

	for _, ns := range namespaces.Items {
		name := ns.GetName()
		if len(name) > 13 && name[:13] == "knative-test-" {
			return name, nil
		}
	}

	return "", fmt.Errorf("no working namespace found with knative-test- prefix")
}

// buildAndPushImage builds the ko image and pushes it to the registry
func buildAndPushImage(ctx context.Context) (string, error) {
	logger, _ := log.LoggerFor(ctx)

	// Get the registry URL from the context (this is localhost:port for external access)
	registryURL, err := registry.Url(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get registry URL: %w", err)
	}

	logger.Infof("Using registry: %s", registryURL)

	// Use ko to build and push the image
	// ko://github.com/conforma/knative-service/cmd/launch-taskrun
	importPath := "github.com/conforma/knative-service/cmd/launch-taskrun"

	// Set up environment for ko
	// KO_DOCKER_REPO should be just the registry host:port with a repository path
	koEnv := []string{
		fmt.Sprintf("KO_DOCKER_REPO=%s/knative-service", registryURL),
		"CGO_ENABLED=0",
		"GOFLAGS=-buildvcs=false",   // Disable VCS stamping for acceptance tests
		"HOME=" + os.Getenv("HOME"), // Ensure HOME is set for go build cache
	}

	// Build and push to the test registry
	// Use --bare to get simpler image names
	// Use --insecure-registry since our test registry doesn't have TLS
	cmd := exec.CommandContext(ctx, "ko", "build", "--bare", "--insecure-registry", importPath)
	cmd.Env = append(os.Environ(), koEnv...)
	cmd.Dir = "." // Ensure we're in the project root

	logger.Info("Running: ko build --bare --insecure-registry " + importPath)

	// Capture both stdout and stderr separately for better debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		logger.Errorf("ko build failed")
		logger.Errorf("stdout: %s", stdout.String())
		logger.Errorf("stderr: %s", stderr.String())
		return "", fmt.Errorf("failed to build image with ko: %w\nStdout: %s\nStderr: %s",
			err, stdout.String(), stderr.String())
	}

	// The output is the image reference (from stdout)
	imageRef := strings.TrimSpace(stdout.String())

	if imageRef == "" {
		return "", fmt.Errorf("ko build produced no output\nStderr: %s", stderr.String())
	}

	logger.Infof("Ko build output: %s", imageRef)

	// The registry is exposed as a NodePort, so pods in the Kind cluster can access it via 127.0.0.1:PORT
	// Using 127.0.0.1 instead of registry.image-registry.svc.cluster.local avoids DNS resolution issues
	// in Tekton Pipeline controller and other cluster components
	// Replace localhost with 127.0.0.1 for consistent resolution
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) == 2 && strings.HasPrefix(parts[0], "localhost:") {
		port := strings.TrimPrefix(parts[0], "localhost:")
		imageRef = fmt.Sprintf("127.0.0.1:%s/%s", port, parts[1])
	}

	logger.Infof("Image reference for cluster: %s", imageRef)

	return imageRef, nil
}

// replaceImageAndNamespace replaces ko:// references and sets the namespace in YAML
func replaceImageAndNamespace(yamlData []byte, imageRef, namespace string) ([]byte, error) {
	// Replace the ko:// image reference with the actual built image
	koImagePattern := "ko://github.com/conforma/knative-service/cmd/launch-taskrun"
	modifiedYAML := bytes.ReplaceAll(yamlData, []byte(koImagePattern), []byte(imageRef))

	// Parse each document and set the namespace
	var result []byte
	reader := util.NewYAMLReader(bufio.NewReader(bytes.NewReader(modifiedYAML)))

	for {
		definition, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read YAML document: %w", err)
		}

		var obj unstructured.Unstructured
		if err = yaml.Unmarshal(definition, &obj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
		}

		// Skip empty documents
		if obj.Object == nil || len(obj.Object) == 0 {
			continue
		}

		// Set the namespace only for namespaced resources (skip cluster-scoped resources)
		kind := obj.GetKind()
		if kind != "ClusterRole" && kind != "ClusterRoleBinding" {
			obj.SetNamespace(namespace)
		}

		// For ClusterRoleBinding, update the subject namespace
		if kind == "ClusterRoleBinding" {
			subjects, found, err := unstructured.NestedSlice(obj.Object, "subjects")
			if err == nil && found {
				for i, subj := range subjects {
					if subjMap, ok := subj.(map[string]interface{}); ok {
						subjMap["namespace"] = namespace
						subjects[i] = subjMap
					}
				}
				_ = unstructured.SetNestedSlice(obj.Object, subjects, "subjects")
			}
		}

		// For ApiServerSource, update the sink reference namespace
		if kind == "ApiServerSource" {
			sinkRef, found, err := unstructured.NestedMap(obj.Object, "spec", "sink", "ref")
			if err == nil && found {
				// Only set namespace if the ref doesn't already have one
				// Broker references need the namespace set
				sinkRef["namespace"] = namespace
				_ = unstructured.SetNestedField(obj.Object, sinkRef, "spec", "sink", "ref")
			}
		}

		// For Trigger, update the subscriber reference namespace if it exists
		if kind == "Trigger" {
			subscriberRef, found, err := unstructured.NestedMap(obj.Object, "spec", "subscriber", "ref")
			if err == nil && found {
				subscriberRef["namespace"] = namespace
				_ = unstructured.SetNestedField(obj.Object, subscriberRef, "spec", "subscriber", "ref")
			}
		}

		// Marshal back to YAML
		objYAML, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal YAML: %w", err)
		}

		result = append(result, []byte("---\n")...)
		result = append(result, objYAML...)
	}

	return result, nil
}

// AddStepsTo adds Knative-related steps to the scenario context
func AddStepsTo(sc *godog.ScenarioContext) {
	sc.Step(`^Knative is installed and configured$`, installKnative)
	sc.Step(`^the knative service is deployed$`, deployKnativeService)
	sc.Step(`^the knative service is healthy$`, checkServiceHealth)
}
