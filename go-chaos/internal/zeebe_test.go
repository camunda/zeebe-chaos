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

	"github.com/camunda/zeebe/clients/go/v8/pkg/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	dummyCreator := func() (int64, error) {
		return 6755399441055751, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 1, 10*time.Millisecond)

	// then
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), "Expected to create process instance on partition 1, but timed out after 10ms.")
}

func Test_ShouldImmediatelyTimeout(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 6755399441055751, nil
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
	dummyCreator := func() (int64, error) {
		if counter == 3 {
			return 2251799813685279, nil
		}
		counter++
		return 0, errors.New("foo")
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 1, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldRetryOnProcessInstanceCreationErrorAndWrongPartitionId(t *testing.T) {
	// given
	counter := 0
	dummyCreator := func() (int64, error) {
		counter++
		if counter == 1 {
			return 4503599627370515, nil
		}
		if counter == 2 {
			return 2251799813685279, nil
		}
		if counter == 4 {
			return 6755399441055757, nil
		}
		return 0, errors.New("foo")
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 3, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldSucceedOnCorrectPartition(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 4503599627370515, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 2, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldSucceedWhenRequiredPartitionIsZero(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 4503599627370515, nil
	}

	// when
	err := CreateProcessInstanceOnPartition(dummyCreator, 0, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldTimeoutIfProcessInstanceCountWasNotReached(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 0, errors.New("maep")
	}

	// when
	err := SendCountOfCommands(dummyCreator, 10, 10*time.Millisecond)

	// then
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), "Expected to send 10 commands, but timed out after 10ms whereas 0 commands have been sent.")
}

func Test_ShouldImmediatelyTimeoutForCountCreation(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 6755399441055751, nil
	}

	// when
	err := SendCountOfCommands(dummyCreator, 10, 0*time.Millisecond)

	// then
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), "Expected to send 10 commands, but timed out after 0s whereas 1 commands have been sent.")
}

func Test_ShouldRetryOnCreatingCountOfProcessInstances(t *testing.T) {
	// given
	counter := 1
	dummyCreator := func() (int64, error) {
		if counter == 3 {
			return 2251799813685279, nil
		}
		counter++
		return 0, errors.New("foo")
	}

	// when
	err := SendCountOfCommands(dummyCreator, 3, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldSucceedOnCorrectInstanceCount(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 4503599627370515, nil
	}

	// when
	err := SendCountOfCommands(dummyCreator, 2, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldSucceedWhenCountOfPartitionsIsZero(t *testing.T) {
	// given
	dummyCreator := func() (int64, error) {
		return 4503599627370515, nil
	}

	// when
	err := SendCountOfCommands(dummyCreator, 0, 1*time.Second)

	// then
	assert.NoError(t, err, "expected no error")
}

func Test_ShouldFindCorrelationKeyForPartition(t *testing.T) {
	// given
	expectedPartition := 47
	// this function only works if the count is larger then the expected partition
	partitionCount := 49

	// when
	correlationKeyForPartition, err := FindCorrelationKeyForPartition(expectedPartition, partitionCount)

	// then
	assert.NoError(t, err)
	hashCode := getSubscriptionHashCode(correlationKeyForPartition)
	actualPartition := (hashCode % partitionCount) + 1

	assert.Equal(t, expectedPartition, actualPartition, "The partitions should be equal")
}

func Test_ShouldFindCorrelationKeyForPartitionWhichIsEqualToCount(t *testing.T) {
	// given
	expectedPartition := 3
	partitionCount := 3

	// when
	correlationKeyForPartition, err := FindCorrelationKeyForPartition(expectedPartition, partitionCount)

	// then
	assert.NoError(t, err)
	hashCode := getSubscriptionHashCode(correlationKeyForPartition)
	actualPartition := (hashCode % partitionCount) + 1

	assert.Equal(t, expectedPartition, actualPartition, "The partitions should be equal")
}

func Test_ShouldFailToFindCorrelationKeyForPartition(t *testing.T) {
	// given
	expectedPartition := 47
	// this function only works if the count is larger than the expected partition
	partitionCount := 149

	// when
	_, err := FindCorrelationKeyForPartition(expectedPartition, partitionCount)

	// then
	assert.Error(t, err, "Expect error for too large partitionCount")
	assert.Contains(t, err.Error(), "must not exceed 78 partitions.")
}

func Test_ShouldFailIfExpectedPartitionIsLargenThanCount(t *testing.T) {
	// given
	expectedPartition := 47
	partitionCount := 3

	// when
	_, err := FindCorrelationKeyForPartition(expectedPartition, partitionCount)

	// then
	assert.Error(t, err, "Expect error for too small partitionCount")
	assert.Contains(t, err.Error(), "must be smaller than partitionsCount")
}

func getSubscriptionHashCode(correlationKey string) int {
	hashCode := 0
	for i := 0; i < len(correlationKey); i++ {
		hashCode = 31*hashCode + int(correlationKey[i])
	}
	return hashCode
}

func Test_ShouldReadDefaultFile(t *testing.T) {
	// given
	fileName := ""
	expectedBytes, err := bpmnContent.ReadFile("bpmn/one_task.bpmn")
	assert.NoError(t, err)

	// when
	defaultFileBytes, defaultFileName, err := readBPMNFileOrDefault(fileName)

	// then
	assert.NoError(t, err)
	assert.Equal(t, "bpmn/one_task.bpmn", defaultFileName)
	assert.Equal(t, expectedBytes, defaultFileBytes)
}

func Test_ShouldReadGivenFile(t *testing.T) {
	// given
	fileName := "somefile.txt"
	expectedBytes := []byte("content")
	err := os.WriteFile(fileName, expectedBytes, 0644)
	assert.NoError(t, err)

	// when
	fileBytes, actualFileName, err := readBPMNFileOrDefault(fileName)

	// then
	assert.NoError(t, err)
	assert.Equal(t, "somefile.txt", actualFileName)
	assert.Equal(t, expectedBytes, fileBytes)
	err = os.RemoveAll(fileName)
	assert.NoError(t, err)
}

func Test_ShouldSetVersionAndProcessIdWhenUsingPICreator(t *testing.T) {
	// given
	options := ProcessInstanceCreationOptions{BpmnProcessId: "processId", Version: 10}
	fakeClient := &FakeClient{}
	creator, err := CreateProcessInstanceCreator(fakeClient, options)
	require.NoError(t, err)

	// when
	processInstanceKey, err := creator()

	// then
	assert.Equal(t, int64(0xCAFE), processInstanceKey)
	assert.Equal(t, int32(10), fakeClient.version)
	assert.Equal(t, "processId", fakeClient.processId)
}

func Test_ShouldSetVariablesForVersionAndProcessIdWhenUsingPICreator(t *testing.T) {
	// given
	options := ProcessInstanceCreationOptions{BpmnProcessId: "processId", Version: 10, Variables: "{\"foo\":123}"}
	fakeClient := &FakeClient{}
	creator, err := CreateProcessInstanceCreator(fakeClient, options)
	require.NoError(t, err)

	// when
	processInstanceKey, err := creator()

	// then
	assert.Equal(t, int64(0xCAFE), processInstanceKey)
	assert.Equal(t, int32(10), fakeClient.version)
	assert.Equal(t, "processId", fakeClient.processId)
	assert.Equal(t, "{\"foo\":123}", fakeClient.vars)
}

func Test_ShouldAwaitResultForProcessInstanceWithVersionAndProcessIdWhenUsingPICreator(t *testing.T) {
	// given
	options := ProcessInstanceCreationOptions{BpmnProcessId: "processId", Version: 10, Variables: "{\"foo\":123}", AwaitResult: true}
	fakeClient := &FakeClient{}
	creator, err := CreateProcessInstanceCreator(fakeClient, options)
	require.NoError(t, err)

	// when
	processInstanceKey, err := creator()

	// then
	assert.Equal(t, int64(0xCAFE), processInstanceKey)
	assert.Equal(t, int32(10), fakeClient.version)
	assert.Equal(t, "processId", fakeClient.processId)
	assert.Equal(t, "{\"foo\":123}", fakeClient.vars)
	assert.True(t, fakeClient.awaitResult)
}

func Test_ShouldSetCorrectPropertiesWhenUsingJobCompleter(t *testing.T) {
	// given
	options := JobCompleteOptions{JobType: "benchmark-task"}
	fakeClient := &FakeClient{}
	completer, err := CreateJobCompleter(fakeClient, options)
	require.NoError(t, err)

	// when
	jobKey, err := completer()

	// then
	assert.Equal(t, int64(1), jobKey)
	assert.Equal(t, "benchmark-task", fakeClient.jobType)
	assert.Equal(t, int64(1), fakeClient.jobKey)
	assert.Equal(t, int32(1), fakeClient.fakeActivateCommand.maxActivate)
}
