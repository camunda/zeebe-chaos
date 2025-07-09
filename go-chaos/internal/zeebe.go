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
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/camunda/zeebe/clients/go/v8/pkg/pb"
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
)

func CreateZeebeClient(port int, credentials *ClientCredentials) (zbc.Client, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)

	clientConfig := &zbc.ClientConfig{
		GatewayAddress:         endpoint,
		DialOpts:               []grpc.DialOption{},
		UsePlaintextConnection: true,
	}

	if credentials != nil {
		credsProvider, err := zbc.NewOAuthCredentialsProvider(&zbc.OAuthProviderConfig{
			Audience:               credentials.Audience,
			AuthorizationServerURL: credentials.AuthServer,
			ClientID:               credentials.ClientID,
			ClientSecret:           credentials.ClientSecret,
		})
		if err != nil {
			return nil, err
		}
		if credsProvider != nil {
			clientConfig.CredentialsProvider = credsProvider
			clientConfig.UsePlaintextConnection = false
		}
	}

	client, err := zbc.NewClient(clientConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}

type ClientCredentials struct {
	AuthServer   string
	Audience     string
	ClientID     string
	ClientSecret string
}

func GetBrokerPodNameForPartitionAndRole(k8Client K8Client,
	zbClient zbc.Client,
	partitionId int,
	role string,
) (string, error) {
	pod, err := GetBrokerPodForPartitionAndRole(k8Client, zbClient, partitionId, role)
	if err != nil {
		return "", err
	}

	return pod.Name, nil
}

func GetBrokerPodForPartitionAndRole(k8Client K8Client,
	zbClient zbc.Client,
	partitionId int,
	role string,
) (*v1.Pod, error) {
	firstBrokerNodeId, err := GetBrokerNodeId(zbClient, partitionId, role)
	if err != nil {
		return nil, err
	}

	return GetBrokerPodForNodeId(k8Client, firstBrokerNodeId)
}

func GetBrokerPodForNodeId(k8Client K8Client, brokerNodeId int32) (*v1.Pod, error) {
	pods, err := k8Client.GetBrokerPods()
	if err != nil {
		return nil, err
	}

	if int(brokerNodeId) >= len(pods.Items) {
		return nil, errors.New(fmt.Sprintf("Expected to find Broker with nodeId %d, but running pod count is %d. Be aware node id's start with zero.", brokerNodeId, len(pods.Items)))
	}

	// the following works since the pods are returned in alphabetical order (and end with '-id')
	return &pods.Items[brokerNodeId], nil
}

func GetBrokerNodeId(zbClient zbc.Client, partitionId int, role string) (int32, error) {
	topologyResponse, err := GetTopology(zbClient)
	if err != nil {
		return 0, err
	}

	partitionsCount := topologyResponse.PartitionsCount
	if partitionsCount < int32(partitionId) {
		errorMsg := fmt.Sprintf("Expected that given partition id (%d) is smaller then the partitions count %d, but was greater.", partitionId, partitionsCount)
		return 0, errors.New(errorMsg)
	}

	nodeId, err := extractNodeId(topologyResponse, partitionId, role)
	if err != nil {
		return 0, err
	}
	return nodeId, nil
}

func GetTopology(zbClient zbc.Client) (*pb.TopologyResponse, error) {
	return zbClient.NewTopologyCommand().Send(context.TODO())
}

func extractNodeId(topologyResponse *pb.TopologyResponse, partitionId int, role string) (int32, error) {
	roleValue, exist := pb.Partition_PartitionBrokerRole_value[role]
	if !exist {
		errorMsg := fmt.Sprintf("Expected a partition role, which is part of [LEADER, FOLLOWER, INACTIVE], but got %s.", role)
		return 0, errors.New(errorMsg)
	}

	nodeId := int32(-1)
	for _, broker := range topologyResponse.Brokers {
		for _, partition := range broker.Partitions {
			if partition.PartitionId == int32(partitionId) &&
				partition.Role == pb.Partition_PartitionBrokerRole(roleValue) {
				nodeId = broker.NodeId
				break
			}
		}
	}

	if nodeId == int32(-1) {
		errorMsg := fmt.Sprintf("Expected to find broker with given partition id (%d) and role %s, but found nothing.", partitionId, role)
		return 0, errors.New(errorMsg)
	}

	return nodeId, nil
}

// bpmnContent holds our static bpmn models, which are copied with the go:embed directive
//
//go:embed bpmn/*
var bpmnContent embed.FS

func DeployModel(client zbc.Client, fileName string) (int64, error) {
	bpmnBytes, fileName, err := readBPMNFileOrDefault(fileName)
	if err != nil {
		return 0, err
	}

	return DeployModelBytes(client, fileName, bpmnBytes)
}

func DeployModelBytes(client zbc.Client, fileName string, bpmnBytes []byte) (int64, error) {
	LogVerbose("Deploy file %s (size: %d bytes).", fileName, len(bpmnBytes))

	response, err := client.NewDeployProcessCommand().AddResource(bpmnBytes, fileName).Send(context.TODO())
	if err != nil {
		return 0, err
	}

	processDefinitionKey := response.Processes[0].ProcessDefinitionKey
	LogVerbose("Deployed process model %s successful with key %d.", fileName, processDefinitionKey)
	return processDefinitionKey, nil
}

type models struct {
	bpmnBytes    []byte
	bpmnFileName string
	dmnBytes     []byte
	dmnFileName  string
}

func readModels(bpmnFileName string, dmnFileName string) (*models, error) {
	bpmnBytes, err := bpmnContent.ReadFile(bpmnFileName)
	if err != nil {
		return nil, err
	}

	dmnBytes, err := bpmnContent.ReadFile(dmnFileName)
	if err != nil {
		return nil, err
	}

	return &models{bpmnBytes: bpmnBytes, dmnBytes: dmnBytes, bpmnFileName: bpmnFileName, dmnFileName: dmnFileName}, nil
}

func deployModels(client zbc.Client, models *models) error {
	_, err := client.NewDeployResourceCommand().AddResource(models.bpmnBytes, models.bpmnFileName).AddResource(models.dmnBytes, models.dmnFileName).Send(context.TODO())
	if err != nil {
		return err
	}
	return nil
}

func DeployDifferentVersions(client zbc.Client, versions int32) error {
	firstModel, err := readModels("bpmn/multi-version/multiVersionModel.bpmn", "bpmn/multi-version/fancyDecision.dmn")
	if err != nil {
		return err
	}

	secondModel, err := readModels("bpmn/multi-version/multiVersionModel_v2.bpmn", "bpmn/multi-version/fancyDecision_v2.dmn")
	if err != nil {
		return err
	}

	LogVerbose("Deploy %d versions of different type of models.", versions)

	count := int32(0)
	for count < versions {
		err := deployModels(client, firstModel)
		if err != nil {
			return err
		}

		err = deployModels(client, secondModel)
		if err != nil {
			return err
		}

		count += 2
		LogVerbose("Deployed [%d/%d] versions.", count, versions)
	}

	return nil
}

func DeployChaosModels(client zbc.Client) error {
	dirEntries, err := bpmnContent.ReadDir("bpmn/chaos")
	if err != nil {
		return err
	}

	for _, dirEntry := range dirEntries {
		path := fmt.Sprintf("bpmn/chaos/%s", dirEntry.Name())

		bpmnBytes, err := bpmnContent.ReadFile(path)
		if err != nil {
			return err
		}

		_, err = DeployModelBytes(client, path, bpmnBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

// if file not exist we read our default BPMN process model and return the content
func readBPMNFileOrDefault(fileName string) ([]byte, string, error) {
	var bpmnBytes []byte
	var err error

	if len(fileName) == 0 {
		fileName = "bpmn/one_task.bpmn"
		bpmnBytes, err = bpmnContent.ReadFile(fileName)
		if err != nil {
			return nil, "", err
		}
	} else if strings.Contains(fileName, "bpmn/") {
		bpmnBytes, err = bpmnContent.ReadFile(fileName)
		if err != nil {
			return nil, "", err
		}
	} else {
		bpmnBytes, err = os.ReadFile(fileName)
		if err != nil {
			return nil, "", err
		}
	}

	return bpmnBytes, fileName, nil
}

type ProcessInstanceCreationOptions struct {
	Version       int32
	BpmnProcessId string
	Variables     string
	AwaitResult   bool
}

type JobCompleteOptions struct {
	JobType string
}

func CreateProcessInstanceCreator(zbClient zbc.Client, options ProcessInstanceCreationOptions) (ProcessInstanceCreator, error) {
	var processInstanceCreator ProcessInstanceCreator
	processInstanceCreator = func() (int64, error) {
		LogVerbose("Send create process instance command, with BPMN process ID '%s' and version '%d' (-1 means latest) [variables: '%s', awaitResult: %t]",
			options.BpmnProcessId, options.Version, options.Variables, options.AwaitResult)
		commandStep3 := zbClient.NewCreateInstanceCommand().BPMNProcessId(options.BpmnProcessId).Version(options.Version)
		if len(options.Variables) != 0 {
			_, err := commandStep3.VariablesFromString(options.Variables)
			if err != nil {
				return 0, err
			}
		}

		if options.AwaitResult {
			instanceWithResultResponse, err := commandStep3.WithResult().Send(context.TODO())
			if err != nil {
				return 0, err
			}
			return instanceWithResultResponse.ProcessInstanceKey, nil
		}
		instanceResponse, err := commandStep3.Send(context.TODO())
		if err != nil {
			return 0, err
		}
		return instanceResponse.ProcessInstanceKey, nil
	}
	return processInstanceCreator, nil
}

type ProcessInstanceCreator func() (int64, error)

func CreateJobCompleter(zbClient zbc.Client, options JobCompleteOptions) (ZCCommandSender, error) {
	var jobCompleter ZCCommandSender
	jobCompleter = func() (int64, error) {
		LogVerbose("Send job activate command, with job type '%s'",
			options.JobType)
		jobs, err := zbClient.NewActivateJobsCommand().JobType(options.JobType).MaxJobsToActivate(1).Send(context.TODO())
		if err != nil {
			return 0, err
		}

		if len(jobs) == 0 {
			return 0, errors.New(fmt.Sprintf("Expected to find job with type '%s', but none found.", options.JobType))
		}

		jobKey := jobs[0].Key
		_, err = zbClient.NewCompleteJobCommand().JobKey(jobKey).Send(context.TODO())
		return jobKey, err
	}
	return jobCompleter, nil
}

type ZCCommandSender func() (int64, error)

func CreateProcessInstanceOnPartition(piCreator ProcessInstanceCreator, requiredPartition int32, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	tickerChan := time.Tick(100 * time.Millisecond)

	partitionId := int32(0)
	for {
		select {
		case <-timeoutChan:
			return errors.New(fmt.Sprintf("Expected to create process instance on partition %d, but timed out after %s.", requiredPartition, timeout.String()))
		case <-tickerChan:
			processInstanceKey, err := piCreator()
			if err != nil {
				// we do not return here, since we want to retry until the timeout
				LogInfo("Encountered an error during process instance creation. Error: %s", err.Error())
				break
			}
			partitionId = ExtractPartitionIdFromKey(processInstanceKey)

			LogVerbose("Created process instance with key %d on partition %d, required partition %d.", processInstanceKey, partitionId, requiredPartition)

			if partitionId == requiredPartition {
				return nil
			} else if requiredPartition == 0 {
				return nil
			}
		}
	}
}

func SendCountOfCommands(commandSender ZCCommandSender, countOfInstances int32, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	tickerChan := time.Tick(100 * time.Millisecond)

	count := int32(0)
	partitionId := int32(0)
	for {
		select {
		case <-timeoutChan:
			return errors.New(fmt.Sprintf("Expected to send %d commands, but timed out after %s whereas %d commands have been sent.", countOfInstances, timeout.String(), count))
		case <-tickerChan:
			key, err := commandSender()
			if err != nil {
				// we do not return here, since we want to retry until the timeout
				LogInfo("Encountered an error during command sending. Error: %s", err.Error())
				break
			}
			count++
			partitionId = ExtractPartitionIdFromKey(key)
			LogVerbose("[%d/%d] Successful command sent, got response with key %d on partition %d.", count, countOfInstances, key, partitionId)

			if count >= countOfInstances {
				return nil
			}
		}
	}
}

func ExtractPartitionIdFromKey(key int64) int32 {
	return int32(key >> 51)
}

func FindCorrelationKeyForPartition(expectedPartition int, partitionsCount int) (string, error) {
	if expectedPartition > partitionsCount {
		return "", errors.New(fmt.Sprintf("expected partition (%d) must be smaller than partitionsCount (%d)", expectedPartition, partitionsCount))
	}

	if partitionsCount >= 78 {
		return "", errors.New(fmt.Sprintf("partitionsCount (%d) must not exceed 78 partitions. Hashcode calculation is based of ASCII 48 to 126.", partitionsCount))
	}

	// The message publish partition distribution is based
	// on the following hashcode and modulo by the partition count
	//
	// Since this calculation is used not only on publish, but also for
	// opening the message and instance subscriptions we must support for
	// backwards compatibility, otherwise message will not be correlated
	//
	// This allows us to make certain assumptions, and shortcuts.
	//
	// static int getSubscriptionHashCode(final DirectBuffer correlationKey) {
	//   is equal to java.lang.String#hashCode
	//   int hashCode = 0;
	//
	//   for (int i = 0, length = correlationKey.capacity(); i < length; i++) {
	//      	hashCode = 31 * hashCode + correlationKey.getByte(i);
	//   }
	//   return hashCode;
	// }
	//
	// and
	//   public static int getSubscriptionPartitionId(
	//      final DirectBuffer correlationKey, final int partitionCount) {
	//    final int hashCode = getSubscriptionHashCode(correlationKey);
	//    // partition ids range from START_PARTITION_ID .. START_PARTITION_ID + partitionCount
	//    return Math.abs(hashCode % partitionCount) + START_PARTITION_ID;
	//  }
	// Based on the hash code function, we now that if the string has a length of one byte
	// no multiplication will happen.
	//
	// Since we need a printable (for variables) character we start with ASCII code 48, until 126.
	// Formulas:
	//
	// expectedPartition = (hashcode mod partitionCount) + partitionStartId
	// hashcode = i ( 48 >= i >= 126) == ASCII CODE
	//
	for hashcode := 48; hashcode < 127; hashcode++ {
		if expectedPartition == ((hashcode % partitionsCount) + 1) {
			return string(rune(hashcode)), nil
		}
	}
	return "", errors.New(fmt.Sprintf("Found no matching correlationKey for expectedPartition %d and partitionCount %d.", expectedPartition, partitionsCount))
}
