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

package backend

import (
	"errors"
	"fmt"

	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	v1 "k8s.io/api/core/v1"
)

func ConnectBrokers(kubeConfigPath string, namespace string) error {
	k8Client, err := internal.CreateK8Client(kubeConfigPath, namespace)
	if err != nil {
		return err
	}

	// No patch is need, since we expect that disconnect was executed before.
	// If not all fine and the pods are already connected.

	// We run connect on all nodes, since roles can have been changed in between so it is easier to run the commands on all nodes.

	podNames, err := k8Client.GetBrokerPodNames()
	if err != nil {
		return err
	}

	if len(podNames) <= 0 {
		errorMsg := fmt.Sprintf("Expected to find brokers in current namespace %s, but found nothing", k8Client.GetCurrentNamespace())
		return errors.New(errorMsg)
	}

	for _, pod := range podNames {
		err = internal.MakeIpReachableForPod(k8Client, pod)
		if err != nil {
			internal.LogVerbose("Error on connection Broker: %s. Error: %s", pod, err.Error())
		} else {
			internal.LogInfo("Connected %s again, removed unreachable routes.", pod)
		}
	}
	return nil
}

func ConnectGateway(kubeConfigPath string, namespace string) error {
	k8Client, err := internal.CreateK8Client(kubeConfigPath, namespace)
	if err != nil {
		return err
	}

	// No patch is need, since we expect that disconnect was executed before.
	// If not all fine and the pods are already connected.

	// We run connect on all nodes
	brokerPods, err := k8Client.GetBrokerPods()
	if err != nil {
		return err
	}

	if len(brokerPods.Items) <= 0 {
		errorMsg := fmt.Sprintf("Expected to find broker(s) in current namespace %s, but found nothing", k8Client.GetCurrentNamespace())
		return errors.New(errorMsg)
	}

	gatewayPod, err := getGatewayPod(k8Client)
	if err != nil {
		return err
	}

	for _, brokerPod := range brokerPods.Items {
		err = internal.MakeIpReachable(k8Client, gatewayPod.Name, brokerPod.Status.PodIP)
		if err != nil {
			internal.LogVerbose("Error on connection gateway: %s. Error: %s", gatewayPod.Name, err.Error())
		} else {
			internal.LogInfo("Connected %s again with %s, removed unreachable routes.", gatewayPod.Name, brokerPod.Name)
		}
	}
	return nil
}

type Broker struct {
	NodeId      int
	PartitionId int
	Role        string
}

type DisconnectBrokerCfg struct {
	Broker1Cfg   Broker
	Broker2Cfg   Broker
	OneDirection bool
}

func DisconnectBroker(kubeConfigPath string, namespace string, disconnectBrokerCfg DisconnectBrokerCfg) error {
	k8Client, err := prepareBrokerDisconnect(kubeConfigPath, namespace)

	zbClient, closeFn, err := ConnectToZeebeCluster(k8Client)
	if err != nil {
		return err
	}
	defer closeFn()

	broker1 := disconnectBrokerCfg.Broker1Cfg
	broker1Pod, err := getBrokerPod(k8Client, zbClient, broker1.NodeId, broker1.PartitionId, broker1.Role)
	if err != nil {
		return err
	}

	broker2 := disconnectBrokerCfg.Broker2Cfg
	broker2Pod, err := getBrokerPod(k8Client, zbClient, broker2.NodeId, broker2.PartitionId, broker2.Role)
	if err != nil {
		return err
	}

	if broker1Pod.Name == broker2Pod.Name {
		internal.LogInfo("Expected to disconnect two DIFFERENT brokers %s and %s, but they are the same. Will do nothing.", broker1Pod.Name, broker2Pod.Name)
		return nil
	}

	return disconnectPods(k8Client, broker1Pod, broker2Pod, disconnectBrokerCfg.OneDirection)
}

type DisconnectGatewayCfg struct {
	DisconnectToAll bool
	OneDirection    bool
	BrokerCfg       Broker
}

func DisconnectGateway(kubeConfigPath string, namespace string, disconnectGatewayCfg DisconnectGatewayCfg) error {
	k8Client, zbClient, closeFn, err := prepareGatewayDisconnect(kubeConfigPath, namespace)
	if err != nil {
		return err
	}
	defer closeFn()

	gatewayPod, err := getGatewayPod(k8Client)

	if disconnectGatewayCfg.DisconnectToAll {
		pods, err := k8Client.GetBrokerPods()
		if err != nil {
			return err
		}

		for _, brokerPod := range pods.Items {
			err := disconnectPods(k8Client, gatewayPod, &brokerPod, disconnectGatewayCfg.OneDirection)
			if err != nil {
				return err
			}
		}
	} else {
		brokerCfg := disconnectGatewayCfg.BrokerCfg
		broker2Pod, err := getBrokerPod(k8Client, zbClient, brokerCfg.NodeId, brokerCfg.PartitionId, brokerCfg.Role)
		if err != nil {
			return err
		}
		err = disconnectPods(k8Client, gatewayPod, broker2Pod, disconnectGatewayCfg.OneDirection)
		if err != nil {
			return err
		}
	}
	return nil
}

func prepareGatewayDisconnect(kubeConfigPath string, namespace string) (internal.K8Client, zbc.Client, func(), error) {
	k8Client, err := prepareBrokerDisconnect(kubeConfigPath, namespace)
	if err != nil {
		return k8Client, nil, nil, err
	}

	internal.LogVerbose("Patched deployment")

	err = k8Client.AwaitReadiness()
	if err != nil {
		return internal.K8Client{}, nil, nil, err
	}

	zbClient, closeFn, err := ConnectToZeebeCluster(k8Client)
	if err != nil {
		return internal.K8Client{}, nil, nil, err
	}

	return k8Client, zbClient, closeFn, nil
}

func prepareBrokerDisconnect(kubeConfigPath string, namespace string) (internal.K8Client, error) {
	k8Client, err := internal.CreateK8Client(kubeConfigPath, namespace)
	if err != nil {
		return internal.K8Client{}, err
	}

	err = k8Client.PauseReconciliation()
	if err != nil {
		return internal.K8Client{}, err
	}

	internal.LogVerbose("Patched statefulset")
	return k8Client, nil
}

func getGatewayPod(k8Client internal.K8Client) (*v1.Pod, error) {
	pods, err := k8Client.GetGatewayPods()
	if err != nil {
		return nil, err
	}

	if pods != nil && len(pods.Items) > 0 {
		return &pods.Items[0], nil
	}

	errorMsg := fmt.Sprintf("Expected to find standalone gateway, but found nothing.")
	return nil, errors.New(errorMsg)
}

func getBrokerPod(k8Client internal.K8Client, zbClient zbc.Client, brokerNodeId int, brokerPartitionId int, brokerRole string) (*v1.Pod, error) {
	var brokerPod *v1.Pod
	var err error
	if brokerNodeId >= 0 {
		brokerPod, err = internal.GetBrokerPodForNodeId(k8Client, int32(brokerNodeId))
		internal.LogVerbose("Found Broker %s with node id %d.", brokerPod.Name, brokerNodeId)
	} else {
		brokerPod, err = internal.GetBrokerPodForPartitionAndRole(k8Client, zbClient, brokerPartitionId, brokerRole)
		internal.LogVerbose("Found Broker %s as %s for partition %d.", brokerPod.Name, brokerRole, brokerPartitionId)
	}

	return brokerPod, err
}

func disconnectPods(k8Client internal.K8Client, firstPod *v1.Pod, secondPod *v1.Pod, oneDirection bool) error {
	err := internal.MakeIpUnreachableForPod(k8Client, secondPod.Status.PodIP, firstPod.Name)
	if err != nil {
		return err
	}

	internal.LogInfo("Disconnect %s from %s", firstPod.Name, secondPod.Name)

	if !oneDirection {
		err = internal.MakeIpUnreachableForPod(k8Client, firstPod.Status.PodIP, secondPod.Name)
		if err != nil {
			return err
		}
		internal.LogInfo("Disconnect %s from %s", secondPod.Name, firstPod.Name)
	}
	return nil
}
