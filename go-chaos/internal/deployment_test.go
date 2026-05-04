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

func setupGatewayServiceTarget(t *testing.T, k8Client K8Client, serviceName string) {
	selector := map[string]string{"app.kubernetes.io/component": "zeebe-gateway"}
	if k8Client.SaaSEnv {
		selector = map[string]string{"app.kubernetes.io/app": "camunda", "app.kubernetes.io/component": "camunda-gateway"}
	}

	k8Client.CreateServiceWithSelector(t, serviceName, selector, []v1.ServicePort{{Port: 26500}})

	_, err := k8Client.Clientset.CoreV1().Pods(k8Client.GetCurrentNamespace()).Create(context.TODO(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-pod", Labels: selector},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}

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

func Test_ShouldReturnTrueForRunningSaaSGatewayDeploymentPre8dot9(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector("app.kubernetes.io/app=zeebe-gateway,app.kubernetes.io/component=standalone-gateway")
	require.NoError(t, err)
	k8Client.createSaaSCRD(t)
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
	selector, err := metav1.ParseToLabelSelector("app.kubernetes.io/app=camunda,app.kubernetes.io/component=camunda-gateway")
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

func Test_ShouldReturnSaaSGatewayDeploymentPre8dot9(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	selector, err := metav1.ParseToLabelSelector("app.kubernetes.io/app=zeebe-gateway,app.kubernetes.io/component=standalone-gateway")
	require.NoError(t, err)
	k8Client.createSaaSCRD(t)
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
	selector, err := metav1.ParseToLabelSelector("app.kubernetes.io/app=camunda,app.kubernetes.io/component=camunda-gateway")
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
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://sm-gateway-service:26500")
}

func Test_ShouldDeployWorkerDeploymentWithDifferentDockerImage(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 1, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://sm-gateway-service:26500")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "testTag")
}

func Test_ShouldNotReturnErrorWhenWorkersAlreadyDeployed(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")
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
	setupGatewayServiceTarget(t, k8Client, "saas-gateway-service")

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://saas-gateway-service:26500")
}

func Test_ShouldDeployWorkerInSaasWithDifferentDockerImageTag(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)
	setupGatewayServiceTarget(t, k8Client, "saas-gateway-service")

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 1, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://saas-gateway-service:26500")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "testTag")
}

func Test_ShouldDeployWorkerWithDifferentPollingDelay(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)
	setupGatewayServiceTarget(t, k8Client, "saas-gateway-service")

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
	setupGatewayServiceTarget(t, k8Client, "saas-gateway-service")

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, "worker", deploymentList.Items[0].Name)
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.brokerUrl=http://saas-gateway-service:26500")
	assert.Equal(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Image, "gcr.io/zeebe-io/worker:zeebe")
	assert.Contains(t, deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env[0].Value, "-Dapp.worker.pollingDelay=1ms")
}

func Test_ShouldDeployWorkerWithTolerationsForSaaS(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	k8Client.createSaaSCRD(t)
	setupGatewayServiceTarget(t, k8Client, "saas-gateway-service")

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
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")

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

func envValue(envs []v1.EnvVar, name string) string {
	for _, e := range envs {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func Test_ShouldRenderSpringBootEnvVarsForSelfManaged(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")

	// when
	err := k8Client.CreateWorkerDeployment("testTag", 50, mockedCredentials())

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(deploymentList.Items))

	envs := deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env
	assert.Equal(t, "worker", envValue(envs, "SPRING_PROFILES_ACTIVE"))
	assert.Equal(t, "http://sm-gateway-service:26500", envValue(envs, "CAMUNDA_CLIENT_GRPC_ADDRESS"))
	assert.Equal(t, "http://sm-gateway-service:8080", envValue(envs, "CAMUNDA_CLIENT_REST_ADDRESS"))
	assert.Equal(t, "false", envValue(envs, "CAMUNDA_CLIENT_PREFER_REST_OVER_GRPC"))
	assert.Equal(t, "62s", envValue(envs, "CAMUNDA_CLIENT_REQUEST_TIMEOUT"))
	assert.Equal(t, "10", envValue(envs, "LOAD_TESTER_WORKER_CAPACITY"))
	assert.Equal(t, "50ms", envValue(envs, "LOAD_TESTER_WORKER_POLLING_DELAY"))
	assert.Equal(t, "50ms", envValue(envs, "LOAD_TESTER_WORKER_COMPLETION_DELAY"))

	// Auth credentials are carried by the legacy CAMUNDA_* env vars (consumed by the OLD
	// camunda-cloud SDK directly). ZEEBE_AUTH_METHOD must engage OIDC when AuthServer is
	// set; otherwise it defaults to "none" and no Authorization header is sent.
	assert.Equal(t, "AuthServer", envValue(envs, "CAMUNDA_AUTHORIZATION_SERVER_URL"))
	assert.Equal(t, "Audience", envValue(envs, "CAMUNDA_TOKEN_AUDIENCE"))
	assert.Equal(t, "ClientId", envValue(envs, "CAMUNDA_CLIENT_ID"))
	assert.Equal(t, "SuperSecret", envValue(envs, "CAMUNDA_CLIENT_SECRET"))
	assert.Equal(t, "oidc", envValue(envs, "ZEEBE_AUTH_METHOD"))
}

func Test_ShouldRenderAuthMethodNoneWhenNoCredentials(t *testing.T) {
	// given
	k8Client := CreateFakeClient()
	setupGatewayServiceTarget(t, k8Client, "sm-gateway-service")

	// when
	err := k8Client.CreateWorkerDeploymentDefault()

	// then
	require.NoError(t, err)
	deploymentList, err := k8Client.Clientset.AppsV1().Deployments(k8Client.GetCurrentNamespace()).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(deploymentList.Items))

	envs := deploymentList.Items[0].Spec.Template.Spec.Containers[0].Env
	assert.Equal(t, "none", envValue(envs, "ZEEBE_AUTH_METHOD"))
	assert.Equal(t, "", envValue(envs, "CAMUNDA_CLIENT_ID"))
}
