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
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
)

func AddBrokersCommand(rootCmd *cobra.Command, flags *Flags) {
	var getZeebeBrokersCmd = &cobra.Command{
		Use:   "brokers",
		Short: "Print the name of the Zeebe broker pods",
		Long:  `Show all names of deployed Zeebe brokers, in the current kubernetes namespace.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			if err != nil {
				panic(err)
			}

			pods, err := k8Client.GetBrokerPodNames()
			if err != nil {
				panic(err)
			}

			for _, item := range pods {
				internal.LogInfo("%s", item)
			}
		},
	}

	rootCmd.AddCommand(getZeebeBrokersCmd)
}
