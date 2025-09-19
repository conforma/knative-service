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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewK8sConfig(t *testing.T) {
	// Save original env vars to restore later
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalKubeconfig != "" {
			os.Setenv("KUBECONFIG", originalKubeconfig)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	t.Run("with KUBECONFIG env var", func(t *testing.T) {
		// Create a temporary kubeconfig file
		tmpFile, err := os.CreateTemp("", "kubeconfig-test-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// Write minimal valid kubeconfig content
		kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user: {}
`
		_, err = tmpFile.WriteString(kubeconfigContent)
		require.NoError(t, err)
		tmpFile.Close()

		// Set KUBECONFIG env var
		os.Setenv("KUBECONFIG", tmpFile.Name())
		os.Unsetenv("HOME") // Clear HOME to ensure KUBECONFIG is used

		config, err := NewK8sConfig()
		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "https://test-cluster", config.Host)
	})

	t.Run("with HOME/.kube/config fallback", func(t *testing.T) {
		// Create temporary home directory with .kube/config
		tmpDir, err := os.MkdirTemp("", "home-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		kubeDir := tmpDir + "/.kube"
		err = os.MkdirAll(kubeDir, 0755)
		require.NoError(t, err)

		kubeconfigPath := kubeDir + "/config"
		kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://home-cluster
  name: home-test
contexts:
- context:
    cluster: home-test
    user: home-test
  name: home-test
current-context: home-test
users:
- name: home-test
  user: {}
`
		err = os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0600)
		require.NoError(t, err)

		// Clear KUBECONFIG and set HOME
		os.Unsetenv("KUBECONFIG")
		os.Setenv("HOME", tmpDir)

		config, err := NewK8sConfig()
		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "https://home-cluster", config.Host)
	})

	t.Run("with invalid kubeconfig", func(t *testing.T) {
		// Create a temporary file with invalid kubeconfig content
		tmpFile, err := os.CreateTemp("", "invalid-kubeconfig-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString("invalid yaml content")
		require.NoError(t, err)
		tmpFile.Close()

		os.Setenv("KUBECONFIG", tmpFile.Name())
		os.Unsetenv("HOME")

		config, err := NewK8sConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to get kubeconfig")
	})

	t.Run("with no config files", func(t *testing.T) {
		// Clear both env vars and set HOME to non-existent directory
		os.Unsetenv("KUBECONFIG")
		os.Setenv("HOME", "/non/existent/directory")

		config, err := NewK8sConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to get kubeconfig")
	})
}

func TestNewControllerRuntimeClient(t *testing.T) {
	// Save original env vars to restore later
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalKubeconfig != "" {
			os.Setenv("KUBECONFIG", originalKubeconfig)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	t.Run("with valid kubeconfig", func(t *testing.T) {
		// Create a temporary kubeconfig file
		tmpFile, err := os.CreateTemp("", "kubeconfig-client-test-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// Write minimal valid kubeconfig content
		kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user: {}
`
		_, err = tmpFile.WriteString(kubeconfigContent)
		require.NoError(t, err)
		tmpFile.Close()

		// Set KUBECONFIG env var
		os.Setenv("KUBECONFIG", tmpFile.Name())
		os.Unsetenv("HOME")

		client, err := NewControllerRuntimeClient()
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("with invalid kubeconfig", func(t *testing.T) {
		// Set HOME to non-existent directory and clear KUBECONFIG
		os.Unsetenv("KUBECONFIG")
		os.Setenv("HOME", "/non/existent/directory")

		client, err := NewControllerRuntimeClient()
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}
