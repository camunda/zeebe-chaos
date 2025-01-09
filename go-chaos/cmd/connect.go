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
	"github.com/spf13/cobra"
	"github.com/camunda/zeebe-chaos/go-chaos/backend"
)

func AddConnectCmd(rootCmd *cobra.Command, flags *Flags) {
	var connect = &cobra.Command{
		Use:   "connect",
		Short: "Connect Zeebe nodes",
		Long:  `Connect all Zeebe nodes again, after they have been disconnected uses sub-commands to connect brokers, gateways, etc.`,
	}

	var connectBrokers = &cobra.Command{
		Use:   "brokers",
		Short: "Connect Zeebe Brokers",
		Long:  `Connect all Zeebe Brokers again, after they have been disconnected.`,
		Run: func(cmd *cobra.Command, args []string) {
			err := backend.ConnectBrokers(flags.kubeConfigPath, flags.namespace)
			ensureNoError(err)
		},
	}

	var connectGateway = &cobra.Command{
		Use:   "gateway",
		Short: "Connect Zeebe Gateway",
		Long:  `Connect all Zeebe Gateway again, after it has been disconnected.`,
		Run: func(cmd *cobra.Command, args []string) {
			err := backend.ConnectGateway(flags.kubeConfigPath, flags.namespace)
			ensureNoError(err)
		},
	}

	rootCmd.AddCommand(connect)
	connect.AddCommand(connectBrokers)
	connect.AddCommand(connectGateway)
}
