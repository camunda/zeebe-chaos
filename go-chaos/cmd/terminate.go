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

package cmd

import (
	"errors"
	"fmt"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func AddTerminateCommand(rootCmd *cobra.Command, flags *Flags) {
	terminateCmd := &cobra.Command{
		Use:   "terminate",
		Short: "Terminates a Zeebe node",
		Long:  `Terminates a Zeebe node, it can be chosen between: broker, gateway or a worker.`,
	}

	terminateBrokerCmd := &cobra.Command{
		Use:   "broker",
		Short: "Terminates a Zeebe broker",
		Long:  `Terminates a Zeebe broker with a certain role and given partition.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)
			gracePeriodSec := int64(0)
			if flags.all {
				restartBrokers(k8Client, "terminate", &gracePeriodSec)
			} else {
				brokerPod := restartBroker(k8Client, flags.nodeId, flags.partitionId, flags.role, &gracePeriodSec, makeClientCredentials(flags))
				internal.LogInfo("Terminated %s", brokerPod)
			}
		},
	}

	terminateGatewayCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Terminates a Zeebe gateway",
		Long:  `Terminates a Zeebe gateway.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)
			gracePeriodSec := int64(0)

			if flags.all {
				restartGateways(k8Client, "terminate", &gracePeriodSec)
			} else {
				gatewayPod := restartGateway(k8Client, &gracePeriodSec)
				internal.LogInfo("Restarted %s", gatewayPod)
			}
		},
	}

	terminateWorkerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Terminates a Zeebe worker",
		Long:  `Terminates a Zeebe worker.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)
			gracePeriodSec := int64(0)
			restartWorker(k8Client, flags.all, "Terminated", &gracePeriodSec)
		},
	}

	rootCmd.AddCommand(terminateCmd)

	terminateCmd.AddCommand(terminateBrokerCmd)
	terminateBrokerCmd.Flags().StringVar(&flags.role, "role", "LEADER", "Specify the partition role [LEADER, FOLLOWER]")
	terminateBrokerCmd.Flags().IntVar(&flags.partitionId, "partitionId", 1, "Specify the id of the partition")
	terminateBrokerCmd.Flags().IntVar(&flags.nodeId, "nodeId", -1, "Specify the nodeId of the Broker")
	terminateBrokerCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all brokers should be terminated")
	terminateBrokerCmd.MarkFlagsMutuallyExclusive("partitionId", "nodeId", "all")
	terminateBrokerCmd.MarkFlagsMutuallyExclusive("role", "all")

	terminateCmd.AddCommand(terminateGatewayCmd)
	terminateGatewayCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all gateways should be terminated")

	terminateCmd.AddCommand(terminateWorkerCmd)
	terminateWorkerCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all workers should be terminated")
}

// Restart a broker pod. Pod is identified either by nodeId or by partitionId and role.
// GracePeriod (in second) can be nil, which would mean using K8 default.
// Returns the broker which has been restarted
func restartBroker(k8Client internal.K8Client, nodeId int, partitionId int, role string, gracePeriod *int64, credentials *internal.ClientCredentials) string {
	port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
	defer closeFn()

	zbClient, err := internal.CreateZeebeClient(port, credentials)
	ensureNoError(err)
	defer zbClient.Close()

	brokerPod := getBrokerPod(k8Client, zbClient, nodeId, partitionId, role)
	err = k8Client.RestartPodWithGracePeriod(brokerPod.Name, gracePeriod)
	ensureNoError(err)

	return brokerPod.Name
}

// Restarts all brokers in the current namespace.
// GracePeriod (in second) can be nil, which would mean using K8 default.
func restartBrokers(k8Client internal.K8Client, actionName string, gracePeriod *int64) {
	brokerPodNames, err := k8Client.GetBrokerPodNames()
	ensureNoError(err)

	if len(brokerPodNames) <= 0 {
		panic(errors.New(fmt.Sprintf("Expected to find a Zeebe broker in namespace %s, but none found", k8Client.GetCurrentNamespace())))
	}

	for _, brokerPodName := range brokerPodNames {
		err = k8Client.RestartPodWithGracePeriod(brokerPodName, gracePeriod)
		ensureNoError(err)
		internal.LogInfo("%s %s", actionName, brokerPodName)
	}
}

// Restart a gateway pod. The pod is the first from a list of existing pods.
// GracePeriod (in second) can be nil, which would mean using K8 default.
// Returns the gateway which has been restarted
func restartGateway(k8Client internal.K8Client, gracePeriod *int64) string {
	gatewayPodNames, err := k8Client.GetGatewayPodNames()
	ensureNoError(err)

	if len(gatewayPodNames) <= 0 {
		panic(errors.New(fmt.Sprintf("Expected to find Zeebe gateway in namespace %s, but none found.", k8Client.GetCurrentNamespace())))
	}

	gatewayPod := gatewayPodNames[0]
	err = k8Client.RestartPodWithGracePeriod(gatewayPod, gracePeriod)
	ensureNoError(err)
	return gatewayPod
}

// Restarts all gateways in the current namespace.
// GracePeriod (in second) can be nil, which would mean using K8 default.
func restartGateways(k8Client internal.K8Client, actionName string, gracePeriod *int64) {
	gatewayPodNames, err := k8Client.GetGatewayPodNames()
	ensureNoError(err)

	if len(gatewayPodNames) <= 0 {
		panic(errors.New(fmt.Sprintf("Expected to find a Zeebe gateways in namespace %s, but none found", k8Client.GetCurrentNamespace())))
	}

	for _, gatewayPodName := range gatewayPodNames {
		err = k8Client.RestartPodWithGracePeriod(gatewayPodName, gracePeriod)
		ensureNoError(err)
		internal.LogInfo("%s %s", actionName, gatewayPodName)
	}
}

// Restart a worker pod. The pod is the first from a list of existing pods, if all is not specified.
// GracePeriod (in second) can be nil, which would mean using K8 default.
// The actionName specifies whether it was restarted or terminated to log the right thing.
func restartWorker(k8Client internal.K8Client, all bool, actionName string, gracePeriod *int64) {
	workerPods, err := k8Client.GetWorkerPods()
	ensureNoError(err)

	if workerPods == nil || len(workerPods.Items) <= 0 {
		panic(errors.New(fmt.Sprintf("Expected to find workers in namespace %s, but none found.", k8Client.GetCurrentNamespace())))
	}

	if all {
		for _, worker := range workerPods.Items {
			err = k8Client.RestartPodWithGracePeriod(worker.Name, gracePeriod)
			ensureNoError(err)
			internal.LogInfo("%s %s", actionName, worker.Name)
		}
	} else {
		workerPod := workerPods.Items[0]
		err = k8Client.RestartPodWithGracePeriod(workerPod.Name, gracePeriod)
		ensureNoError(err)

		internal.LogInfo("%s %s", actionName, workerPod.Name)
	}
}
