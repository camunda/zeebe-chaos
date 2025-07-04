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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ShouldReturnSelfManagedStatefulSet(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector(getSelfManagedZeebeStatefulSetLabels())
	require.NoError(t, err)
	k8Client.CreateStatefulSetWithLabelsAndName(t, selector, "zeebe")

	// when
	statefulset, err := k8Client.GetZeebeStatefulSet()

	// then
	require.NoError(t, err)
	assert.NotNil(t, statefulset)
	assert.Equal(t, "zeebe", statefulset.Name)
}

func Test_ShouldReturnSaaSStatefulSet(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)
	k8Client.CreateStatefulSetWithLabelsAndName(t, &metav1.LabelSelector{}, "zeebe")

	// when
	statefulset, err := k8Client.GetZeebeStatefulSet()

	// then
	require.NoError(t, err)
	assert.NotNil(t, statefulset)
	assert.Equal(t, "zeebe", statefulset.Name)
}

func Test_ShouldReturnErrorForNonExistingStatefulSet(t *testing.T) {
	// given
	k8Client := CreateFakeClient()

	// when
	statefulset, err := k8Client.GetZeebeStatefulSet()

	// then
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not uniquely identify the stateful set for Zeebe")
	assert.Nil(t, statefulset)
}

func Test_ShouldReturnErrorForNonExistingStatefulSetInSaaS(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	_, err := k8Client.GetZeebeStatefulSet()

	// then
	require.Error(t, err)
	require.Contains(t, err.Error(), "statefulsets.apps \"zeebe\" not found")

	// API No longer returns nil pointer but empty initialized struct
	//assert.Nil(t, statefulset)

}
