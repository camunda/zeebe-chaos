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
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func AddRestartCmd(rootCmd *cobra.Command, flags *Flags) {

	var restartCmd = &cobra.Command{
		Use:   "restart",
		Short: "Restarts a Zeebe node",
		Long:  `Restarts a Zeebe node, it can be chosen between: broker, gateway or a worker.`,
	}

	var restartBrokerCmd = &cobra.Command{
		Use:   "broker",
		Short: "Restarts a Zeebe broker",
		Long:  `Restarts a Zeebe broker with a certain role and given partition.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)
			if flags.all {
				restartBrokers(k8Client, "restart", nil)
			} else {
				brokerPod := restartBroker(k8Client, flags.nodeId, flags.partitionId, flags.role, nil)
				internal.LogInfo("Restarted %s", brokerPod)
			}
		},
	}

	var restartGatewayCmd = &cobra.Command{
		Use:   "gateway",
		Short: "Restarts a Zeebe gateway",
		Long:  `Restarts a Zeebe gateway.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			if flags.all {
				restartGateways(k8Client, "restart", nil)
			} else {
				gatewayPod := restartGateway(k8Client, nil)
				internal.LogInfo("Restarted %s", gatewayPod)
			}
		},
	}

	var restartWorkerCmd = &cobra.Command{
		Use:   "worker",
		Short: "Restart a Zeebe worker",
		Long:  `Restart a Zeebe worker.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)
			restartWorker(k8Client, flags.all, "Restarted", nil)
		},
	}

	rootCmd.AddCommand(restartCmd)
	restartCmd.AddCommand(restartBrokerCmd)
	restartBrokerCmd.Flags().StringVar(&flags.role, "role", "LEADER", "Specify the partition role [LEADER, FOLLOWER, INACTIVE]")
	restartBrokerCmd.Flags().IntVar(&flags.partitionId, "partitionId", 1, "Specify the id of the partition")
	restartBrokerCmd.Flags().IntVar(&flags.nodeId, "nodeId", -1, "Specify the nodeId of the Broker")
	restartBrokerCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all brokers should be restarted")
	restartBrokerCmd.MarkFlagsMutuallyExclusive("partitionId", "nodeId", "all")
	restartBrokerCmd.MarkFlagsMutuallyExclusive("role", "all")

	restartCmd.AddCommand(restartGatewayCmd)
	restartGatewayCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all gateways should be restarted")

	restartCmd.AddCommand(restartWorkerCmd)
	restartWorkerCmd.Flags().BoolVar(&flags.all, "all", false, "Specify whether all workers should be restarted")
}
