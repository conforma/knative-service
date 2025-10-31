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

package kind

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/phayes/freeport"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	util "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	w "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	k "sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/yaml"

	"github.com/conforma/knative-service/acceptance/kubernetes/types"
	"github.com/conforma/knative-service/acceptance/kustomize"
	"github.com/conforma/knative-service/acceptance/log"
	"github.com/conforma/knative-service/acceptance/registry"
	"github.com/conforma/knative-service/acceptance/testenv"
)

type key int

const testStateKey = key(0)

// cluster consumers, we wait for every consumer to stop using the cluster
// before we shutdown the cluster
var clusterGroup = sync.WaitGroup{}

// make sure we try to create the cluster only once
var create = sync.Once{}

// make sure we try to destroy the cluster only once
var destroy = sync.Once{}

// single instance of Kind cluster
var globalCluster *kindCluster

type testState struct {
	namespace string
}

func (t testState) Key() any {
	return testStateKey
}

const clusterConfiguration = `kind: ClusterConfiguration
apiServer:
  extraArgs:
    "service-node-port-range": "1-65535"` // the extra port range for accessing the image registry at the random port

// We pass the registry port to Kustomize via an environment variable, we spawn
// Kustomize from the process runnning the test, to prevent concurrency issues
// with many tests running more than one kustomization when we modify the
// environment we use this mutex
var envMutex = sync.Mutex{}

type kindCluster struct {
	name           string
	kubeconfigPath string
	registryPort   int32
	provider       *k.Provider
	config         *rest.Config
	client         *kubernetes.Clientset
	dynamic        dynamic.Interface
	mapper         meta.RESTMapper
}

func (k *kindCluster) Up(_ context.Context) bool {
	if k == nil || k.provider == nil || k.name == "" {
		return false
	}

	nodes, err := k.provider.ListNodes(k.name)

	return len(nodes) > 0 && err == nil
}

// Start creates a new randomly named Kind cluster and provisions it for use
func Start(givenCtx context.Context) (ctx context.Context, kCluster types.Cluster, err error) {
	logger, ctx := log.LoggerFor(givenCtx)
	defer func() {
		clusterGroup.Add(1)
		logger.Info("Registered with cluster group")
	}()

	create.Do(func() {
		logger.Info("Starting Kind cluster")

		var configDir string
		configDir, err = os.MkdirTemp("", "knative-service-acceptance.*")
		if err != nil {
			logger.Errorf("Unable to create temp directory: %v", err)
			return
		}

		var id *big.Int
		id, err = rand.Int(rand.Reader, big.NewInt(math.MaxUint32))
		if err != nil {
			logger.Errorf("Unable to generate random cluster id: %v", err)
			return
		}

		kCluster := kindCluster{
			name:     fmt.Sprintf("acceptance-%d", id.Uint64()),
			provider: k.NewProvider(k.ProviderWithLogger(logger)),
		}
		kCluster.kubeconfigPath = path.Join(configDir, "kubeconfig")

		defer func() {
			if err != nil {
				logger.Infof("An error occurred creating the cluster: %v", err)
				// an error happened we need to cleanup
				if err := kCluster.provider.Delete(kCluster.name, kCluster.kubeconfigPath); err != nil {
					logger.Infof("An error occurred creating deleting the cluster: %v", err)
				}
			}
		}()

		var port int
		if port, err = freeport.GetFreePort(); err != nil {
			logger.Errorf("Unable to determine a free port: %v", err)
			return
		} else {
			// Validate port range to prevent integer overflow
			if port < 0 || port > 65535 {
				logger.Errorf("Invalid port range: %d", port)
				err = fmt.Errorf("port out of valid range: %d", port)
				return
			}
			kCluster.registryPort = int32(port) // #nosec G115 - port validated to be within int32 range
		}

		// Use a stable node image version that works reliably with Kind v0.26.0
		// v1.31.0 is tested and compatible with Kind v0.26.0 and cgroup v2
		nodeImage := "kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865"

		if err = kCluster.provider.Create(kCluster.name,
			k.CreateWithV1Alpha4Config(&v1alpha4.Cluster{
				TypeMeta: v1alpha4.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "kind.x-k8s.io/v1alpha4",
				},
				Nodes: []v1alpha4.Node{
					{
						Role:  v1alpha4.ControlPlaneRole,
						Image: nodeImage,
						KubeadmConfigPatches: []string{
							clusterConfiguration,
						},
						// exposes the registry port to the host OS
						ExtraPortMappings: []v1alpha4.PortMapping{
							{
								ContainerPort: kCluster.registryPort,
								HostPort:      kCluster.registryPort,
								Protocol:      v1alpha4.PortMappingProtocolTCP,
								ListenAddress: "127.0.0.1",
							},
						},
					},
				},
			}),
			k.CreateWithKubeconfigPath(kCluster.kubeconfigPath)); err != nil {
			logger.Errorf("Unable launch the Kind cluster: %v", err)
			return
		}

		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		rules.ExplicitPath = kCluster.kubeconfigPath

		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil)

		if kCluster.config, err = clientConfig.ClientConfig(); err != nil {
			logger.Errorf("Unable get the client config: %v", err)
			return
		}

		if kCluster.dynamic, err = dynamic.NewForConfig(kCluster.config); err != nil {
			logger.Errorf("Unable get the dynamic client config: %v", err)
			return
		}

		if kCluster.client, err = kubernetes.NewForConfig(kCluster.config); err != nil {
			logger.Errorf("Unable get create k8s client: %v", err)
			return
		}

		discovery := discovery.NewDiscoveryClientForConfigOrDie(kCluster.config)

		var resources []*restmapper.APIGroupResources
		if resources, err = restmapper.GetAPIGroupResources(discovery); err != nil {
			logger.Errorf("Unable access API resources: %v", err)
			return
		} else {
			kCluster.mapper = restmapper.NewDiscoveryRESTMapper(resources)
		}

		var yaml []byte
		yaml, err = renderTestConfiguration(&kCluster)
		if err != nil {
			logger.Errorf("Unable to kustomize test configuration: %v", err)
			return
		}

		err = applyConfiguration(ctx, &kCluster, yaml)
		if err != nil {
			logger.Errorf("Unable apply cluster configuration: %v", err)
			return
		}

		// Install Knative components after base configuration
		// This is done separately to avoid kustomize remote resource merging issues
		logger.Info("Installing Knative components...")
		err = installKnativeComponents(ctx, &kCluster)
		if err != nil {
			logger.Errorf("Unable to install Knative components: %v", err)
			return
		}

		globalCluster = &kCluster

		logger.Info("Cluster started")
	})

	if err != nil {
		// the Once block above set the error
		logger.Error("Unable to start the cluster")
		return
	}

	if globalCluster == nil {
		// some other, not this one, goroutine's Once resulted in an error and
		// didn't set the globalCluster
		return ctx, nil, errors.New("no cluster available")
	}

	ctx, err = registry.Register(ctx, fmt.Sprintf("localhost:%d", globalCluster.registryPort))

	return ctx, globalCluster, err
}

// renderTestConfiguration renders the hack/test Kustomize directory into a
// multi-document YAML. The port for the cluster registry, needed to configure
// the k8s Service for it is passed via REGISTRY_PORT environment variable
func renderTestConfiguration(k *kindCluster) (yaml []byte, err error) {
	envMutex.Lock()
	if err := os.Setenv("REGISTRY_PORT", fmt.Sprint(k.registryPort)); err != nil {
		return nil, err
	}

	defer func() {
		_ = os.Unsetenv("REGISTRY_PORT") // ignore errors
		envMutex.Unlock()
	}()

	return kustomize.Render(path.Join("test"))
}

// applyConfiguration runs equivalent of kubectl apply for each document in the
// definitions YAML
func applyConfiguration(ctx context.Context, k *kindCluster, definitions []byte) (err error) {
	reader := util.NewYAMLReader(bufio.NewReader(bytes.NewReader(definitions)))
	for {
		var definition []byte
		definition, err = reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return
		}

		var obj unstructured.Unstructured
		if err = yaml.Unmarshal(definition, &obj); err != nil {
			return
		}

		var mapping *meta.RESTMapping
		if mapping, err = k.mapper.RESTMapping(obj.GroupVersionKind().GroupKind()); err != nil {
			return
		}

		var c dynamic.ResourceInterface = k.dynamic.Resource(mapping.Resource)
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			c = c.(dynamic.NamespaceableResourceInterface).Namespace(obj.GetNamespace())
		}

		_, err = c.Apply(ctx, obj.GetName(), &obj, metav1.ApplyOptions{FieldManager: "application/apply-patch"})
		if err != nil {
			return
		}
	}

	// Wait for essential deployments to be ready
	// Adjust namespaces based on what's deployed in hack/test
	err = waitForAvailableDeploymentsIn(ctx, k, "image-registry")

	return
}

// waitForAvailableDeploymentsIn makes sure that all deployments in the provided
// namespaces are available
func waitForAvailableDeploymentsIn(ctx context.Context, k *kindCluster, namespaces ...string) (err error) {
	for _, namespace := range namespaces {
		watcher := cache.NewListWatchFromClient(k.client.AppsV1().RESTClient(), "deployments", namespace, fields.Everything())

		a := newAvail()
		_, err = w.UntilWithSync(ctx, watcher, &appsv1.Deployment{}, nil, (&a).allAvailable)
	}

	return
}

// keeps track of what deployment is available, the available map is keyed by
// <namespace>/<name>, the value is either true - available, or false - not
// available
type avail struct {
	available map[string]bool
}

func newAvail() avail {
	return avail{
		available: map[string]bool{},
	}
}

// allAvailable is invoked by the watcher for each change to the object, the
// object's availability is tracked and if all objects are available true is
// returned, stopping the watcher
func (a *avail) allAvailable(event watch.Event) (bool, error) {
	deployment := event.Object.(*appsv1.Deployment)

	for _, condition := range deployment.Status.Conditions {
		namespace := deployment.GetNamespace()
		name := deployment.GetName()

		if condition.Type == appsv1.DeploymentAvailable {
			a.available[namespace+"/"+name] = condition.Status == v1.ConditionTrue
			break
		}
	}

	for _, available := range a.available {
		if !available {
			return false, nil
		}
	}

	return true, nil
}

func (k *kindCluster) KubeConfig(ctx context.Context) (string, error) {
	if bytes, err := os.ReadFile(k.kubeconfigPath); err != nil {
		return "", err
	} else {
		return string(bytes), err
	}
}

func (k *kindCluster) Stop(ctx context.Context) (context.Context, error) {
	logger, ctx := log.LoggerFor(ctx)

	if !k.Up(ctx) {
		logger.Log("[Stop] Cluster not up")
		return ctx, nil
	}

	// release cluster
	clusterGroup.Done()
	logger.Log("[Stop] Released cluster to group")

	return ctx, nil
}

func Destroy(ctx context.Context) {
	logger, _ := log.LoggerFor(ctx)
	destroy.Do(func() {
		if globalCluster == nil {
			logger.Log("[Destroy] Skipping global cluster destruction")
			return
		}
		logger.Log("[Destroy] Destroying global cluster")

		// wait for other cluster consumers to finish
		logger.Log("[Destroy] Waiting for all consumers to finish")
		clusterGroup.Wait()
		logger.Log("[Destroy] Last global cluster consumer finished")

		defer func() {
			kindDir := path.Join(globalCluster.kubeconfigPath, "..")
			if err := os.RemoveAll(kindDir); err != nil {
				panic(err)
			}
		}()

		// ignore error
		if err := globalCluster.provider.Delete(globalCluster.name, globalCluster.kubeconfigPath); err != nil {
			panic(err)
		}
		logger.Log("[Destroy] Destroyed global cluster")
	})
}

func (k *kindCluster) CreateNamespace(ctx context.Context) (context.Context, error) {
	t := &testState{}
	ctx, err := testenv.SetupState(ctx, &t)
	if err != nil {
		return ctx, err
	}

	if t.namespace != "" {
		// already created
		return ctx, nil
	}

	namespace, err := k.client.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "knative-test-",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return ctx, err
	}

	t.namespace = namespace.GetName()

	return ctx, nil
}

func (k *kindCluster) Registry(ctx context.Context) (string, error) {
	return fmt.Sprintf("registry.image-registry.svc.cluster.local:%d", k.registryPort), nil
}

func (k *kindCluster) Dynamic() dynamic.Interface {
	return k.dynamic
}

func (k *kindCluster) Mapper() meta.RESTMapper {
	return k.mapper
}

// installKnativeComponents installs Knative Serving and Eventing components
// This is done separately from kustomization to avoid duplicate CRD issues
func installKnativeComponents(ctx context.Context, k *kindCluster) error {
	logger, ctx := log.LoggerFor(ctx)
	knativeVersion := "v1.12.0"

	// Helper function to download and apply YAML from URL
	applyFromURL := func(url string) error {
		logger.Infof("Applying %s", url)
		resp, err := http.Get(url) // #nosec G107 - URL is controlled, pointing to official Knative releases
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download %s: HTTP %d", url, resp.StatusCode)
		}

		yamlData, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", url, err)
		}

		// Apply each document in the YAML
		reader := util.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlData)))
		for {
			definition, err := reader.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to read YAML document from %s: %w", url, err)
			}

			var obj unstructured.Unstructured
			if err = yaml.Unmarshal(definition, &obj); err != nil {
				return fmt.Errorf("failed to unmarshal YAML from %s: %w", url, err)
			}

			// Skip empty documents
			if obj.Object == nil || len(obj.Object) == 0 {
				continue
			}

			mapping, err := k.mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
			if err != nil {
				return fmt.Errorf("failed to get REST mapping for %s from %s: %w", obj.GroupVersionKind(), url, err)
			}

			var c dynamic.ResourceInterface = k.dynamic.Resource(mapping.Resource)
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				namespace := obj.GetNamespace()
				if namespace == "" {
					namespace = "default"
				}
				c = c.(dynamic.NamespaceableResourceInterface).Namespace(namespace)
			}

			_, err = c.Apply(ctx, obj.GetName(), &obj, metav1.ApplyOptions{
				FieldManager: "kind-acceptance-test",
				Force:        true,
			})
			if err != nil {
				return fmt.Errorf("failed to apply %s %s from %s: %w", obj.GetKind(), obj.GetName(), url, err)
			}
		}

		return nil
	}

	// Install Knative Serving
	logger.Info("Installing Knative Serving CRDs...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/serving/releases/download/knative-%s/serving-crds.yaml", knativeVersion)); err != nil {
		return err
	}

	// Wait a moment for the API server to register the new CRDs
	logger.Info("Waiting for CRDs to be registered...")
	time.Sleep(5 * time.Second)

	// Refresh the REST mapper after installing CRDs so it knows about the new resource types
	logger.Info("Refreshing REST mapper after installing Serving CRDs...")
	if err := refreshRESTMapper(ctx, k); err != nil {
		return fmt.Errorf("failed to refresh REST mapper: %w", err)
	}

	logger.Info("Installing Knative Serving core...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/serving/releases/download/knative-%s/serving-core.yaml", knativeVersion)); err != nil {
		return err
	}

	logger.Info("Installing Kourier networking...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/net-kourier/releases/download/knative-%s/kourier.yaml", knativeVersion)); err != nil {
		return err
	}

	// Configure Kourier as the ingress
	logger.Info("Configuring Knative Serving to use Kourier...")
	if err := patchConfigMapForKourier(ctx, k); err != nil {
		return fmt.Errorf("failed to configure Kourier: %w", err)
	}

	// Install Knative Eventing
	logger.Info("Installing Knative Eventing CRDs...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/eventing/releases/download/knative-%s/eventing-crds.yaml", knativeVersion)); err != nil {
		return err
	}

	// Wait a moment for the API server to register the new CRDs
	logger.Info("Waiting for CRDs to be registered...")
	time.Sleep(5 * time.Second)

	// Refresh the REST mapper after installing CRDs so it knows about the new resource types
	logger.Info("Refreshing REST mapper after installing Eventing CRDs...")
	if err := refreshRESTMapper(ctx, k); err != nil {
		return fmt.Errorf("failed to refresh REST mapper: %w", err)
	}

	logger.Info("Installing Knative Eventing core...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/eventing/releases/download/knative-%s/eventing-core.yaml", knativeVersion)); err != nil {
		return err
	}

	logger.Info("Installing in-memory channel...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/eventing/releases/download/knative-%s/in-memory-channel.yaml", knativeVersion)); err != nil {
		return err
	}

	logger.Info("Installing MT Channel Broker...")
	if err := applyFromURL(fmt.Sprintf("https://github.com/knative/eventing/releases/download/knative-%s/mt-channel-broker.yaml", knativeVersion)); err != nil {
		return err
	}

	// Wait for Knative components to be ready
	logger.Info("Waiting for Knative Serving to be ready...")
	if err := waitForAvailableDeploymentsIn(ctx, k, "knative-serving", "kourier-system"); err != nil {
		return fmt.Errorf("Knative Serving not ready: %w", err)
	}

	logger.Info("Waiting for Knative Eventing to be ready...")
	if err := waitForAvailableDeploymentsIn(ctx, k, "knative-eventing"); err != nil {
		return fmt.Errorf("Knative Eventing not ready: %w", err)
	}

	// Now that Knative Eventing is ready, apply the default broker
	logger.Info("Creating default broker...")
	if err := createDefaultBroker(ctx, k); err != nil {
		return fmt.Errorf("failed to create default broker: %w", err)
	}

	logger.Info("Knative components installed successfully")
	return nil
}

// patchConfigMapForKourier patches the Knative Serving config-network ConfigMap to use Kourier
func patchConfigMapForKourier(ctx context.Context, k *kindCluster) error {
	configMap, err := k.client.CoreV1().ConfigMaps("knative-serving").Get(ctx, "config-network", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get config-network ConfigMap: %w", err)
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	configMap.Data["ingress-class"] = "kourier.ingress.networking.knative.dev"

	_, err = k.client.CoreV1().ConfigMaps("knative-serving").Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update config-network ConfigMap: %w", err)
	}

	return nil
}

// createDefaultBroker creates a default Broker in the default namespace
func createDefaultBroker(ctx context.Context, k *kindCluster) error {
	broker := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "eventing.knative.dev/v1",
			"kind":       "Broker",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": "default",
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

	mapping, err := k.mapper.RESTMapping(broker.GroupVersionKind().GroupKind())
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for Broker: %w", err)
	}

	_, err = k.dynamic.Resource(mapping.Resource).Namespace("default").Apply(
		ctx,
		"default",
		broker,
		metav1.ApplyOptions{
			FieldManager: "kind-acceptance-test",
			Force:        true,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create default broker: %w", err)
	}

	return nil
}

// refreshRESTMapper refreshes the REST mapper to pick up newly installed CRDs
func refreshRESTMapper(ctx context.Context, k *kindCluster) error {
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(k.config)

	resources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return fmt.Errorf("failed to get API group resources: %w", err)
	}

	k.mapper = restmapper.NewDiscoveryRESTMapper(resources)
	return nil
}
