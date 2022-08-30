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

package internal

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/camunda-cloud/zeebe/clients/go/pkg/pb"
	"github.com/stretchr/testify/assert"
)

func Test_ExtractNodeId(t *testing.T) {
	// given
	partitionId := 3
	role := "LEADER"
	topology := createTopologyStub()

	// when
	nodeId, err := extractNodeId(&topology, partitionId, role)

	// then
	assert.NoError(t, err)
	assert.Equal(t, int32(2), nodeId, "NodeId is not equal")
}

func Test_FailWhenRoleNotCorrect(t *testing.T) {
	// given
	role := "nil"

	// when
	_, err := extractNodeId(nil, 1, role)

	// then
	assert.Error(t, err)
}

func Test_FailWhenPartitionNotCorrect(t *testing.T) {
	// given
	role := "LEADER"
	partitionId := -1
	stub := createTopologyStub()

	// when
	_, err := extractNodeId(&stub, partitionId, role)

	// then
	assert.Error(t, err)
}

func Test_FailWhenPartitionNotFound(t *testing.T) {
	// given
	role := "LEADER"
	partitionId := 4
	stub := createTopologyStub()

	// when
	_, err := extractNodeId(&stub, partitionId, role)

	// then
	assert.Error(t, err)
}

func createTopologyStub() pb.TopologyResponse {
	return pb.TopologyResponse{
		Brokers: []*pb.BrokerInfo{
			{
				NodeId: 0,
				Partitions: []*pb.Partition{
					{
						PartitionId: 1,
						Role:        pb.Partition_LEADER,
					},
					{
						PartitionId: 2,
						Role:        pb.Partition_FOLLOWER,
					},
					{
						PartitionId: 3,
						Role:        pb.Partition_FOLLOWER,
					},
				},
			},
			{
				NodeId: 1,
				Partitions: []*pb.Partition{
					{
						PartitionId: 1,
						Role:        pb.Partition_FOLLOWER,
					},
					{
						PartitionId: 2,
						Role:        pb.Partition_LEADER,
					},
					{
						PartitionId: 3,
						Role:        pb.Partition_FOLLOWER,
					},
				},
			},
			{
				NodeId: 2,
				Partitions: []*pb.Partition{
					{
						PartitionId: 1,
						Role:        pb.Partition_FOLLOWER,
					},
					{
						PartitionId: 2,
						Role:        pb.Partition_FOLLOWER,
					},
					{
						PartitionId: 3,
						Role:        pb.Partition_LEADER,
					},
				},
			},
		},
	}
}

func Test_ExtractPartitionId(t *testing.T) {
	// given
	key := int64(6755399441055751)

	// when
	partitionId := ExtractPartitionIdFromKey(key)

	// then
	assert.Equal(t, int32(3), partitionId)
}

func Test_ShouldTimeoutIfProcessInstanceWasNotCreatedOnRequiredPartition(t *testing.T) {
	// given
	dummyCreator := func(processName string) (*pb.CreateProcessInstanceResponse, error) {
		return &pb.CreateProcessInstanceResponse{
			ProcessInstanceKey: 6755399441055751,
		}, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 1, 10*time.Millisecond)

	// then
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), "Expected to create process instance on partition 1, but timed out after 10ms.")
}

func Test_ShouldImmediatelyTimeout(t *testing.T) {
	// given
	dummyCreator := func(processName string) (*pb.CreateProcessInstanceResponse, error) {
		return &pb.CreateProcessInstanceResponse{
			ProcessInstanceKey: 6755399441055751,
		}, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 1, 0*time.Millisecond)

	// then
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), "Expected to create process instance on partition 1, but timed out after 0s.")
}

func Test_ShouldRetryOnProcessInstanceCreationError(t *testing.T) {
	// given
	counter := 1
	dummyCreator := func(processName string) (*pb.CreateProcessInstanceResponse, error) {
		if counter == 3 {
			return &pb.CreateProcessInstanceResponse{
				ProcessInstanceKey: 2251799813685279,
			}, nil
		}
		counter++
		return nil, errors.New("foo")
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 1, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldRetryOnProcessInstanceCreationErrorAndWrongPartitionId(t *testing.T) {
	// given
	counter := 0
	dummyCreator := func(processName string) (*pb.CreateProcessInstanceResponse, error) {
		counter++
		if counter == 1 {
			return &pb.CreateProcessInstanceResponse{
				ProcessInstanceKey: 4503599627370515,
			}, nil
		}
		if counter == 2 {
			return &pb.CreateProcessInstanceResponse{
				ProcessInstanceKey: 2251799813685279,
			}, nil
		}
		if counter == 4 {
			return &pb.CreateProcessInstanceResponse{
				ProcessInstanceKey: 6755399441055757,
			}, nil
		}
		return nil, errors.New("foo")
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 3, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldSucceedOnCorrectPartition(t *testing.T) {
	// given
	dummyCreator := func(processName string) (*pb.CreateProcessInstanceResponse, error) {
		return &pb.CreateProcessInstanceResponse{
			ProcessInstanceKey: 4503599627370515,
		}, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 2, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldReadDefaultFile(t *testing.T) {
	// given
	fileName := ""
	expectedBytes, err := bpmnContent.ReadFile("bpmn/one_task.bpmn")
	assert.NoError(t, err)

	// when
	defaultFileBytes, err := readBPMNFileOrDefault(fileName)

	// then
	assert.NoError(t, err)
	assert.Equal(t, expectedBytes, defaultFileBytes)
}

func Test_ShouldReadGivenFile(t *testing.T) {
	// given
	fileName := "somefile.txt"
	expectedBytes := []byte("content")
	err := os.WriteFile(fileName, expectedBytes, 0644)
	assert.NoError(t, err)

	// when
	fileBytes, err := readBPMNFileOrDefault(fileName)

	// then
	assert.NoError(t, err)
	assert.Equal(t, expectedBytes, fileBytes)
	err = os.RemoveAll(fileName)
	assert.NoError(t, err)
}
