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
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

type BackupType string

const (
	PRIMARY_STORAGE   BackupType = "runtime"
	SECONDARY_STORAGE BackupType = "history"
)

func AddBackupCommand(rootCmd *cobra.Command, flags *Flags) {
	backupCommand := &cobra.Command{
		Use:   "backup",
		Short: "Controls Zeebe runtime backups",
		Long:  "Creates and monitors Orchestration Cluster backups through the REST API.",
	}
	setupBackupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Configure legacy S3 backup settings on a Zeebe cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupBackup(flags.kubeConfigPath, flags.namespace)
		},
	}
	takeBackupCommand := &cobra.Command{
		Use:   "take",
		Short: "Trigger a runtime backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return takeBackup(flags)
		},
	}
	waitForBackupCommand := &cobra.Command{
		Use:   "wait",
		Short: "Wait for a runtime backup to complete",
		RunE: func(cmd *cobra.Command, args []string) error {
			return waitForBackup(flags)
		},
	}

	rootCmd.AddCommand(backupCommand)
	backupCommand.AddCommand(setupBackupCommand)
	backupCommand.AddCommand(takeBackupCommand)
	takeBackupCommand.Flags().StringVar(&flags.backupId, "backupId", strconv.FormatInt(time.Now().UnixMilli(), 10), "backup ID to use")
	backupCommand.AddCommand(waitForBackupCommand)
	waitForBackupCommand.Flags().StringVar(&flags.backupId, "backupId", "", "backup ID to wait for")
}

func takeBackup(flags *Flags) error {
	return withBackupID(flags, true, func(client *orchestrationRESTClient, backupID int64) error {
		return client.takeBackup(PRIMARY_STORAGE, backupID)
	})
}

func waitForBackup(flags *Flags) error {
	return withBackupID(flags, false, func(client *orchestrationRESTClient, backupID int64) error {
		return client.waitForBackup(PRIMARY_STORAGE, backupID, 30*time.Minute)
	})
}

// withBackupID parses the required --backupId flag and runs the action against the
// Orchestration Cluster REST API, optionally waiting for the cluster to be ready first.
func withBackupID(flags *Flags, awaitReadiness bool, action func(client *orchestrationRESTClient, backupID int64) error) error {
	backupID, err := parseBackupID(flags.backupId)
	if err != nil {
		return err
	}
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}
	if awaitReadiness {
		if err := k8Client.AwaitReadiness(); err != nil {
			return err
		}
	}
	return withBackupAPI(k8Client, flags, func(client *orchestrationRESTClient) error {
		return action(client, backupID)
	})
}

func withBackupAPI(k8Client internal.K8Client, flags *Flags, action func(*orchestrationRESTClient) error) error {
	return withRESTClient(k8Client, flags, action)
}

func parseBackupID(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("backupId is required")
	}
	backupID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || backupID < 1 {
		return 0, fmt.Errorf("backupId must be a positive integer: %q", value)
	}
	return backupID, nil
}

type backupStatus struct {
	State         string `json:"state"`
	FailureReason string `json:"failureReason"`
}
