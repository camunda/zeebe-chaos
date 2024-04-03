// Copyright 2023 Camunda Services GmbH
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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeebe-io/zeebe-chaos/go-chaos/internal"
)

func AddClusterCommands(rootCmd *cobra.Command, flags *Flags) {
	var clusterCommand = &cobra.Command{
		Use:   "cluster",
		Short: "Interact with the Cluster API",
		Long:  "Can be used to query cluster topology and to request dynamic scaling",
	}
	var statusCommand = &cobra.Command{
		Use:   "status",
		Short: "Queries the current cluster topology",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printCurrentTopology(flags)
		},
	}
	var waitCommand = &cobra.Command{
		Use:   "wait",
		Short: "Waits for a topology change to complete",
		RunE: func(cmd *cobra.Command, args []string) error {
			return portForwardAndWaitForChange(flags)
		},
	}
	var scaleCommand = &cobra.Command{
		Use:   "scale",
		Short: "Scales the cluster to the given size",
		RunE: func(cmd *cobra.Command, args []string) error {
			return scaleCluster(flags)
		},
	}
	var forceFailoverCommand = &cobra.Command{
		Use:   "forceFailover",
		Short: "Force scale down the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return forceFailover(flags)
		},
	}

	rootCmd.AddCommand(clusterCommand)
	clusterCommand.AddCommand(statusCommand)
	clusterCommand.AddCommand(waitCommand)
	clusterCommand.AddCommand(forceFailoverCommand)
	waitCommand.Flags().Int64Var(&flags.changeId, "changeId", 0, "The id of the change to wait for")
	clusterCommand.AddCommand(scaleCommand)
	scaleCommand.Flags().IntVar(&flags.brokers, "brokers", 0, "The amount of brokers to scale to")
	scaleCommand.Flags().Int32Var(&flags.replicationFactor, "replicationFactor", -1, "The new replication factor")
	scaleCommand.MarkFlagRequired("brokers")
	forceFailoverCommand.Flags().Int32Var(&flags.regions, "regions", 1, "The number of regions in the cluster")
	forceFailoverCommand.Flags().Int32Var(&flags.regionId, "regionId", 0, "The id of the region to failover to")
	forceFailoverCommand.MarkFlagRequired("regions")
	forceFailoverCommand.MarkFlagRequired("regionId")
}

func scaleCluster(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	ensureNoError(err)

	port, closePortForward := k8Client.MustGatewayPortForward(0, 9600)
	defer closePortForward()
	currentTopology, err := QueryTopology(port)
	ensureNoError(err)
	if currentTopology.PendingChange != nil {
		return fmt.Errorf("cluster is already scaling")
	}

	err = k8Client.PauseReconciliation()
	if err != nil {
		return err
	}

	if len(currentTopology.Brokers) > flags.brokers {
		_, err = scaleDownBrokers(k8Client, port, flags.brokers, flags.replicationFactor)
	} else if len(currentTopology.Brokers) < flags.brokers {
		_, err = scaleUpBrokers(k8Client, port, flags.brokers, flags.replicationFactor)
	} else {
		internal.LogInfo("cluster is already at size %d", flags.brokers)
		return nil
	}
	ensureNoError(err)

	return nil
}

func scaleUpBrokers(k8Client internal.K8Client, port int, brokers int, replicationFactor int32) (*ChangeResponse, error) {
	changeResponse, err := requestBrokerScaling(port, brokers, replicationFactor)
	ensureNoError(err)
	_, err = k8Client.ScaleZeebeCluster(brokers)
	ensureNoError(err)
	return changeResponse, nil
}

func scaleDownBrokers(k8Client internal.K8Client, port int, brokers int, replicationFactor int32) (*ChangeResponse, error) {
	changeResponse, err := requestBrokerScaling(port, brokers, replicationFactor)
	ensureNoError(err)

	// Wait for brokers to leave before scaling down
	timeout := time.Minute * 25
	err = waitForChange(port, changeResponse.ChangeId, timeout)
	ensureNoError(err)
	_, err = k8Client.ScaleZeebeCluster(brokers)

	ensureNoError(err)
	return changeResponse, nil
}

func requestBrokerScaling(port int, brokers int, replicationFactor int32) (*ChangeResponse, error) {
	brokerIds := make([]int32, brokers)
	for i := 0; i < brokers; i++ {
		brokerIds[i] = int32(i)
	}
	return sendScaleRequest(port, brokerIds, false, replicationFactor)
}

func sendScaleRequest(port int, brokerIds []int32, force bool, replicationFactor int32) (*ChangeResponse, error) {
	forceParam := "false"
	if force {
		forceParam = "true"
	}
	url := fmt.Sprintf("http://localhost:%d/actuator/cluster/brokers?force=%s", port, forceParam)
	if replicationFactor > 0 {
		url = url + fmt.Sprintf("&replicationFactor=%d", replicationFactor)
	}
	request, err := json.Marshal(brokerIds)
	if err != nil {
		return nil, err
	}
	internal.LogInfo("Requesting scaling %s with input  %s", url, request)
	resp, err := http.Post(url, "application/json", bytes.NewReader(request))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("scaling failed with code %d", resp.StatusCode)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var changeResponse ChangeResponse
	err = json.Unmarshal(response, &changeResponse)
	if err != nil {
		return nil, err
	}
	return &changeResponse, nil
}

func printCurrentTopology(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		panic(err)
	}

	port, closePortForward := k8Client.MustGatewayPortForward(0, 9600)
	defer closePortForward()

	topology, err := QueryTopology(port)
	if err != nil {
		return err
	}
	formatted, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		return err
	}
	internal.LogInfo("Current topology: %s", string(formatted))
	return nil
}

func portForwardAndWaitForChange(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		panic(err)
	}

	port, closePortForward := k8Client.MustGatewayPortForward(0, 9600)
	defer closePortForward()

	// Wait for shorter time. Retry and longer timeout can be configured in the chaos experiment description
	timeout := time.Minute * 5
	return waitForChange(port, flags.changeId, timeout)
}

func waitForChange(port int, changeId int64, timeout time.Duration) error {
	interval := time.Second * 5
	iterations := int(timeout / interval)
	for i := 0; i < int(iterations); i++ {
		topology, err := QueryTopology(port)
		if err != nil {
			internal.LogInfo("Failed to query topology: %s", err)
			continue
		}
		if changeId <= 0 {
			if topology.PendingChange != nil {
				changeId = topology.PendingChange.Id
			} else if topology.LastChange != nil {
				changeId = topology.LastChange.Id
			} else {
				internal.LogInfo("No change exists")
				return nil
			}
		}
		changeStatus := describeChangeStatus(topology, int64(changeId))
		switch changeStatus {
		case ChangeStatusCompleted:
			internal.LogInfo("Change %d completed successfully", changeId)
			return nil
		case ChangeStatusFailed:
			internal.LogInfo("Change %d failed with status %s", changeId, topology.LastChange.Status)
			return fmt.Errorf("change %d failed with status %s", changeId, topology.LastChange.Status)
		case ChangeStatusOutdated:
			internal.LogInfo("Change %d is outdated but was most likely completed successfully, latest change is %d", changeId, topology.LastChange.Id)
			return nil
		case ChangeStatusPending:
			competed := len(topology.PendingChange.Completed)
			pending := len(topology.PendingChange.Pending)
			total := competed + pending
			internal.LogInfo("Change %d is %s with %d/%d operations complete", changeId, topology.PendingChange.Status, competed, total)
		case ChangeStatusUnknown:
			internal.LogInfo("Change %d not yet started", changeId)
		}
		internal.LogVerbose("Waiting %s before checking again. Iteration %d out of %d", interval, i, iterations)
		time.Sleep(interval)
	}

	return fmt.Errorf("change %d did not complete within 25 minutes", changeId)
}

func forceFailover(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	ensureNoError(err)

	port, closePortForward := k8Client.MustGatewayPortForward(0, 9600)
	defer closePortForward()
	currentTopology, err := QueryTopology(port)
	ensureNoError(err)
	if currentTopology.PendingChange != nil {
		return fmt.Errorf("cluster is already scaling")
	}

	brokersInRegion := getBrokers(currentTopology, flags.regions, flags.regionId)

	changeResponse, err := sendScaleRequest(port, brokersInRegion, true, -1)
	ensureNoError(err)

	timeout := time.Minute * 5
	err = waitForChange(port, changeResponse.ChangeId, timeout)
	ensureNoError(err)

	return nil
}

func getBrokers(topology *CurrentTopology, regions int32, regionId int32) []int32 {
	brokersInRegion := make([]int32, 0)
	for _, b := range topology.Brokers {
		if b.Id%regions == regionId {
			brokersInRegion = append(brokersInRegion, b.Id)
		}
	}

	return brokersInRegion
}

type ChangeStatus string

const (
	ChangeStatusOutdated  ChangeStatus = "OUTDATED"
	ChangeStatusCompleted ChangeStatus = "COMPLETED"
	ChangeStatusFailed    ChangeStatus = "FAILED"
	ChangeStatusPending   ChangeStatus = "PENDING"
	ChangeStatusUnknown   ChangeStatus = "UNKNOWN"
)

func describeChangeStatus(topology *CurrentTopology, changeId int64) ChangeStatus {
	if topology.LastChange != nil && topology.LastChange.Id == changeId {
		if topology.LastChange.Status == "COMPLETED" {
			return ChangeStatusCompleted
		} else {
			return ChangeStatusFailed
		}
	} else if topology.LastChange != nil && topology.LastChange.Id > changeId {
		return ChangeStatusOutdated
	} else if topology.PendingChange != nil && topology.PendingChange.Id == changeId {
		return ChangeStatusPending
	} else {
		return ChangeStatusUnknown
	}
}

func QueryTopology(port int) (*CurrentTopology, error) {
	url := fmt.Sprintf("http://localhost:%d/actuator/cluster", port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("expected status code 200 but got %d", resp.StatusCode)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var topology CurrentTopology
	err = json.Unmarshal(body, &topology)
	if err != nil {
		return nil, err
	}
	return &topology, nil
}

type ChangeResponse struct {
	ChangeId         int64
	CurrentTopology  []BrokerState
	PlannedChanges   []Operation
	ExpectedTopology []BrokerState
}

type CurrentTopology struct {
	Version       int32
	Brokers       []BrokerState
	LastChange    *LastChange
	PendingChange *TopologyChange
}

type BrokerState struct {
	Id            int32
	State         string
	Version       int64
	LastUpdatedAt string
	Partitions    []PartitionState
}

type PartitionState struct {
	Id       int32
	State    string
	Priority int32
}

type LastChange struct {
	Id          int64
	Status      string
	StartedAt   string
	CompletedAt string
}

type TopologyChange struct {
	Id              int64
	Status          string
	StartedAt       string
	CompletedAt     string
	InternalVersion int64
	Completed       []Operation
	Pending         []Operation
}

type Operation struct {
	Operation   string
	BrokerId    int32
	PartitionId int32
	Priority    int32
}
