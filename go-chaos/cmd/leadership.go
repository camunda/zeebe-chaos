// Copyright 2026 Camunda Services GmbH
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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

// PartitionLeadership is where one partition's leadership is against where the
// cluster configuration wants it.
//
// The wanted leader is derived from the replicas' priorities rather than from
// PartitionMetadata.primary, which reports a placement the gateway computed and
// not the member Raft elects (see camunda/camunda#40036).
type PartitionLeadership struct {
	PartitionId  int32
	Leader       int32
	WantedLeader int32
	Priority     int32
}

func (p PartitionLeadership) balanced() bool {
	return p.Leader == p.WantedLeader
}

// What a broker reports about one partition it replicates. Only the role is
// read here; the endpoint reports processing and health detail besides.
type partitionStatus struct {
	Role string `json:"role"`
}

func addVerifyLeadershipCommand(verifyCmd *cobra.Command, flags *Flags) {
	verifyLeadershipCmd := &cobra.Command{
		Use:   "leadership",
		Short: "Verify that every partition is led by its highest-priority replica",
		Long: `Verifies that leadership is where the cluster configuration wants it.

Each partition's wanted leader is the replica the configuration gives the highest priority, so
this passes on a cluster whose leadership never drifted and fails on one where an election left a
lower-priority replica leading. Both halves are read from the management API - the priorities from
the cluster configuration and the roles from each broker's own view of the partitions it replicates
- so the check needs no client credentials. The two views are gossiped separately, so the cluster is
retried until the timeout before it is reported unbalanced.`,
		// An unbalanced cluster is a finding rather than a misuse of the probe,
		// so its usage is not printed over which partitions are unbalanced.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return verifyLeadership(flags)
		},
	}
	verifyCmd.AddCommand(verifyLeadershipCmd)

	verifyLeadershipCmd.Flags().IntVar(&flags.timeoutInSec, "timeoutInSec", 60,
		"how long to allow the two gossiped views to agree before reporting the cluster unbalanced")
	verifyLeadershipCmd.Flags().BoolVar(&flags.expectUnbalanced, "expectUnbalanced", false,
		"invert the check, so it passes only while some partition is led by a lower-priority replica")
}

func verifyLeadership(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}

	brokerPods, err := k8Client.GetBrokerPodNames()
	if err != nil {
		return err
	}
	if len(brokerPods) == 0 {
		return fmt.Errorf("no broker pods found in namespace %s", k8Client.GetCurrentNamespace())
	}

	timeout := time.Duration(flags.timeoutInSec) * time.Second
	interval := time.Second * 2
	started := time.Now()
	var lastLeadership []PartitionLeadership
	var lastErr error

	for {
		lastLeadership, lastErr = queryLeadership(k8Client, brokerPods)
		if lastErr == nil {
			unbalanced := unbalancedPartitions(lastLeadership)
			// Either state can be the one being waited for, so both end the
			// wait as soon as they are seen rather than after the timeout.
			if (len(unbalanced) == 0) != flags.expectUnbalanced {
				internal.LogInfo("%s", describeLeadership(lastLeadership))
				if flags.expectUnbalanced {
					internal.LogInfo("%d partitions are led by a lower-priority replica: %v",
						len(unbalanced), unbalanced)
				} else {
					internal.LogInfo("Every partition is led by its highest-priority replica.")
				}
				return nil
			}
		} else {
			internal.LogVerbose("Failed to read leadership: %s", lastErr)
		}

		if time.Since(started) >= timeout {
			break
		}
		time.Sleep(interval)
	}

	if lastErr != nil {
		return fmt.Errorf("could not read leadership within %s: %w", timeout, lastErr)
	}
	internal.LogInfo("%s", describeLeadership(lastLeadership))
	if flags.expectUnbalanced {
		return fmt.Errorf("every partition is led by its highest-priority replica, expected at least one not to be")
	}
	unbalanced := unbalancedPartitions(lastLeadership)
	return fmt.Errorf("%d partitions are led by a lower-priority replica: %v", len(unbalanced), unbalanced)
}

// queryLeadership reads the priorities from the cluster configuration and the
// roles from every broker, because the configuration says which replica should
// lead a partition and only the brokers say which one does.
func queryLeadership(k8Client internal.K8Client, brokerPods []string) ([]PartitionLeadership, error) {
	managementPort, closeManagement := k8Client.MustPodPortForward(brokerPods[0], 0, 9600)
	topology, err := QueryTopology(managementPort)
	closeManagement()
	if err != nil {
		return nil, err
	}

	leaders, err := queryLeaders(k8Client, brokerPods)
	if err != nil {
		return nil, err
	}
	wanted := highestPriorityReplicaByPartition(topology)

	leadership := make([]PartitionLeadership, 0, len(wanted))
	for partitionId, wantedLeader := range wanted {
		leader, hasLeader := leaders[partitionId]
		if !hasLeader {
			// A partition with no leader at all is not led by the replica the
			// configuration wants, so it counts as unbalanced.
			leader = -1
		}
		leadership = append(leadership, PartitionLeadership{
			PartitionId:  partitionId,
			Leader:       leader,
			WantedLeader: wantedLeader.brokerId,
			Priority:     wantedLeader.priority,
		})
	}
	if len(leadership) == 0 {
		return nil, fmt.Errorf("the cluster configuration reported no partitions with a priority")
	}
	sort.Slice(leadership, func(i, j int) bool {
		return leadership[i].PartitionId < leadership[j].PartitionId
	})
	return leadership, nil
}

// queryLeaders asks every broker which of its partitions it leads. A broker
// that cannot be reached is reported rather than skipped, since a partition
// only it leads would otherwise read as having no leader at all.
func queryLeaders(k8Client internal.K8Client, brokerPods []string) (map[int32]int32, error) {
	leaders := make(map[int32]int32)
	for _, podName := range brokerPods {
		nodeId, err := nodeIdOfPod(podName)
		if err != nil {
			return nil, err
		}
		partitions, err := queryPartitions(k8Client, podName)
		if err != nil {
			return nil, fmt.Errorf("failed to read partitions of %s: %w", podName, err)
		}
		for partitionId, status := range partitions {
			if status.Role == "LEADER" {
				leaders[partitionId] = nodeId
			}
		}
	}
	return leaders, nil
}

func queryPartitions(k8Client internal.K8Client, podName string) (map[int32]partitionStatus, error) {
	port, closePortForward := k8Client.MustPodPortForward(podName, 0, 9600)
	defer closePortForward()

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/actuator/partitions", port))
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("expected status code 200 but got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// The endpoint keys partitions by id as a string, so they are read as such
	// and converted rather than relying on numeric JSON keys.
	var reported map[string]partitionStatus
	if err := json.Unmarshal(body, &reported); err != nil {
		return nil, err
	}
	partitions := make(map[int32]partitionStatus, len(reported))
	for id, status := range reported {
		partitionId, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("%s reported a partition id that is not a number: %s", podName, id)
		}
		partitions[int32(partitionId)] = status
	}
	return partitions, nil
}

// nodeIdOfPod reads a broker's node id from its pod name, which a StatefulSet
// suffixes with the ordinal the broker takes as its node id.
func nodeIdOfPod(podName string) (int32, error) {
	ordinal := podName[strings.LastIndex(podName, "-")+1:]
	nodeId, err := strconv.Atoi(ordinal)
	if err != nil {
		return 0, fmt.Errorf("could not read a node id from pod name %s: %w", podName, err)
	}
	return int32(nodeId), nil
}

type replicaPriority struct {
	brokerId int32
	priority int32
}

// highestPriorityReplicaByPartition answers per partition which replica the
// configuration prefers. Only active brokers are considered, since a broker
// leaving is not somewhere leadership can go; ties resolve to the lowest broker
// id so that the answer is stable.
func highestPriorityReplicaByPartition(topology *CurrentTopology) map[int32]replicaPriority {
	wanted := make(map[int32]replicaPriority)
	for _, broker := range topology.Brokers {
		if broker.State != "ACTIVE" {
			continue
		}
		for _, partition := range broker.Partitions {
			current, seen := wanted[partition.Id]
			if !seen || partition.Priority > current.priority ||
				(partition.Priority == current.priority && broker.Id < current.brokerId) {
				wanted[partition.Id] = replicaPriority{brokerId: broker.Id, priority: partition.Priority}
			}
		}
	}
	return wanted
}

func unbalancedPartitions(leadership []PartitionLeadership) []int32 {
	unbalanced := make([]int32, 0)
	for _, partition := range leadership {
		if !partition.balanced() {
			unbalanced = append(unbalanced, partition.PartitionId)
		}
	}
	return unbalanced
}

func describeLeadership(leadership []PartitionLeadership) string {
	lines := make([]string, 0, len(leadership)+1)
	lines = append(lines, "  Partition | Leader | Wanted leader (priority)")
	for _, partition := range leadership {
		leader := fmt.Sprintf("%d", partition.Leader)
		if partition.Leader < 0 {
			leader = "none"
		}
		marker := " "
		if !partition.balanced() {
			marker = "!"
		}
		lines = append(lines, fmt.Sprintf("%s %9d | %6s | %d (%d)",
			marker, partition.PartitionId, leader, partition.WantedLeader, partition.Priority))
	}
	return strings.Join(lines, "\n")
}
