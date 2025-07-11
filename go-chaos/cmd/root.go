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
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
}

type Flags struct {
	partitionId        int
	role               string
	nodeId             int
	processModelPath   string
	versionCount       int
	variables          string
	msgName            string
	awaitResult        bool
	broker1PartitionId int
	broker1Role        string
	broker1NodeId      int
	broker2PartitionId int
	broker2Role        string
	broker2NodeId      int

	// deploy worker
	pollingDelayMs int

	// backup
	backupId string

	// disconnect
	oneDirection    bool
	disconnectToAll bool

	// stress

	cpuStress    bool
	memoryStress bool
	ioStress     bool
	timeoutSec   string

	// terminate

	all bool

	// verify
	version        int
	bpmnProcessId  string
	timeoutInSec   int
	kubeConfigPath string
	namespace      string
	instanceCount  int
	jobCount       int
	jobType        string

	// cluster
	changeId          int64
	brokers           int
	regionId          int32
	regions           int32
	replicationFactor int32

	// dataloss
	awaitReadiness bool

	// client connection
	authServer   string
	audience     string
	clientId     string
	clientSecret string
}

var (
	Version        = "development"
	Commit         = "HEAD"
	Verbose        bool
	JsonLogging    bool
	DockerImageTag string = "zeebe"
)

func NewCmd() *cobra.Command {
	flags := Flags{}

	rootCmd := &cobra.Command{
		Use:   "zbchaos",
		Short: "Zeebe chaos is a chaos experiment tool for Zeebe",
		Long: `A chaos experimenting toolkit for Zeebe.
    Perfect to inject some chaos into your brokers and gateways.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			internal.Verbosity = Verbose
			internal.LogVerbose("Flags: %v", flags)
			internal.JsonLogging = JsonLogging
			if JsonLogging {
				internal.JsonLogger = log.With().Logger()
			}
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&JsonLogging, "jsonLogging", "", false, "json logging output")
	rootCmd.PersistentFlags().StringVar(&flags.kubeConfigPath, "kubeconfig", "", "path the the kube config that will be used")
	rootCmd.PersistentFlags().StringVarP(&flags.namespace, "namespace", "n", "", "connect to the given namespace")
	rootCmd.PersistentFlags().StringVarP(&DockerImageTag, "dockerImageTag", "", DockerImageTag, "use the given docker image tag for deployed resources, e.g. worker/starter")
	// auth flags
	rootCmd.PersistentFlags().StringVar(&flags.audience, "audience", "", "authentication audience")
	rootCmd.PersistentFlags().StringVar(&flags.authServer, "authServer", "", "authentication authserver")
	rootCmd.PersistentFlags().StringVar(&flags.clientId, "clientId", "", "authentication clientId")
	rootCmd.PersistentFlags().StringVar(&flags.clientSecret, "clientSecret", "", "authentication clientSecret")
	rootCmd.Parent().MarkFlagsRequiredTogether("audience", "authServer", "clientId", "clientSecret")

	AddBackupCommand(rootCmd, &flags)
	AddBrokersCommand(rootCmd, &flags)
	AddConnectCmd(rootCmd, &flags)
	AddDatalossSimulationCmd(rootCmd, &flags)
	AddDeployCmd(rootCmd, &flags)
	AddDisconnectCommand(rootCmd, &flags)
	AddExportingCmds(rootCmd, &flags)
	AddPublishCmd(rootCmd, &flags)
	AddRestartCmd(rootCmd, &flags)
	AddStressCmd(rootCmd, &flags)
	AddTerminateCommand(rootCmd, &flags)
	AddTopologyCmd(rootCmd, &flags)
	AddVerifyCommands(rootCmd, &flags)
	AddVersionCmd(rootCmd)
	AddWorkerCmd(rootCmd)
	AddClusterCommands(rootCmd, &flags)

	return rootCmd
}

func createK8ClientWithFlags(flags *Flags) (internal.K8Client, error) {
	return internal.CreateK8Client(flags.kubeConfigPath, flags.namespace)
}

func Execute() {
	if err := NewCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func makeClientCredentials(flags *Flags) *internal.ClientCredentials {
	// check only one of the auth flags, as they are `MarkFlagsRequiredTogether`
	if flags.clientSecret == "" {
		return nil
	}
	credentials := &internal.ClientCredentials{
		AuthServer:   flags.authServer,
		Audience:     flags.audience,
		ClientID:     flags.clientId,
		ClientSecret: flags.clientSecret,
	}

	return credentials
}
