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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_DescribeChangeStatusWithPending(t *testing.T) {
	// given
	topology := CurrentTopology{
		Version: 1,
		Brokers: []BrokerState{},
		LastChange: &LastChange{
			Id:     2,
			Status: "COMPLETED",
		},
		PendingChange: &TopologyChange{
			Id:              3,
			Status:          "IN_PROGRESS",
			InternalVersion: 1,
			Completed: []Operation{
				{
					Operation: "ADD",
					BrokerId:  1,
				},
			},
			Pending: []Operation{
				{
					Operation: "ADD_BROKER",
					BrokerId:  2,
				},
			},
		},
	}

	// then
	assert.Equal(t, ChangeStatusPending, describeChangeStatus(&topology, 3))
	assert.Equal(t, ChangeStatusCompleted, describeChangeStatus(&topology, 2))
	assert.Equal(t, ChangeStatusOutdated, describeChangeStatus(&topology, 1))
	assert.Equal(t, ChangeStatusUnknown, describeChangeStatus(&topology, 4))
}

func Test_DescribeChangeStatusWithoutChanges(t *testing.T) {
	// given
	topology := CurrentTopology{
		Version:       1,
		Brokers:       []BrokerState{},
		LastChange:    nil,
		PendingChange: nil,
	}

	// then
	assert.Equal(t, ChangeStatusUnknown, describeChangeStatus(&topology, 1))
	assert.Equal(t, ChangeStatusUnknown, describeChangeStatus(&topology, 2))
}

func Test_DescribeChangeStatusWithoutCompleted(t *testing.T) {
	// given
	topology := CurrentTopology{
		Version:    1,
		Brokers:    []BrokerState{},
		LastChange: nil,
		PendingChange: &TopologyChange{
			Id:              3,
			Status:          "IN_PROGRESS",
			InternalVersion: 1,
			Completed: []Operation{
				{
					Operation: "ADD",
					BrokerId:  1,
				},
			},
			Pending: []Operation{
				{
					Operation: "ADD_BROKER",
					BrokerId:  2,
				},
			},
		},
	}

	// then
	assert.Equal(t, ChangeStatusUnknown, describeChangeStatus(&topology, 1))
	assert.Equal(t, ChangeStatusPending, describeChangeStatus(&topology, 3))
	assert.Equal(t, ChangeStatusUnknown, describeChangeStatus(&topology, 4))
}

func Test_ClusterPatchRequestJsonWithAllFields(t *testing.T) {
	// given
	req := (&ClusterPatchRequest{}).withBrokers([]int32{1, 2, 3}).withPartitions(6, 3)
	// when
	json, err := json.Marshal(req)
	// then
	assert.Nil(t, err)
	assert.NotNil(t, json)
	expected := `{"brokers":{"count":3},"partitions":{"count":6,"replicationFactor":3}}`
	assert.Equal(t, expected, string(json))
}

func Test_ClusterPatchRequestJsonBrokerOnly(t *testing.T) {
	// given
	req := (&ClusterPatchRequest{}).withBrokers([]int32{1, 2, 3}).withPartitions(0, 0)
	// when
	json, err := json.Marshal(req)
	// then
	assert.Nil(t, err)
	assert.NotNil(t, json)
	expected := `{"brokers":{"count":3},"partitions":null}`
	assert.Equal(t, expected, string(json))
}

func Test_ClusterPatchRequestJsonPartitionOnly(t *testing.T) {
	// given
	req := (&ClusterPatchRequest{}).withBrokers(nil).withPartitions(8, 3)
	// when
	json, err := json.Marshal(req)
	// then
	assert.Nil(t, err)
	assert.NotNil(t, json)
	expected := `{"brokers":null,"partitions":{"count":8,"replicationFactor":3}}`
	assert.Equal(t, expected, string(json))
}

func Test_ClusterPatchRequestJsonReplicationFactorOnly(t *testing.T) {
	// given
	req := (&ClusterPatchRequest{}).withBrokers(nil).withPartitions(0, 3)
	// when
	json, err := json.Marshal(req)
	// then
	assert.Nil(t, err)
	assert.NotNil(t, json)
	expected := `{"brokers":null,"partitions":{"count":null,"replicationFactor":3}}`
	assert.Equal(t, expected, string(json))
}

func Test_BrokersInOtherRegionsCalculation(t *testing.T) {
	// given
	currentTopology := CurrentTopology{
		Brokers: []BrokerState{
			{Id: 0},
			{Id: 1},
			{Id: 2},
			{Id: 3},
		},
	}

	// when - failing over to region 0
	brokersInOtherRegions := getBrokersInOtherRegions(&currentTopology, 2, 0)

	// then - brokers in region 1 should be removed
	assert.Equal(t, []int32{1, 3}, brokersInOtherRegions)
}

func Test_JsonUnmarshalOfClusterTopology(t *testing.T) {
	// given
	jsonData := `{
	"brokers": [
		{"id": 1, "lastUpdatedAt": "2025-09-22T10:27:50.128126716Z", "partitions": [
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 1, "priority": 2, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 2, "priority": 3, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 3, "priority": 1, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 4, "priority": 1, "state": "ACTIVE"}
		], "state": "ACTIVE", "version": 2},
		{"id": 2, "lastUpdatedAt": "2025-09-22T10:27:53.491714139Z", "partitions": [
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 1, "priority": 1, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 2, "priority": 2, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 3, "priority": 3, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 4, "priority": 2, "state": "ACTIVE"}
		], "state": "ACTIVE", "version": 2},
		{"id": 0, "lastUpdatedAt": "2025-09-22T10:27:47.333185087Z", "partitions": [
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 1, "priority": 3, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 2, "priority": 1, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 3, "priority": 2, "state": "ACTIVE"},
			{"config": {"exporting": {"exporters": [{"id": "camundaexporter", "state": "ENABLED"}, {"id": "MetricsExporter", "state": "ENABLED"}]}}, "id": 4, "priority": 3, "state": "ACTIVE"}
		], "state": "ACTIVE", "version": 2}
	],
	"clusterId": "5f3d18fb-0025-454e-94a8-c44b75ba5bcb",
	"lastChange": {"completedAt": "2025-09-22T10:27:53.609679808Z", "id": 2, "startedAt": "2025-09-22T10:27:44.470567918Z", "status": "COMPLETED"},
	"routing": {
		"messageCorrelation": {"partitionCount": 3, "strategy": "HashMod"},
		"requestHandling": {"partitionCount": 4, "strategy": "AllPartitions"},
		"version": 3
	},
	"version": 3
}`

	// when
	var topology CurrentTopology
	err := json.Unmarshal([]byte(jsonData), &topology)

	// then
	assert.Nil(t, err)
	assert.Equal(t, 3, len(topology.Brokers))
	assert.Equal(t, 4, len(topology.Brokers[0].Partitions))
	assert.Equal(t, int32(4), topology.partitionCount())
	assert.Equal(t, "5f3d18fb-0025-454e-94a8-c44b75ba5bcb", topology.ClusterId)
	assert.NotNil(t, topology.Routing)
	assert.Equal(t, 3, topology.Routing.MessageCorrelation.PartitionCount)
	assert.Equal(t, "HashMod", topology.Routing.MessageCorrelation.Strategy)
	assert.Equal(t, 4, topology.Routing.RequestHandling.PartitionCount)
	assert.Equal(t, "AllPartitions", topology.Routing.RequestHandling.Strategy)
	assert.Equal(t, 4, topology.Routing.RequestHandling.PartitionCount)
}
