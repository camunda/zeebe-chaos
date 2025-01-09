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

package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/camunda/zeebe/clients/go/v8/pkg/entities"
	zbworker "github.com/camunda/zeebe/clients/go/v8/pkg/worker"
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/spf13/cobra"
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	worker "github.com/camunda/zeebe-chaos/go-chaos/worker"
	"google.golang.org/grpc"
)

const jobTypeZbChaos = "zbchaos"
const jobTypeReadExperiments = "readExperiments"

const ENV_AUTHORIZATION_SERVER_URL = "CHAOS_AUTOMATION_CLUSTER_AUTHORIZATION_SERVER_URL"
const ENV_CLIENT_ID = "CHAOS_AUTOMATION_CLUSTER_CLIENT_ID"
const ENV_CLIENT_SECRET = "CHAOS_AUTOMATION_CLUSTER_CLIENT_SECRET"
const ENV_ADDRESS = "CHAOS_AUTOMATION_CLUSTER_ADDRESS"

func AddWorkerCmd(rootCmd *cobra.Command) {
	var workerCommand = &cobra.Command{
		Use:   "worker",
		Short: "Starts a worker for zbchaos jobs",
		Long:  "Starts a worker for zbchaos jobs that executes zbchaos commands",
		Run:   start_worker,
	}
	rootCmd.AddCommand(workerCommand)
}

func start_worker(cmd *cobra.Command, args []string) {
	// The credentials are set via env var's.
	// We use here different names for the environment variables on purpose.
	// If we use the normal ZEEBE_ environment variables we would run
	// into conflicts when using multiple zeebe clients, the env vars will always overwrite
	// direct values
	credsProvider, err := zbc.NewOAuthCredentialsProvider(&zbc.OAuthProviderConfig{
		Audience:               strings.TrimSuffix(os.Getenv(ENV_ADDRESS), ":443"),
		AuthorizationServerURL: os.Getenv(ENV_AUTHORIZATION_SERVER_URL),
		ClientID:               os.Getenv(ENV_CLIENT_ID),
		ClientSecret:           os.Getenv(ENV_CLIENT_SECRET),
	})
	if err != nil {
		panic(err)
	}

	client, err := zbc.NewClient(&zbc.ClientConfig{
		GatewayAddress:         os.Getenv(ENV_ADDRESS),
		CredentialsProvider:    credsProvider,
		DialOpts:               []grpc.DialOption{},
		UsePlaintextConnection: false,
	})

	if err != nil {
		panic(err)
	}

	internal.LogVerbose("Connect to: %s.", os.Getenv(ENV_ADDRESS))

	OpenWorkers(client)
}

func OpenWorkers(client zbc.Client) {
	internal.LogVerbose("Open workers: [%s, %s].", jobTypeZbChaos, jobTypeReadExperiments)

	// Allow only one job at a time, otherwise job handling might interfere (e.g. override global vars)
	jobWorker := client.NewJobWorker().JobType(jobTypeZbChaos).Handler(handleZbChaosJob).MaxJobsActive(1).Open()
	client.NewJobWorker().JobType(jobTypeReadExperiments).Handler(worker.HandleReadExperiments).MaxJobsActive(1).Open()
	jobWorker.AwaitClose()
}

func handleZbChaosJob(client zbworker.JobClient, job entities.Job) {
	worker.HandleZbChaosJob(client, job, runZbChaosCommand)
}

func runZbChaosCommand(args []string, ctx context.Context) error {
	internal.LogInfo("Running command with args: %v ", args)
	cmd := NewCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteContextC(ctx)
	if err != nil {
		return err
	}
	return nil
}
