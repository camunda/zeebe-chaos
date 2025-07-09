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

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/cmd"
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func Test_ShouldBeAbleToDeployChaosModels(t *testing.T) {
	// given
	ctx := context.Background()
	container := CreateEZEContainer(t, ctx)
	defer container.StopLogProducer()
	mappedPort, err := container.MappedPort(ctx, "26500/tcp")
	require.NoError(t, err)
	zeebeClient, err := internal.CreateZeebeClient(mappedPort.Int())
	require.NoError(t, err)

	// when
	err = internal.DeployChaosModels(zeebeClient)

	// then
	require.NoError(t, err)
}

func Test_ShouldBeAbleToRunExperiments(t *testing.T) {
	t.Skip("Conceptually wrong")
	// given
	internal.Verbosity = true
	cmd.Verbose = true
	ctx := context.Background()
	container := CreateEZEContainer(t, ctx)
	defer container.StopLogProducer()
	mappedPort, err := container.MappedPort(ctx, "26500/tcp")
	require.NoError(t, err)
	zeebeClient, err := internal.CreateZeebeClient(mappedPort.Int())
	require.NoError(t, err)
	err = internal.DeployChaosModels(zeebeClient)

	// required variables
	vars := make(map[string]interface{})
	vars["clusterPlan"] = "test" // specifies the cluster plan for which we read the experiments
	vars["clusterId"] = ""       // need to be set to empty string, otherwise we run into a SIGSEG
	vars["zeebeImage"] = "gcr.io/zeebe-io/zeebe:SNAPSHOT"

	commandStep3, err := zeebeClient.NewCreateInstanceCommand().BPMNProcessId("chaosToolkit").LatestVersion().VariablesFromMap(vars)
	require.NoError(t, err)
	timeout := time.After(60 * time.Second)
	done := make(chan struct{})

	// when
	// concurrent open workers and start instance (await result)
	go cmd.OpenWorkers(zeebeClient)
	go func() {
		internal.LogInfo("Create ChaosToolkit instance")
		deadline, cancelFunc := context.WithDeadline(ctx, time.UnixMilli(time.Now().UnixMilli()+int64(60*time.Minute)))
		defer cancelFunc()
		response, err := commandStep3.WithResult().Send(deadline)
		require.NoError(t, err)
		internal.LogInfo("Instance %d [definition %d ] completed", response.ProcessInstanceKey, response.ProcessDefinitionKey)
		close(done)
	}()

	// then
	select {
	case <-timeout:
		t.Fatal("Process instance hasn't been completed in time.")
	case <-done: // wait until instance has been completed
	}
}

func CreateEZEContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/camunda-community-hub/eze:1.0.2",
		ExposedPorts: []string{"26500/tcp"},
		WaitingFor:   wait.ForLog("EZE agent started at 0.0.0.0:26500"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	err = container.StartLogProducer(ctx)
	require.NoError(t, err)
	printer := Printer{}
	container.FollowOutput(&printer)
	return container
}

type Printer struct {
}

func (p *Printer) Accept(l testcontainers.Log) {
	fmt.Print(string(l.Content))
}
