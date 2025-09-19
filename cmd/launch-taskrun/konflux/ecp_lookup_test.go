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

package konflux

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gozap "go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	t *testing.T
}

func (m *mockLogger) Info(msg string, fields ...gozap.Field) {
	m.t.Logf("INFO: %s %v", msg, fields)
}

func (m *mockLogger) Warn(msg string, fields ...gozap.Field) {
	m.t.Logf("WARN: %s %v", msg, fields)
}

func (m *mockLogger) Error(err error, msg string, fields ...gozap.Field) {
	m.t.Logf("ERROR: %s: %v %v", msg, err, fields)
}

func TestFindECP_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	// Create test objects
	releasePlan := &ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rp",
			Namespace: "test-ns",
			Labels: map[string]string{
				"release.appstudio.openshift.io/releasePlanAdmission": "test-rpa",
			},
		},
		Spec: ReleasePlanSpec{
			Application: "test-app",
			Target:      "target-ns",
		},
	}

	rpa := &ReleasePlanAdmission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rpa",
			Namespace: "target-ns",
		},
		Spec: ReleasePlanAdmissionSpec{
			Policy: "custom-policy",
		},
	}

	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Spec: json.RawMessage(`{"application":"test-app"}`),
	}

	// Create fake client with test objects
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(releasePlan, rpa).
		Build()

	logger := &mockLogger{t: t}

	// Test successful ECP lookup
	ecp, err := FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)

	assert.NoError(t, err)
	assert.Equal(t, "target-ns/custom-policy", ecp)
}

func TestFindECP_DefaultPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	// Create test objects with empty policy (should use default)
	releasePlan := &ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rp",
			Namespace: "test-ns",
			Labels: map[string]string{
				"release.appstudio.openshift.io/releasePlanAdmission": "test-rpa",
			},
		},
		Spec: ReleasePlanSpec{
			Application: "test-app",
			Target:      "target-ns",
		},
	}

	rpa := &ReleasePlanAdmission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rpa",
			Namespace: "target-ns",
		},
		Spec: ReleasePlanAdmissionSpec{
			Policy: "", // Empty policy should use default
		},
	}

	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Spec: json.RawMessage(`{"application":"test-app"}`),
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(releasePlan, rpa).
		Build()

	logger := &mockLogger{t: t}

	ecp, err := FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)

	assert.NoError(t, err)
	assert.Equal(t, "target-ns/registry-standard", ecp)
}

func TestFindECP_NoReleasePlans(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Spec: json.RawMessage(`{"application":"test-app"}`),
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	logger := &mockLogger{t: t}

	_, err := FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no release plans found in namespace")
}

func TestFindECP_NoMatchingApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	// Create release plan for different application
	releasePlan := &ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rp",
			Namespace: "test-ns",
		},
		Spec: ReleasePlanSpec{
			Application: "different-app",
			Target:      "target-ns",
		},
	}

	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Spec: json.RawMessage(`{"application":"test-app"}`),
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(releasePlan).
		Build()

	logger := &mockLogger{t: t}

	_, err := FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no release plans found for application name: test-app")
}

func TestFindECP_RPANotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))

	releasePlan := &ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rp",
			Namespace: "test-ns",
			Labels: map[string]string{
				"release.appstudio.openshift.io/releasePlanAdmission": "missing-rpa",
			},
		},
		Spec: ReleasePlanSpec{
			Application: "test-app",
			Target:      "target-ns",
		},
	}

	snapshot := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Spec: json.RawMessage(`{"application":"test-app"}`),
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(releasePlan).
		Build()

	logger := &mockLogger{t: t}

	_, err := FindEnterpriseContractPolicy(context.Background(), cli, logger, snapshot)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get release plan admission")
}
