// Copyright 2022 Camunda Services GmbH
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

package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func Test_shouldGetSelfManagedBrokerLabels(t *testing.T) {
	// given
	var expected = "app.kubernetes.io/component=zeebe-broker"

	// when
	actual := getSelfManagedBrokerLabels()

	// then
	assert.Equal(t, expected, actual, "Labels should be equal")
}

func Test_shouldGetSaasBrokerLabels(t *testing.T) {
	// given
	var expected = "app.kubernetes.io/app=zeebe,app.kubernetes.io/component=gateway"

	// when
	actual := getSaasBrokerLabels()

	// then
	assert.Equal(t, expected, actual, "Labels should be equal")
}

func Test_shouldGetSelfManagedGatewayLabels(t *testing.T) {
	// given
	var expected = "app.kubernetes.io/component=zeebe-gateway"

	// when
	actual := getSelfManagedGatewayLabels()

	// then
	assert.Equal(t, expected, actual, "Labels should be equal")
}

func Test_shouldGetSaasGatewayLabels(t *testing.T) {
	// given
	var expected = "app.kubernetes.io/app in (zeebe-gateway,camunda),app.kubernetes.io/component in (standalone-gateway, camunda-gateway)"

	// when
	actual := getSaasGatewayLabels()

	// then
	assert.Equal(t, expected, actual, "Labels should be equal")
}

func Test_shouldGetSaasGatewayServiceLabels(t *testing.T) {
	// given
	var expected = "app.kubernetes.io/app in (zeebe-gateway,camunda)"

	// when
	actual := getSaasGatewayServiceLabels()

	// then
	assert.Equal(t, expected, actual, "Labels should be equal")
}

func Test_shouldRemoveNamespaceLabel(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSNamespace(t)

	// when
	err := k8Client.disableSaaSNamespaceSecurityLabel()

	// then
	require.NoError(t, err)
	namespace, err := k8Client.Clientset.CoreV1().Namespaces().Get(context.TODO(), k8Client.GetCurrentNamespace(), metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, namespace.Labels)
}
