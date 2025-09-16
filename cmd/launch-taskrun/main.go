package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceclient "github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	tektontypedv1 "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/typed/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	coretypedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	gozap "go.uber.org/zap"
)

// --- Interfaces for testability ---
type K8sConfigMapGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error)
}

type K8sCoreV1 interface {
	ConfigMaps(namespace string) K8sConfigMapGetter
}

type K8sClient interface {
	CoreV1() K8sCoreV1
}

type TektonTaskRunCreator interface {
	Create(ctx context.Context, taskRun *tektonv1.TaskRun, opts metav1.CreateOptions) (*tektonv1.TaskRun, error)
}

type TektonV1 interface {
	TaskRuns(namespace string) TektonTaskRunCreator
}

type TektonClient interface {
	TektonV1() TektonV1
}

// --- Logger interface and zapLogger ---
type Logger interface {
	Info(msg string, fields ...gozap.Field)
	Error(err error, msg string, fields ...gozap.Field)
}

type zapLogger struct {
	l *gozap.Logger
}

func (z *zapLogger) Info(msg string, fields ...gozap.Field) { z.l.Info(msg, fields...) }
func (z *zapLogger) Error(err error, msg string, fields ...gozap.Field) {
	z.l.Error(msg, append(fields, gozap.Error(err))...)
}

// --- ConfigMap Cache ---
type configMapCache struct {
	mu    sync.RWMutex
	cache map[string]*cachedConfigMap
	ttl   time.Duration
}

type cachedConfigMap struct {
	config    *TaskRunConfig
	timestamp time.Time
}

func newConfigMapCache(ttl time.Duration) *configMapCache {
	return &configMapCache{
		cache: make(map[string]*cachedConfigMap),
		ttl:   ttl,
	}
}

func (c *configMapCache) get(key string) (*TaskRunConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, exists := c.cache[key]; exists {
		if time.Since(cached.timestamp) < c.ttl {
			return cached.config, true
		}
		// Cache expired, remove it
		delete(c.cache, key)
	}
	return nil, false
}

func (c *configMapCache) set(key string, config *TaskRunConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cachedConfigMap{
		config:    config,
		timestamp: time.Now(),
	}
}

// clear removes all entries from the cache
// This method is currently unused but kept for potential future use
//
//nolint:unused // Utility function kept for future use
func (c *configMapCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cachedConfigMap)
}

// --- Real implementations ---
type realK8sClient struct{ client *kubernetes.Clientset }

func (r *realK8sClient) CoreV1() K8sCoreV1 { return &realK8sCoreV1{client: r.client.CoreV1()} }

type realK8sCoreV1 struct{ client coretypedv1.CoreV1Interface }

func (r *realK8sCoreV1) ConfigMaps(ns string) K8sConfigMapGetter {
	return &realK8sConfigMapGetter{client: r.client.ConfigMaps(ns)}
}

type realK8sConfigMapGetter struct {
	client coretypedv1.ConfigMapInterface
}

func (r *realK8sConfigMapGetter) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	return r.client.Get(ctx, name, opts)
}

type realTektonClient struct{ client *tektonclientset.Clientset }

func (r *realTektonClient) TektonV1() TektonV1 { return &realTektonV1{client: r.client.TektonV1()} }

type realTektonV1 struct {
	client tektontypedv1.TektonV1Interface
}

func (r *realTektonV1) TaskRuns(ns string) TektonTaskRunCreator {
	return &realTektonTaskRunCreator{client: r.client.TaskRuns(ns)}
}

type realTektonTaskRunCreator struct {
	client tektontypedv1.TaskRunInterface
}

func (r *realTektonTaskRunCreator) Create(ctx context.Context, taskRun *tektonv1.TaskRun, opts metav1.CreateOptions) (*tektonv1.TaskRun, error) {
	return r.client.Create(ctx, taskRun, opts)
}

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
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              json.RawMessage `json:"spec,omitempty"`
}

type CloudEventData struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

type TaskRunConfig struct {
	PolicyConfiguration             string `json:"POLICY_CONFIGURATION"`
	PublicKey                       string `json:"PUBLIC_KEY"`
	RekorHost                       string `json:"REKOR_HOST"`
	IgnoreRekor                     string `json:"IGNORE_REKOR"`
	Strict                          string `json:"STRICT"`
	Info                            string `json:"INFO"`
	TufMirror                       string `json:"TUF_MIRROR"`
	SslCertDir                      string `json:"SSL_CERT_DIR"`
	CaTrustConfigmapName            string `json:"CA_TRUST_CONFIGMAP_NAME"`
	CaTrustConfigMapKey             string `json:"CA_TRUST_CONFIG_MAP_KEY"`
	ExtraRuleData                   string `json:"EXTRA_RULE_DATA"`
	SingleComponent                 string `json:"SINGLE_COMPONENT"`
	SingleComponentCustomResource   string `json:"SINGLE_COMPONENT_CUSTOM_RESOURCE"`
	SingleComponentCustomResourceNs string `json:"SINGLE_COMPONENT_CUSTOM_RESOURCE_NS"`
}

type Service struct {
	k8sClient     K8sClient
	tektonClient  TektonClient
	logger        Logger
	configMapName string
	configCache   *configMapCache
}

type ServiceConfig struct {
	ConfigMapName string
	CacheTTL      time.Duration
}

func NewServiceWithDependencies(k8s K8sClient, tekton TektonClient, logger Logger, config ServiceConfig) *Service {
	if config.ConfigMapName == "" {
		config.ConfigMapName = "taskrun-config"
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute // Default 5 minute TTL
	}
	return &Service{
		k8sClient:     k8s,
		tektonClient:  tekton,
		logger:        logger,
		configMapName: config.ConfigMapName,
		configCache:   newConfigMapCache(config.CacheTTL),
	}
}

func NewService(config ServiceConfig) (*Service, error) {
	var k8sConfig *rest.Config
	var err error
	k8sConfig, err = rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
		}
	}
	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}
	tektonClient, err := tektonclientset.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create tekton client: %w", err)
	}
	return NewServiceWithDependencies(
		&realK8sClient{client: k8sClient},
		&realTektonClient{client: tektonClient},
		&zapLogger{l: gozap.NewExample()},
		config,
	), nil
}

func (s *Service) handleCloudEvent(ctx context.Context, event cloudevents.Event) error {
	s.logger.Info("Received CloudEvent", gozap.String("type", event.Type()))
	var eventData CloudEventData
	if err := event.DataAs(&eventData); err != nil {
		return fmt.Errorf("failed to parse event data: %w", err)
	}
	if eventData.Kind != "Snapshot" || eventData.APIVersion != "appstudio.redhat.com/v1alpha1" {
		s.logger.Info("Ignoring resource", gozap.String("apiVersion", eventData.APIVersion), gozap.String("kind", eventData.Kind))
		return nil
	}
	s.logger.Info("Processing Snapshot", gozap.String("name", eventData.Metadata.Name), gozap.String("namespace", eventData.Metadata.Namespace))
	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventData.Metadata.Name,
			Namespace: eventData.Metadata.Namespace,
		},
		Spec: eventData.Spec,
	}
	return s.processSnapshot(ctx, snapshot)
}

func (s *Service) processSnapshot(ctx context.Context, snapshot *Snapshot) error {
	s.logger.Info("Starting to process snapshot", gozap.String("name", snapshot.Name), gozap.String("namespace", snapshot.Namespace))

	config, err := s.readConfigMap(ctx, snapshot.Namespace)
	if err != nil {
		s.logger.Error(err, "Failed to read configmap")
		return fmt.Errorf("failed to read configmap: %w", err)
	}
	s.logger.Info("Successfully read configmap", gozap.String("namespace", snapshot.Namespace))

	taskRun, err := s.createTaskRun(snapshot, config)
	if err != nil {
		s.logger.Error(err, "Failed to create taskrun")
		return fmt.Errorf("failed to create taskrun: %w", err)
	}
	s.logger.Info("Successfully created taskrun spec", gozap.String("taskrunName", taskRun.Name))

	// Add timeout for Tekton API call
	trCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	createdTaskRun, err := s.tektonClient.TektonV1().TaskRuns(snapshot.Namespace).Create(trCtx, taskRun, metav1.CreateOptions{})
	if err != nil {
		s.logger.Error(err, "Failed to create taskrun in cluster")
		return fmt.Errorf("failed to create taskrun in cluster: %w", err)
	}

	s.logger.Info("Successfully created TaskRun", gozap.String("name", createdTaskRun.Name), gozap.String("namespace", createdTaskRun.Namespace))
	return nil
}

func (s *Service) readConfigMap(ctx context.Context, namespace string) (*TaskRunConfig, error) {
	// Check cache first
	cachedConfig, found := s.configCache.get(namespace)
	if found {
		s.logger.Info("Using cached config for namespace", gozap.String("namespace", namespace))
		return cachedConfig, nil
	}

	// If not in cache, fetch from K8s
	configMap, err := s.k8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, s.configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s: %w", s.configMapName, err)
	}
	config := &TaskRunConfig{}
	if val, exists := configMap.Data["POLICY_CONFIGURATION"]; exists {
		config.PolicyConfiguration = val
	}
	if val, exists := configMap.Data["PUBLIC_KEY"]; exists {
		config.PublicKey = val
	}
	if val, exists := configMap.Data["IGNORE_REKOR"]; exists {
		config.IgnoreRekor = val
	}

	// Cache the fetched config
	s.configCache.set(namespace, config)
	s.logger.Info("Fetched and cached config for namespace", gozap.String("namespace", namespace))
	return config, nil
}

func (s *Service) createTaskRun(snapshot *Snapshot, config *TaskRunConfig) (*tektonv1.TaskRun, error) {
	specJSON, err := json.Marshal(snapshot.Spec)
	// log the specJSON
	s.logger.Info("SpecJSON", gozap.String("specJSON", string(specJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot spec: %w", err)
	}
	// Helper function to create ParamValue with validation
	createParamValue := func(value string) tektonv1.ParamValue {
		if value == "" {
			value = "true" // Default to "true" for empty values
		}
		return tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value}
	}

	params := []tektonv1.Param{
		{Name: "POLICY_CONFIGURATION", Value: createParamValue(config.PolicyConfiguration)},
		{Name: "PUBLIC_KEY", Value: createParamValue(config.PublicKey)},
		{Name: "IGNORE_REKOR", Value: createParamValue(config.IgnoreRekor)},
		{Name: "STRICT", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "true"}},
		{Name: "INFO", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "true"}},
		{Name: "show-successes", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "true"}},
		{Name: "WORKERS", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "1"}},
		{Name: "IMAGES", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: string(specJSON)}},
	}

	// Debug logging for all parameters
	for _, param := range params {
		s.logger.Info("TaskRun param", gozap.String("name", param.Name), gozap.String("type", string(param.Value.Type)), gozap.String("value", param.Value.StringVal))
	}

	// Debug logging for resolver parameters
	resolverParams := []tektonv1.Param{
		{Name: "bundle", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/conforma/tekton-task:latest"}},
		{Name: "name", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "verify-enterprise-contract"}},
	}
	for _, param := range resolverParams {
		s.logger.Info("Resolver param", gozap.String("name", param.Name), gozap.String("type", string(param.Value.Type)), gozap.String("value", param.Value.StringVal))
	}

	return &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("verify-enterprise-contract-%s-%d", snapshot.Name, time.Now().Unix()),
			Namespace: snapshot.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "conforma-verifier-listener",
				"app.kubernetes.io/instance":   snapshot.Name,
				"app.kubernetes.io/component":  "taskrun",
				"app.kubernetes.io/part-of":    "conforma-verifier",
				"app.kubernetes.io/managed-by": "conforma-verifier-listener",
			},
		},
		Spec: tektonv1.TaskRunSpec{
			TaskRef: &tektonv1.TaskRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "bundles",
					Params:   resolverParams,
				},
			},
			Params: params,
		},
	}, nil
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
	s.service.logger.Info("Starting server", gozap.String("port", s.port))
	return s.ceClient.StartReceiver(context.Background(), s.service.handleCloudEvent)
}

func main() {
	service, err := NewService(ServiceConfig{})
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
