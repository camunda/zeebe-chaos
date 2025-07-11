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
	"context"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func AddPublishCmd(rootCmd *cobra.Command, flags *Flags) {
	publishCmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a message",
		Long:  `Publish a message to a certain partition.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			panicOnError(err)

			port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
			defer closeFn()

			zbClient, err := internal.CreateZeebeClient(port, makeClientCredentials(flags))
			panicOnError(err)
			defer zbClient.Close()

			topology, err := internal.GetTopology(zbClient)
			panicOnError(err)

			correlationKey, err := internal.FindCorrelationKeyForPartition(flags.partitionId, int(topology.PartitionsCount))
			panicOnError(err)

			internal.LogVerbose("Send message '%s', with correaltion key '%s' (ASCII: %d) ", flags.msgName, correlationKey, int(correlationKey[0]))

			messageResponse, err := zbClient.NewPublishMessageCommand().MessageName(flags.msgName).CorrelationKey(correlationKey).TimeToLive(time.Hour * 1).Send(context.TODO())
			partitionIdFromKey := internal.ExtractPartitionIdFromKey(messageResponse.Key)

			internal.LogInfo("Message was sent and returned key %d, which corresponds to partition: %d", messageResponse.Key, partitionIdFromKey)
		},
	}

	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().IntVar(&flags.partitionId, "partitionId", 1, "Specify the id of the partition")
	publishCmd.Flags().StringVar(&flags.msgName, "msgName", "msg", "Specify the name of the message, which should be published.")
}

func panicOnError(err error) {
	if err != nil {
		panic(err.Error())
	}
}
