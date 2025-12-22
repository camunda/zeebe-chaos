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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func CreateFakeClient() K8Client {
	scheme := runtime.NewScheme()
	groupVersionKind := schema.GroupVersionKind{Group: "cloud.camunda.io", Version: "v1alpha1", Kind: "zeebeclusters"}
	scheme.AddKnownTypeWithName(groupVersionKind, &unstructured.Unstructured{})

	k8Client := K8Client{Clientset: fake.NewSimpleClientset(),
		DynamicClient: dynamicFake.NewSimpleDynamicClient(scheme),
		ClientConfig:  &testClientConfig{namespace: "testNamespace"}}
	return k8Client
}

type testClientConfig struct {
	namespace          string
	namespaceSpecified bool
	err                error
}

func (c *testClientConfig) Namespace() (string, bool, error) {
	return c.namespace, c.namespaceSpecified, c.err
}

func (c *testClientConfig) RawConfig() (api.Config, error) {
	panic("implement me")
}

func (c *testClientConfig) ClientConfig() (*rest.Config, error) {
	panic("implement me")
}

func (c *testClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	panic("implement me")
}

func (c K8Client) CreatePodWithLabels(t *testing.T, selector *metav1.LabelSelector) {
	c.CreatePodWithLabelsAndName(t, selector, "testPod")
}

func (c K8Client) CreatePodWithLabelsAndName(t *testing.T, selector *metav1.LabelSelector, podName string) {
	_, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Create(context.TODO(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels, Name: podName},
		Spec:       v1.PodSpec{},
	}, metav1.CreateOptions{})

	require.NoError(t, err)
}

func (c K8Client) CreateBrokerPodsWithStatus(t *testing.T, selector *metav1.LabelSelector, podName string, podPhase v1.PodPhase, readyStatus bool) {
	_, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Create(context.TODO(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels, Name: podName},
		Spec:       v1.PodSpec{},
		Status: v1.PodStatus{
			Phase: podPhase,
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: readyStatus,
				},
			},
		},
	}, metav1.CreateOptions{})

	require.NoError(t, err)
}

func (c K8Client) CreateDeploymentWithLabelsAndName(t *testing.T, selector *metav1.LabelSelector, name string) {
	_, err := c.Clientset.AppsV1().Deployments(c.GetCurrentNamespace()).Create(context.TODO(), &v12.Deployment{
		ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels, Name: name},
		Spec:       v12.DeploymentSpec{},
		Status: v12.DeploymentStatus{
			UnavailableReplicas: 0,
			AvailableReplicas:   1,
		},
	}, metav1.CreateOptions{})

	require.NoError(t, err)
}

func (c *K8Client) createSaaSCRD(t *testing.T) {
	zeebeCrd := schema.GroupVersionResource{Group: "cloud.camunda.io", Version: "v1alpha1", Resource: "zeebeclusters"}
	obj := &unstructured.Unstructured{}
	namespace := c.GetCurrentNamespace()
	clusterId := strings.TrimSuffix(namespace, "-zeebe")
	obj.SetName(clusterId)

	_, err := c.DynamicClient.Resource(zeebeCrd).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	saas, err := c.isSaaSEnvironment()
	require.NoError(t, err)
	c.SaaSEnv = saas
}

func (c K8Client) CreateStatefulSetWithLabelsAndName(t *testing.T, selector *metav1.LabelSelector, name string) {
	_, err := c.Clientset.AppsV1().StatefulSets(c.GetCurrentNamespace()).Create(context.TODO(), &v12.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels, Name: name},
		Spec:       v12.StatefulSetSpec{},
		Status:     v12.StatefulSetStatus{},
	}, metav1.CreateOptions{})

	require.NoError(t, err)
}

func (c *K8Client) createSaaSNamespace(t *testing.T) {
	namespace := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   c.GetCurrentNamespace(),
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "true"},
		},
	}
	_, err := c.Clientset.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
	require.NoError(t, err)
}

// CreateServiceWithSelector creates a ClusterIP service with the given name, selector labels, and ports.
func (c K8Client) CreateServiceWithSelector(t *testing.T, name string, selector map[string]string, ports []v1.ServicePort) {
	if len(ports) == 0 {
		ports = []v1.ServicePort{{Port: 26500}}
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: selector},
		Spec:       v1.ServiceSpec{Selector: selector, Ports: ports, Type: v1.ServiceTypeClusterIP},
	}
	_, err := c.Clientset.CoreV1().Services(c.GetCurrentNamespace()).Create(context.TODO(), svc, metav1.CreateOptions{})
	require.NoError(t, err)
}
