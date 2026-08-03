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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ShouldPreferTheHighestPriorityReplica(t *testing.T) {
	// given
	topology := CurrentTopology{Brokers: []BrokerState{
		{Id: 0, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 1}}},
		{Id: 1, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 3}}},
		{Id: 2, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 2}}},
	}}

	// when
	wanted := highestPriorityReplicaByPartition(&topology)

	// then
	assert.Equal(t, replicaPriority{brokerId: 1, priority: 3}, wanted[1])
}

func Test_ShouldIgnoreReplicasOnBrokersThatAreNotActive(t *testing.T) {
	// given a broker leaving the cluster holds the highest priority
	topology := CurrentTopology{Brokers: []BrokerState{
		{Id: 0, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 1}}},
		{Id: 1, State: "LEAVING", Partitions: []PartitionState{{Id: 1, State: "LEAVING", Priority: 3}}},
	}}

	// when
	wanted := highestPriorityReplicaByPartition(&topology)

	// then leadership is wanted where it can actually go
	assert.Equal(t, replicaPriority{brokerId: 0, priority: 1}, wanted[1])
}

func Test_ShouldResolveEqualPrioritiesToTheLowestBrokerId(t *testing.T) {
	// given
	topology := CurrentTopology{Brokers: []BrokerState{
		{Id: 2, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 1}}},
		{Id: 1, State: "ACTIVE", Partitions: []PartitionState{{Id: 1, State: "ACTIVE", Priority: 1}}},
	}}

	// when
	wanted := highestPriorityReplicaByPartition(&topology)

	// then
	assert.Equal(t, int32(1), wanted[1].brokerId)
}

func Test_ShouldReadTheNodeIdFromTheBrokerPodName(t *testing.T) {
	// when
	nodeId, err := nodeIdOfPod("camunda-4")

	// then
	require.NoError(t, err)
	assert.Equal(t, int32(4), nodeId)
}

func Test_ShouldFailOnAPodNameWithoutAnOrdinal(t *testing.T) {
	// when
	_, err := nodeIdOfPod("camunda-gateway")

	// then
	require.Error(t, err)
}

func Test_ShouldReportPartitionsLedByALowerPriorityReplica(t *testing.T) {
	// given
	leadership := []PartitionLeadership{
		{PartitionId: 1, Leader: 0, WantedLeader: 0},
		{PartitionId: 2, Leader: 1, WantedLeader: 2},
		{PartitionId: 3, Leader: -1, WantedLeader: 0},
	}

	// when
	unbalanced := unbalancedPartitions(leadership)

	// then a partition with no leader at all counts as unbalanced
	assert.Equal(t, []int32{2, 3}, unbalanced)
}
