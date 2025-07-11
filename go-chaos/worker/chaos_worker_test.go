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

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	chaos_experiments "github.com/camunda/zeebe-chaos/go-chaos/internal/chaos-experiments"
	"github.com/camunda/zeebe/clients/go/v8/pkg/entities"
	"github.com/camunda/zeebe/clients/go/v8/pkg/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ShouldFailToHandleJobWithoutPayload(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	commandRunner := func(args []string, ctx context.Context) error {
		return nil // success
	}
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key: 123,
		},
	}

	// when
	HandleZbChaosJob(fakeJobClient, job, commandRunner)

	// then
	assert.True(t, fakeJobClient.Failed)
	assert.Equal(t, 123, fakeJobClient.Key)
	assert.Equal(t, 0, fakeJobClient.RetriesVal)
}

func Test_ShouldFailToHandleReadExperimentsJobWithoutPayload(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key: 123,
		},
	}

	// when
	HandleReadExperiments(fakeJobClient, job)

	// then
	assert.True(t, fakeJobClient.Failed)
	assert.Equal(t, 123, fakeJobClient.Key)
	assert.Equal(t, 0, fakeJobClient.RetriesVal)
}

func Test_ShouldFailWhenDockerImageIsNotSet(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	variables := createZbChaosVariables()
	variables.ZeebeImage = ""
	jsonString, err := json.Marshal(variables)
	commandRunner := func(args []string, ctx context.Context) error {
		return nil // success
	}

	require.NoError(t, err)
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key:                123,
			ProcessInstanceKey: 456,
			Retries:            1,
			Variables:          string(jsonString),
		},
	}

	// when
	HandleZbChaosJob(fakeJobClient, job, commandRunner)

	// then
	assert.True(t, fakeJobClient.Failed)
	assert.Equal(t, 123, fakeJobClient.Key)
	assert.Equal(t, 0, fakeJobClient.RetriesVal)
}

func Test_ShouldHandleCommand(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	jsonString, err := createVariablesAsJson()
	var appliedArgs []string
	commandRunner := func(args []string, ctx context.Context) error {
		appliedArgs = args
		return nil // success
	}

	require.NoError(t, err)
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key:                123,
			ProcessInstanceKey: 456,
			Variables:          jsonString,
		},
	}

	// when
	HandleZbChaosJob(fakeJobClient, job, commandRunner)

	// then
	assert.True(t, fakeJobClient.Succeeded)
	assert.Equal(t, 123, fakeJobClient.Key)
	expectedArgs := []string{
		"--namespace", "clusterId-zeebe",
		"--authServer=https://auth.com/url",
		"--audience=zeebe.com",
		"--clientId=randomClientId",
		"--clientSecret=superSecret",
		"disconnect", "gateway",
		"--all",
		"--verbose",
		"--jsonLogging",
		"--dockerImageTag",
		"test",
	}
	assert.Equal(t, expectedArgs, appliedArgs)
}

func Test_ShouldHandleCommandForSelfManagedWhenNoClusterId(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	variables := createZbChaosVariables()
	variables.ClusterId = new(string)

	jsonBytes, err := json.Marshal(variables)
	var appliedArgs []string
	commandRunner := func(args []string, ctx context.Context) error {
		appliedArgs = args
		return nil // success
	}

	require.NoError(t, err)
	jsonString := string(jsonBytes)
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key:                123,
			ProcessInstanceKey: 456,
			Variables:          jsonString,
		},
	}

	// when
	HandleZbChaosJob(fakeJobClient, job, commandRunner)

	// then
	assert.True(t, fakeJobClient.Succeeded)
	assert.Equal(t, 123, fakeJobClient.Key)
	expectedArgs := []string{
		"--authServer=https://auth.com/url",
		"--audience=zeebe.com",
		"--clientId=randomClientId",
		"--clientSecret=superSecret",
		"disconnect", "gateway",
		"--all",
		"--verbose",
		"--jsonLogging",
		"--dockerImageTag",
		"test",
	}
	assert.Equal(t, expectedArgs, appliedArgs)
}

func Test_ShouldSendExperimentsForClusterPlan(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key:       123,
			Variables: "{\"clusterPlan\":\"Production - S\", \"targetVersion\":\"8.8.0\"}",
		},
	}

	// when
	HandleReadExperiments(fakeJobClient, job)

	// then
	assert.True(t, fakeJobClient.Succeeded)
	assert.Equal(t, 123, fakeJobClient.Key)
	// as we don't have a version in this test, we should omit version bounded experiments
	experiments, err := chaos_experiments.ReadExperimentsForClusterPlan("Production - S", "8.8.0")
	require.NoError(t, err)
	assert.Equal(t, experiments, fakeJobClient.Variables)
}

func Test_ShouldFailWhenNoClusterPlanForReadExperimentsJob(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Key:       123,
			Variables: "{\"clusterPlan\":\"noop\", \"targetVersion\":\"8.8.0\"}",
		},
	}

	// when
	HandleReadExperiments(fakeJobClient, job)

	// then
	assert.True(t, fakeJobClient.Failed)
	assert.Equal(t, 123, fakeJobClient.Key)
	assert.Equal(t, "No experiments found for cluster plan 'noop'", fakeJobClient.ErrorMsg)
}

func Test_ShouldFailJobWhenHandleFails(t *testing.T) {
	// given
	fakeJobClient := &FakeJobClient{}
	jsonString, err := createVariablesAsJson()
	var appliedArgs []string
	commandRunner := func(args []string, ctx context.Context) error {
		appliedArgs = args
		return errors.New("failed")
	}

	require.NoError(t, err)
	job := entities.Job{
		ActivatedJob: &pb.ActivatedJob{
			Retries:            3,
			Key:                123,
			ProcessInstanceKey: 456,
			Variables:          jsonString,
		},
	}

	// when
	HandleZbChaosJob(fakeJobClient, job, commandRunner)

	// then
	assert.True(t, fakeJobClient.Failed)
	assert.Equal(t, 123, fakeJobClient.Key)
	// retry count is not decreased
	assert.Equal(t, 3, fakeJobClient.RetriesVal)
	assert.Equal(t, time.Duration(10)*time.Second, fakeJobClient.RetryBackoff)
	expectedArgs := []string{
		"--namespace", "clusterId-zeebe",
		"--authServer=https://auth.com/url",
		"--audience=zeebe.com",
		"--clientId=randomClientId",
		"--clientSecret=superSecret",
		"disconnect", "gateway",
		"--all",
		"--verbose",
		"--jsonLogging",
		"--dockerImageTag",
		"test",
	}
	assert.Equal(t, expectedArgs, appliedArgs)
}

func Test_shouldUnmarshalAuthCredentials(t *testing.T) {
	jsonAuth := `{
		"audience":"audience.zeebe.ultrawombat.com",
		"authorizationURL":"https://login.cloud.ultrawombat.com/oauth/token",
		"clientId":"myClientId",
		"clientSecret":"superSecret",
		"contactPoint":"example.chaos-1.zeebe.ultrawombat.com:443"
	}`
	var authentication AuthenticationProvider
	err := json.Unmarshal([]byte(jsonAuth), &authentication)
	assert.Nil(t, err)

	assert.Equal(t, authentication.Audience, "audience.zeebe.ultrawombat.com")
	assert.Equal(t, authentication.ClientId, "myClientId")
	assert.Equal(t, authentication.AuthorizationURL, "https://login.cloud.ultrawombat.com/oauth/token")
	assert.Equal(t, authentication.ClientSecret, "superSecret")
}

func Test_shouldCreateFlagsCorrectly(t *testing.T) {
	auth := createZbChaosVariables().AuthenticationDetails

	flags := auth.toFlags()
	expectedFlags := []string{
		"--authServer=https://auth.com/url",
		"--audience=zeebe.com",
		"--clientId=randomClientId",
		"--clientSecret=superSecret",
	}
	assert.Equal(t, expectedFlags, flags)
}

func createVariablesAsJson() (string, error) {
	variables := createZbChaosVariables()

	marshal, err := json.Marshal(variables)
	return string(marshal), err
}

func createZbChaosVariables() ZbChaosVariables {
	clusterId := "clusterId"
	title := "Fake experiment"
	variables := ZbChaosVariables{
		Title:     &title,
		ClusterId: &clusterId,
		Provider: ChaosProvider{
			Path:      "zbchaos",
			Arguments: []string{"disconnect", "gateway", "--all"},
		},
		ZeebeImage: "gcr.io/zeebe-io/zeebe:test",
		AuthenticationDetails: AuthenticationProvider{
			Audience:         "zeebe.com",
			AuthorizationURL: "https://auth.com/url",
			ClientId:         "randomClientId",
			ClientSecret:     "superSecret",
		},
	}
	return variables
}
