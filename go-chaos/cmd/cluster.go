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
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func AddClusterCommands(rootCmd *cobra.Command, flags *Flags) {
	clusterCommand := &cobra.Command{
		Use:   "cluster",
		Short: "Interact with the Cluster API",
		Long:  "Can be used to query cluster topology and to request dynamic scaling",
	}
	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Queries the current cluster topology",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printCurrentTopology(flags)
		},
	}
	waitCommand := &cobra.Command{
		Use:   "wait",
		Short: "Waits for a topology change to complete",
		RunE: func(cmd *cobra.Command, args []string) error {
			return portForwardAndWaitForChange(flags)
		},
	}
	scaleCommand := &cobra.Command{
		Use:   "scale",
		Short: "Scales the cluster to the given size",
		RunE: func(cmd *cobra.Command, args []string) error {
			return scaleCluster(flags)
		},
	}
	forceFailoverCommand := &cobra.Command{
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
	scaleCommand.Flags().Int32Var(&flags.partitionCount, "partitionCount", -1, "The number of partitions to scale to")
	scaleCommand.MarkFlagsOneRequired("brokers", "partitionCount", "replicationFactor")
	forceFailoverCommand.Flags().Int32Var(&flags.regions, "regions", 1, "The number of regions in the cluster")
	forceFailoverCommand.Flags().Int32Var(&flags.regionId, "regionId", 0, "The id of the region to failover to")
	forceFailoverCommand.Flags().IntVar(&flags.brokers, "brokers", -1, "The number of brokers")
	forceFailoverCommand.MarkFlagRequired("brokers")
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

	if flags.brokers > 0 && len(currentTopology.Brokers) > flags.brokers {
		_, err = scaleDownBrokers(k8Client, port, flags.brokers, flags.replicationFactor)
	} else if len(currentTopology.Brokers) < flags.brokers {
		_, err = scaleUpBrokers(k8Client, port, flags.brokers, flags.partitionCount, flags.replicationFactor)
	} else if currentTopology.partitionCount() <= flags.partitionCount || (flags.replicationFactor > 0 && flags.partitionCount <= 0) {
		_, err = scalePartitions(k8Client, port, flags.partitionCount, flags.replicationFactor)
	} else if currentTopology.partitionCount() >= flags.partitionCount {
		internal.LogInfo("Cannot scale down to %d or it's the same number of partitions", flags.partitionCount)
	} else {
		internal.LogInfo("cluster is already at size %d", flags.brokers)
		return nil
	}
	ensureNoError(err)

	return nil
}

func scalePartitions(k8Client internal.K8Client, port int, partitionCount int32, replicationFactor int32) (*ChangeResponse, error) {
	changeResponse, err := sendScaleRequest(port, nil, partitionCount, false, replicationFactor)
	ensureNoError(err)
	timeout := time.Minute * 5
	err = waitForChange(port, changeResponse.ChangeId, timeout)
	return changeResponse, nil
}

func scaleUpBrokers(k8Client internal.K8Client, port int, brokers int, partitionCount int32, replicationFactor int32) (*ChangeResponse, error) {
	changeResponse, err := requestBrokerScaling(port, brokers, partitionCount, replicationFactor)
	ensureNoError(err)
	_, err = k8Client.ScaleZeebeCluster(brokers)
	ensureNoError(err)
	timeout := time.Minute * 5
	err = waitForChange(port, changeResponse.ChangeId, timeout)
	return changeResponse, nil
}

func scaleDownBrokers(k8Client internal.K8Client, port int, brokers int, replicationFactor int32) (*ChangeResponse, error) {
	changeResponse, err := requestBrokerScaling(port, brokers, 0, replicationFactor)
	ensureNoError(err)

	// Wait for brokers to leave before scaling down
	timeout := time.Minute * 25
	err = waitForChange(port, changeResponse.ChangeId, timeout)
	ensureNoError(err)
	_, err = k8Client.ScaleZeebeCluster(brokers)

	ensureNoError(err)
	return changeResponse, nil
}

func requestBrokerScaling(port int, brokers int, partitionCount int32, replicationFactor int32) (*ChangeResponse, error) {
	brokerIds := make([]int32, brokers)
	for i := 0; i < brokers; i++ {
		brokerIds[i] = int32(i)
	}
	return sendScaleRequest(port, brokerIds, partitionCount, false, replicationFactor)
}

func sendScaleRequest(port int, brokerIds []int32, partitionCount int32, force bool, replicationFactor int32) (*ChangeResponse, error) {
	req := &ClusterPatchRequest{}
	if force {
		req = req.withBrokersToRemove(brokerIds)
	} else {
		req = req.withBrokers(brokerIds)
	}
	req = req.withPartitions(partitionCount, replicationFactor)

	changeResponse, err := sendPatchCluster(port, force, *req)
	if err == nil {
		return changeResponse, err
	}

	// if it failed due to 405, fall back
	var statErr *httpStatusError
	if errors.As(err, &statErr) && statErr.StatusCode == http.StatusMethodNotAllowed {
		internal.LogInfo("PATCH endpoint not supported (<8.6?), falling back to legacy endpoint…")
		return sendScaleRequestLegacy(port, brokerIds, force, replicationFactor)
	}

	return nil, err
}

func sendPatchCluster(port int, force bool, clusterRequest ClusterPatchRequest) (*ChangeResponse, error) {
	forceParam := "false"
	if force {
		forceParam = "true"
	}
	url := fmt.Sprintf("http://localhost:%d/actuator/cluster?force=%s", port, forceParam)
	requestBody, err := json.Marshal(clusterRequest)
	if err != nil {
		return nil, err
	}
	internal.LogInfo("Requesting scaling %s with input %s \n", url, string(requestBody))
	resp, err := sendHTTPJsonRequest(url, "PATCH", requestBody)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	internal.LogInfo("Response body %s", string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, &httpStatusError{StatusCode: resp.StatusCode}
	}

	var changeResponse ChangeResponse
	err = json.Unmarshal(body, &changeResponse)
	if err != nil {
		return nil, err
	}
	return &changeResponse, nil
}

func sendHTTPJsonRequest(url, method string, body []byte) (*http.Response, error) {
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// sendScaleRequestLegacy implements the old behavior:
//
//	POST http://…/actuator/cluster/brokers?force=…&replicationFactor=…
//	with a JSON body of brokerIds to support 8.5 chaos tests
func sendScaleRequestLegacy(port int, brokerIds []int32, force bool, replicationFactor int32) (*ChangeResponse, error) {
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
	if resp == nil {
		return nil, fmt.Errorf("received nil response from server")
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

	// Wait until we see 4 brokers in the topology
	timeout := time.Minute * 5
	interval := time.Second * 5
	iterations := int(timeout / interval)
	var currentTopology *CurrentTopology

	for i := 0; i < iterations; i++ {
		topology, err := QueryTopology(port)
		if err != nil {
			if i == iterations-1 {
				panic(err)
			}
			time.Sleep(interval)
			continue
		}

		if len(topology.Brokers) == flags.brokers {
			currentTopology = topology
			break
		}

		time.Sleep(interval)
	}

	if currentTopology == nil {
		return fmt.Errorf("timeout waiting for %d brokers in topology", flags.brokers)
	}

	if currentTopology.PendingChange != nil {
		return fmt.Errorf("cluster is already scaling")
	}

	brokersToRemove := getBrokersInOtherRegions(currentTopology, flags.regions, flags.regionId)

	changeResponse, err := sendScaleRequest(port, brokersToRemove, 0, true, -1)
	ensureNoError(err)

	err = waitForChange(port, changeResponse.ChangeId, timeout)
	ensureNoError(err)

	return nil
}

func getBrokersInOtherRegions(topology *CurrentTopology, regions int32, regionId int32) []int32 {
	brokersInOtherRegions := make([]int32, 0)
	for _, b := range topology.Brokers {
		if b.Id%regions != regionId {
			brokersInOtherRegions = append(brokersInOtherRegions, b.Id)
		}
	}

	return brokersInOtherRegions
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
	ClusterId     string
	Brokers       []BrokerState
	LastChange    *LastChange
	PendingChange *TopologyChange
	Routing       *RoutingState
}

func (topology *CurrentTopology) partitionCount() int32 {
	partitionCount := int32(0)

	for _, broker := range topology.Brokers {
		for _, partition := range broker.Partitions {
			partitionCount = max(partitionCount, partition.Id)
		}
	}
	return partitionCount
}

type BrokerState struct {
	Id         int32
	State      string
	Version    int64
	Partitions []PartitionState
}

type PartitionState struct {
	Id       int32
	State    string
	Priority int32
}

type LastChange struct {
	Id     int64
	Status string
}

type TopologyChange struct {
	Id              int64
	Status          string
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

type ClusterPatchRequest struct {
	Brokers    *ClusterPatchRequestBroker    `json:"brokers"`
	Partitions *ClusterPatchRequestPartition `json:"partitions"`
}

func (req *ClusterPatchRequest) withBrokers(brokerIds []int32) *ClusterPatchRequest {
	if brokerIds != nil && len(brokerIds) > 0 {
		req.Brokers = &ClusterPatchRequestBroker{Count: int32(len(brokerIds))}
	}
	return req
}

func (req *ClusterPatchRequest) withBrokersToRemove(brokerIds []int32) *ClusterPatchRequest {
	if len(brokerIds) > 0 {
		req.Brokers = &ClusterPatchRequestBroker{Remove: brokerIds}
	}
	return req
}

func (req *ClusterPatchRequest) withPartitions(partitionCount, replicationFactor int32) *ClusterPatchRequest {
	if partitionCount > 0 {
		req.Partitions = &ClusterPatchRequestPartition{Count: &partitionCount}
	}
	if replicationFactor > 0 {
		if req.Partitions == nil {
			req.Partitions = &ClusterPatchRequestPartition{}
		}
		req.Partitions.ReplicationFactor = &replicationFactor
	}
	return req
}

type ClusterPatchRequestBroker struct {
	Count  int32   `json:"count,omitempty"`
	Add    []int32 `json:"add,omitempty"`
	Remove []int32 `json:"remove,omitempty"`
}

type ClusterPatchRequestPartition struct {
	Count             *int32 `json:"count"`
	ReplicationFactor *int32 `json:"replicationFactor"`
}

type httpStatusError struct {
	StatusCode int
	Body       []byte
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP status %d: %s", e.StatusCode, string(e.Body))
}
