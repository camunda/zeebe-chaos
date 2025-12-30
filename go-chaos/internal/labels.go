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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func getSelfManagedZeebeStatefulSetLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/component": "core",
			"app.kubernetes.io/name":      "camunda-platform",
		},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func getSelfManagedZeebeStatefulSetLabelsBefore88() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/component": "zeebe-broker",
			"app.kubernetes.io/name":      "camunda-platform",
		},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func (c K8Client) getBrokerLabels() string {
	if c.SaaSEnv {
		return getSaasBrokerLabels()
	} else {
		return getSelfManagedBrokerLabels()
	}
}

func getSelfManagedBrokerLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/component": "zeebe-broker"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func getSaasBrokerLabels() string {
	// For backwards compatability the brokers kept the gateway labels, for a statefulset the labels are not modifiable
	// To still be able to distinguish the standalone gateway with the broker, the gateway got a new label.
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/app": "zeebe", "app.kubernetes.io/component": "gateway"},
	}

	return labels.Set(labelSelector.MatchLabels).String()
}

func getSelfManagedGatewayLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/component": "zeebe-gateway"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func getSaasGatewayLabels() string {
	// For backwards compatability the brokers kept the gateway labels, for a statefulset the labels are not modifiable
	// To still be able to distinguish the standalone gateway with the broker, the gateway got a new label.
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/app": "zeebe-gateway", "app.kubernetes.io/component": "standalone-gateway"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func getSaasGatewayServiceLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/app": "zeebe-gateway"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func (c K8Client) getGatewayLabels() string {
	if c.SaaSEnv {
		return getSaasGatewayLabels()
	} else {
		return getSelfManagedGatewayLabels()
	}
}

func (c K8Client) getGatewayServiceLabels() string {
	if c.SaaSEnv {
		return getSaasGatewayServiceLabels()
	} else {
		return getSelfManagedGatewayLabels()
	}
}

func getSelfManagedCoreLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/component": "core"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func (c K8Client) GetCoreLabels() string {
	return getSelfManagedCoreLabels()
}

func (c K8Client) getWorkerLabels() string {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "worker"},
	}
	return labels.Set(labelSelector.MatchLabels).String()
}

func (c K8Client) disableSaaSNamespaceSecurityLabel() error {
	ns, err := c.Clientset.CoreV1().Namespaces().Get(context.TODO(), c.GetCurrentNamespace(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	LogVerbose("Removing namespace label: 'pod-security.kubernetes.io/enforce' to allow further privileges.")
	delete(ns.Labels, "pod-security.kubernetes.io/enforce")

	_, err = c.Clientset.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
	return err
}
