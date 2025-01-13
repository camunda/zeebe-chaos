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

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/cmd"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func Test_ShouldBeAbleToQueryTopology(t *testing.T) {
	// given
	ctx := context.Background()
	container := CreateZeebeContainer(t, ctx)
	mappedPort, err := container.MappedPort(ctx, "9600/tcp")
	require.NoError(t, err)

	// when
	var topology *cmd.CurrentTopology
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		topology, err = cmd.QueryTopology(mappedPort.Int())
		require.NoError(t, err)
		if topology.Version > 0 || ctx.Err() != nil {
			break
		}
	}

	// then
	require.NoError(t, err)
	require.NotNil(t, topology)
	require.NotNil(t, topology.Brokers)
	require.Equal(t, 1, len(topology.Brokers))
	require.Equal(t, int32(1), topology.Version)
	require.Nil(t, topology.LastChange)
	require.Nil(t, topology.PendingChange)
}

func CreateZeebeContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:        "camunda/zeebe:8.4.0-alpha2",
		ExposedPorts: []string{"26500/tcp", "9600/tcp"},
		WaitingFor:   wait.ForHTTP("/actuator/health").WithPort("9600"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	return container
}
