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

package k8s

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/conforma/knative-service/cmd/launch-taskrun/konflux"
)

func NewK8sConfig() (*rest.Config, error) {
	k8sConfig, err := rest.InClusterConfig()
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
	return k8sConfig, nil
}

func NewControllerRuntimeClient() (client.Client, error) {
	k8sConfig, err := NewK8sConfig()
	if err != nil {
		return nil, err
	}

	s := runtime.NewScheme()

	// Add the core Kubernetes types
	if err = scheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add core k8s types to scheme: %w", err)
	}

	// Add the custom stub Konflux types
	if err = konflux.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add ecp types to scheme: %w", err)
	}

	cli, err := client.New(k8sConfig, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return cli, nil
}
