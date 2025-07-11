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
	"github.com/camunda/zeebe-chaos/go-chaos/backend"
	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

func AddDeployCmd(rootCmd *cobra.Command, flags *Flags) {
	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy certain resource",
		Long:  `Deploy certain resource, like process model(s) or kubernetes manifest.`,
	}

	deployProcessModelCmd := &cobra.Command{
		Use:   "process",
		Short: "Deploy a process model to Zeebe",
		Long: `Deploy a process model to Zeebe. 
Can be used to deploy a specific process model or multiple version of a default BPMN and DMN model.
Defaults to the later, which is useful for experimenting with deployment distribution.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
			defer closeFn()

			zbClient, err := internal.CreateZeebeClient(port, makeClientCredentials(flags))
			ensureNoError(err)
			defer zbClient.Close()

			processDefinitionKey, err := internal.DeployModel(zbClient, flags.processModelPath)
			ensureNoError(err)

			internal.LogInfo("Deployed given process model %s, under key %d!", flags.processModelPath, processDefinitionKey)
		},
	}

	deployMultiVersionProcessModelCmd := &cobra.Command{
		Use:   "multi-version",
		Short: "Deploy multiple versions to Zeebe",
		Long: `Deploy multiple versions of process and dmn models to Zeebe.
Useful for experimenting with deployment distribution.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			port, closeFn := k8Client.MustGatewayPortForward(0, 26500)
			defer closeFn()

			zbClient, err := internal.CreateZeebeClient(port, makeClientCredentials(flags))
			ensureNoError(err)
			defer zbClient.Close()

			err = internal.DeployDifferentVersions(zbClient, int32(flags.versionCount))
			ensureNoError(err)
			internal.LogInfo("Deployed different process models of different types and versions to zeebe!")
		},
	}

	deployWorkerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Deploy a worker deployment to the Zeebe cluster",
		Long: `Deploy a worker deployment to the Zeebe cluster. 
The workers can be used as part of some chaos experiments to complete process instances etc.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			credentials := makeClientCredentials(flags)
			err = k8Client.CreateWorkerDeployment(DockerImageTag, flags.pollingDelayMs, credentials)
			ensureNoError(err)

			internal.LogInfo("Worker successfully deployed to the current namespace: %s", k8Client.GetCurrentNamespace())
		},
	}

	deployChaosModels := &cobra.Command{
		Use:   "chaos",
		Short: "Deploy all chaos BPMN models to the Zeebe cluster",
		Long: `Deploy all chaos BPMN models to the to the Zeebe cluster. 
The process models allow to execute chaos experiments.`,
		Run: func(cmd *cobra.Command, args []string) {
			k8Client, err := createK8ClientWithFlags(flags)
			ensureNoError(err)

			credentials := makeClientCredentials(flags)
			zbClient, closeFn, err := backend.ConnectToZeebeCluster(k8Client, credentials)
			ensureNoError(err)
			defer closeFn()

			err = internal.DeployChaosModels(zbClient)
			ensureNoError(err)

			internal.LogInfo("Deployed successfully process models to run chaos experiments")
		},
	}

	rootCmd.AddCommand(deployCmd)

	deployCmd.AddCommand(deployProcessModelCmd)
	deployProcessModelCmd.Flags().StringVar(&flags.processModelPath, "processModelPath", "",
		"Specify the path to a BPMN process model, which should be deployed. Defaults to a benchmark process model with one task (included in zbchaos). If the path starts with 'bpmn/' zbchaos will look for a referenced model bundled within the cli, like: 'bpmn/one_task.bpmn'.")

	deployCmd.AddCommand(deployMultiVersionProcessModelCmd)
	deployMultiVersionProcessModelCmd.Flags().IntVar(&flags.versionCount, "versionCount", 10,
		"Specify how many different versions of a default BPMN and DMN model should be deployed. Useful for testing deployment distribution.")

	deployCmd.AddCommand(deployWorkerCmd)
	deployWorkerCmd.Flags().IntVar(&flags.pollingDelayMs, "pollingDelay", 1, "Specifies the worker's polling interval in milliseconds")

	deployCmd.AddCommand(deployChaosModels)
}
