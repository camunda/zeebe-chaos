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

	v1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ShouldReturnTrueForRunningGatewayDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector(getSelfManagedGatewayLabels())
	require.NoError(t, err)
	k8Client.CreateDeploymentWithLabelsAndName(t, selector, "gateway")

	// when
	running, err := k8Client.checkIfGatewaysAreRunning()

	// then
	require.NoError(t, err)
	assert.Equal(t, true, running)
}

func Test_ShouldReturnTrueForRunningSaaSGatewayDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector(getSaasGatewayLabels())
	require.NoError(t, err)
	k8Client.createSaaSCRD(t)
	k8Client.CreateDeploymentWithLabelsAndName(t, selector, "gateway")

	// when
	running, err := k8Client.checkIfGatewaysAreRunning()

	// then
	require.NoError(t, err)
	assert.Equal(t, true, running)
}

func Test_ShouldReturnErrorForNonExistingDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()

	// when
	running, err := k8Client.checkIfGatewaysAreRunning()

	// then
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected to find standalone gateway deployment")
	assert.Equal(t, false, running)
}

func Test_ShouldReturnGatewayDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector(getSelfManagedGatewayLabels())
	require.NoError(t, err)
	k8Client.CreateDeploymentWithLabelsAndName(t, selector, "gateway")

	// when
	deployment, err := k8Client.getGatewayDeployment()

	// then
	require.NoError(t, err)
	assert.Equal(t, "gateway", deployment.Name)
}

func Test_ShouldReturnSaaSGatewayDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector(getSaasGatewayLabels())
	require.NoError(t, err)
	k8Client.createSaaSCRD(t)
	k8Client.CreateDeploymentWithLabelsAndName(t, selector, "gateway")

	// when
	deployment, err := k8Client.getGatewayDeployment()

	// then
	require.NoError(t, err)
	assert.Equal(t, "gateway", deployment.Name)
}

func Test_ShouldDeployWorkerDeployment(t *testing.T) {
	// given
	k8Client := CreateFakeClient()

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://testNamespace-zeebe-gateway:26500")
}

func Test_ShouldDeployWorkerDeploymentWithDifferentDockerImage(t *testing.T) {
	// given
	k8Client := CreateFakeClient()

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 1, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://testNamespace-zeebe-gateway:26500")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "testTag")
}

func Test_ShouldNotReturnErrorWhenWorkersAlreadyDeployed(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	_ = k8Client.CreateWorkerDeploymentDefault()

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
}

func Test_ShouldDeployWorkerInSaas(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://zeebe-service:26500")
}

func Test_ShouldDeployWorkerInSaasWithDifferentDockerImageTag(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 1, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://zeebe-service:26500")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "testTag")
}

func Test_ShouldDeployWorkerWithDifferentPollingDelay(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 50, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.worker.pollingDelay=50ms")
}

func Test_ShouldDeployWorkerWithDefaults(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://zeebe-service:26500")
	assert.Equal(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "gcr.io/zeebe-io/worker:zeebe")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.worker.pollingDelay=1ms")
}

func Test_ShouldDeployWorkerWithTolerationsForSaaS(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	podSpec := deploymentList.Items[0].Spec.Template.Spec
	assert.Equal(t, 1, len(podSpec.Tolerations))
	toleration := podSpec.Tolerations[0]

	assert.Equal(t, v1.TolerationOpEqual, toleration.Operator)
	assert.Equal(t, v1.TaintEffectNoSchedule, toleration.Effect)
	assert.Equal(t, "components.gke.io/camunda-managed-components", toleration.Key)
	assert.Equal(t, "true", toleration.Value)
}

func Test_ShouldDeployWorkerWithoutTolerationsForSM(t *testing.T) {
	// given
	k8Client := CreateFakeClient()

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	podSpec := deploymentList.Items[0].Spec.Template.Spec
	assert.Equal(t, 0, len(podSpec.Tolerations))
}

func mockedCredentials() *ClientCredentials {
	return &ClientCredentials{
		AuthServer:   "AuthServer",
		Audience:     "Audience",
		ClientId:     "ClientId",
		ClientSecret: "SuperSecret",
	}
}
