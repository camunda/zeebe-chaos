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
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
)

func AddStressCmd(rootCmd *cobra.Command, flags *Flags) {
	stress := &cobra.Command{
		Use:   "stress",
		Short: "Put stress on a Zeebe node",
		Long:  `Put stress on a Zeebe node. Node can be choose from gateway or brokers. Stress can be of different kind: memory, io or CPU. The different stress types can be combined.`,
	}

	stressBroker := &cobra.Command{
		Use:   "broker",
		Short: "Put stress on a Zeebe Broker",
		Long:  `Put stress on a Zeebe Broker. Broker can be identified via ID or partition and role. Stress can be of different kinds: memory, io or CPU.`,
		Run: func(cmd *cobra.Command, args []string) {
			internal.Verbosity = Verbose
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
			defer closeFn()

			zbClient, err := internal.CreateZeebeClient(port)
			ensureNoError(err)
			defer zbClient.Close()

			pod := getBrokerPod(k8Client, zbClient, flags.nodeId, flags.partitionId, flags.role)
			internal.LogInfo("Put stress on %s", pod.Name)

			stressType := internal.StressType{CpuStress: flags.cpuStress, IoStress: flags.ioStress, MemStress: flags.memoryStress}
			err = internal.PutStressOnPod(k8Client, flags.timeoutSec, pod.Name, "zeebe", stressType)
			ensureNoError(err)
		},
	}

	stressGateway := &cobra.Command{
		Use:   "gateway",
		Short: "Put stress on a Zeebe Gateway",
		Long:  `Put stress on a Zeebe Gateway. Stress can be of different kinds: memory, io or CPU.`,
		Run: func(cmd *cobra.Command, args []string) {
			internal.Verbosity = Verbose
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			pod := getGatewayPod(k8Client)
			internal.LogInfo("Put stress on %s", pod.Name)

			stressType := internal.StressType{CpuStress: flags.cpuStress, IoStress: flags.ioStress, MemStress: flags.memoryStress}
			err = internal.PutStressOnPod(k8Client, flags.timeoutSec, pod.Name, "zeebe-gateway", stressType)
			ensureNoError(err)
		},
	}

	rootCmd.AddCommand(stress)
	stress.PersistentFlags().BoolVar(&flags.cpuStress, "cpu", true, "Specify whether CPU stress should put on the node")
	stress.PersistentFlags().BoolVar(&flags.memoryStress, "memory", false, "Specify whether memory stress should put on the node")
	stress.PersistentFlags().BoolVar(&flags.ioStress, "io", false, "Specify whether io stress should put on the node")
	stressBroker.PersistentFlags().StringVar(&flags.timeoutSec, "timeout", "30", "Specify how long the stress should be executed in seconds. Default: 30")

	// stress brokers
	stress.AddCommand(stressBroker)

	stressBroker.Flags().IntVar(&flags.nodeId, "nodeId", -1, "Specify the nodeId of the Broker")
	stressBroker.Flags().StringVar(&flags.role, "role", "LEADER", "Specify the partition role [LEADER, FOLLOWER] of the Broker")
	stressBroker.Flags().IntVar(&flags.partitionId, "partitionId", 1, "Specify the partition id of the Broker")

	stress.AddCommand(stressGateway)

}

func getBrokerPod(k8Client internal.K8Client, zbClient zbc.Client, brokerNodeId int, brokerPartitionId int, brokerRole string) *v1.Pod {
	var brokerPod *v1.Pod
	var err error
	if brokerNodeId >= 0 {
		brokerPod, err = internal.GetBrokerPodForNodeId(k8Client, int32(brokerNodeId))
		ensureNoError(err)
		internal.LogVerbose("Found Broker %s with node id %d.", brokerPod.Name, brokerNodeId)
	} else {
		brokerPod, err = internal.GetBrokerPodForPartitionAndRole(k8Client, zbClient, brokerPartitionId, brokerRole)
		ensureNoError(err)
		internal.LogVerbose("Found Broker %s as %s for partition %d.", brokerPod.Name, brokerRole, brokerPartitionId)
	}

	return brokerPod
}

func getGatewayPod(k8Client internal.K8Client) *v1.Pod {
	pods, err := k8Client.GetGatewayPods()
	ensureNoError(err)

	if pods != nil && len(pods.Items) > 0 {
		return &pods.Items[0]
	}

	panic(errors.New(fmt.Sprintf("Expected to find standalone gateway, but found nothing.")))
}
