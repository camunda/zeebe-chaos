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
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/camunda/zeebe/clients/go/v8/pkg/pb"
	"github.com/spf13/cobra"
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
)

func AddTopologyCmd(rootCmd *cobra.Command, flags *Flags) {

	var topologyCmd = &cobra.Command{
		Use:   "topology",
		Short: "Print the Zeebe topology deployed in the current namespace",
		Long:  `Shows the current Zeebe topology, in the current kubernetes namespace.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			if err != nil {
				panic(err)
			}

			port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
			defer closeFn()

			client, err := internal.CreateZeebeClient(port)
			if err != nil {
				panic(err)
			}

			response, err := client.NewTopologyCommand().Send(context.TODO())
			if err != nil {
				panic(err)
			}

			builder := strings.Builder{}
			writeTopologyToOutput(&builder, response)
			internal.LogInfo(builder.String())
		},
	}
	rootCmd.AddCommand(topologyCmd)
}

func writeTopologyToOutput(output io.Writer, response *pb.TopologyResponse) {
	writer := tabwriter.NewWriter(output, 10, 0, 2, ' ', tabwriter.Debug)
	addLineToWriter(writer, writeHeader(response.PartitionsCount))
	writeTopology(response, writer)
	writer.Flush()
}

func writeTopology(response *pb.TopologyResponse, writer *tabwriter.Writer) {
	sort.Slice(response.Brokers, func(i, j int) bool {
		return response.Brokers[i].NodeId < response.Brokers[j].NodeId
	})
	for _, broker := range response.Brokers {
		addLineToWriter(writer, createBrokerTopologyString(response.PartitionsCount, broker))
	}
}

func createBrokerTopologyString(partitionsCount int32, broker *pb.BrokerInfo) string {
	line := fmt.Sprintf("%d", broker.NodeId)
	for i := int32(1); i < partitionsCount+1; i++ {
		line = fmt.Sprintf("%s\t", line)
		for _, partition := range broker.Partitions {
			if partition.PartitionId == i {
				line = fmt.Sprintf("%s%s (%s)", line, partition.Role.String(), partition.Health.String())
				break
			}
		}
	}
	return line
}

func writeHeader(partitionsCount int32) string {
	line := fmt.Sprintf("Node")
	for i := int32(0); i < partitionsCount; i++ {
		line = fmt.Sprintf("%s\tPartition %d", line, i+1)
	}
	return line
}

func addLineToWriter(writer *tabwriter.Writer, line string) {
	_, err := fmt.Fprintln(writer, line)
	if err != nil {
		panic(err.Error())
	}
}
