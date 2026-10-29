// Copyright 2026 Camunda Services GmbH
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/spf13/cobra"
)

type Mode string

const (
	PROCESSING Mode = "PROCESSING"
	RECOVERING Mode = "RECOVERING"
)

func parseMode(value string) (Mode, error) {
	switch mode := Mode(value); mode {
	case PROCESSING, RECOVERING:
		return mode, nil
	default:
		return "", fmt.Errorf("mode must be one of %s, %s: %q", PROCESSING, RECOVERING, value)
	}
}

func AddRecoveryCommands(rootCmd *cobra.Command, flags *Flags) {
	recoveryCommand := &cobra.Command{
		Use:   "recovery",
		Short: "Restore ",
		Long:  "In",
	}
	modeChangeCommand := &cobra.Command{
		Use:   "mode",
		Short: "Change the mode of the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return changeClusterMode(flags)
		},
	}
	restoreCommand := &cobra.Command{
		Use:   "restore",
		Short: "Restore a runtime backup while the cluster is in recovery mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return restoreBackup(flags)
		},
	}

	restoreLegacyCommand := &cobra.Command{
		Use:   "restore-legacy",
		Short: "Restore a cluster configured via 'backup setup' using restore init containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return restoreFromBackupLegacy(flags)
		},
	}

	rootCmd.AddCommand(recoveryCommand)
	recoveryCommand.AddCommand(modeChangeCommand)
	recoveryCommand.AddCommand(restoreCommand)
	modeChangeCommand.Flags().StringVar(&flags.mode, "mode", "", fmt.Sprintf("mode to switch the cluster to (%s, %s)", PROCESSING, RECOVERING))
	_ = modeChangeCommand.MarkFlagRequired("mode")
	restoreCommand.Flags().StringVar(&flags.backupId, "backupId", "", "backup ID to restore")
	_ = restoreCommand.MarkFlagRequired("backupId")
	recoveryCommand.AddCommand(restoreLegacyCommand)
	restoreLegacyCommand.Flags().StringVar(&flags.backupId, "backupId", "", "backup ID to restore")
	_ = restoreLegacyCommand.MarkFlagRequired("backupId")
}

func changeClusterMode(flags *Flags) error {
	mode, err := parseMode(flags.mode)
	if err != nil {
		return err
	}
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}
	return withRESTClient(k8Client, flags, func(client *orchestrationRESTClient) error {
		changeID, err := client.changeMode(mode)
		if err != nil {
			return err
		}
		return client.waitForChange(changeID, 15*time.Minute)
	})
}

func restoreBackup(flags *Flags) error {
	return withBackupID(flags, false, func(client *orchestrationRESTClient, backupID int64) error {
		restoreChangeID, err := client.restore(backupID)
		if err != nil {
			return err
		}
		if err := client.waitForRestore(restoreChangeID, time.Hour); err != nil {
			return err
		}
		return client.waitForChange(restoreChangeID, time.Hour)
	})
}

func withRESTClient(k8Client internal.K8Client, flags *Flags, action func(*orchestrationRESTClient) error) error {
	port, closePortForward := k8Client.MustGatewayPortForward(0, 8080)
	defer closePortForward()
	client, err := newOrchestrationRESTClient(port, makeClientCredentials(flags))
	if err != nil {
		return err
	}
	return action(client)
}

type orchestrationRESTClient struct {
	baseURL     string
	httpClient  *http.Client
	credentials *zbc.OAuthCredentialsProvider
	// interval between status polls, defaults to 5 seconds when zero
	pollEvery time.Duration
}

func (client *orchestrationRESTClient) pollInterval() time.Duration {
	if client.pollEvery > 0 {
		return client.pollEvery
	}
	return 5 * time.Second
}

func newOrchestrationRESTClient(port int, credentials *internal.ClientCredentials) (*orchestrationRESTClient, error) {
	client := &orchestrationRESTClient{baseURL: fmt.Sprintf("http://localhost:%d/v2", port), httpClient: http.DefaultClient}
	if credentials == nil {
		return client, nil
	}
	provider, err := zbc.NewOAuthCredentialsProvider(&zbc.OAuthProviderConfig{
		Audience: credentials.Audience, AuthorizationServerURL: credentials.AuthServer, ClientID: credentials.ClientId, ClientSecret: credentials.ClientSecret,
	})
	if err != nil {
		return nil, err
	}
	client.credentials = provider
	return client, nil
}

func (client *orchestrationRESTClient) changeMode(mode Mode) (string, error) {
	var response changeResponse
	_, err := client.request(context.TODO(), http.MethodPatch, "/mode?mode="+string(mode), nil, http.StatusOK, &response)
	return response.ChangeID, err
}

func (client *orchestrationRESTClient) restore(backupID int64) (string, error) {
	var response changeResponse
	_, err := client.request(context.TODO(), http.MethodPost, "/restore", map[string][]int64{"backupIds": {backupID}}, http.StatusAccepted, &response)
	return response.ChangeID, err
}

func (client *orchestrationRESTClient) takeBackup(backupType BackupType, backupID int64) error {
	_, err := client.request(context.TODO(), http.MethodPost, "/backups/"+string(backupType), map[string]int64{"backupId": backupID}, http.StatusAccepted, nil)
	return err
}

func (client *orchestrationRESTClient) waitForBackup(backupType BackupType, backupID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var backup backupStatus
		if _, err := client.request(context.TODO(), http.MethodGet, fmt.Sprintf("/backups/%s/%d", backupType, backupID), nil, http.StatusOK, &backup); err != nil {
			return err
		}
		switch backup.State {
		case "COMPLETED":
			internal.LogInfo("%s backup %d completed", backupType, backupID)
			return nil
		case "FAILED", "DOES_NOT_EXIST":
			return fmt.Errorf("%s backup %d ended in state %s: %s", backupType, backupID, backup.State, backup.FailureReason)
		}
		time.Sleep(client.pollInterval())
	}
	return fmt.Errorf("%s backup %d did not complete within %s", backupType, backupID, timeout)
}

func (client *orchestrationRESTClient) waitForRestore(changeID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var status restoreStatus
		statusCode, err := client.request(context.TODO(), http.MethodGet, "/restore", nil, http.StatusOK, &status)
		if err != nil {
			var httpErr *httpStatusError
			if errors.As(err, &httpErr) && statusCode == http.StatusNotFound {
				return nil
			}
			return err
		}
		if status.ChangeID != changeID {
			return fmt.Errorf("restore status belongs to change %s instead of %s", status.ChangeID, changeID)
		}
		switch status.Status {
		case "COMPLETED":
			return nil
		case "FAILED", "CANCELLED":
			return fmt.Errorf("restore change %s ended in state %s", changeID, status.Status)
		}
		time.Sleep(client.pollInterval())
	}
	return fmt.Errorf("restore change %s did not complete within %s", changeID, timeout)
}

func (client *orchestrationRESTClient) waitForChange(changeID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var topology restTopology
		_, err := client.request(context.TODO(), http.MethodGet, "/topology", nil, http.StatusOK, &topology)
		if err != nil {
			return err
		}
		if topology.LastCompletedChangeID == changeID {
			return nil
		}
		time.Sleep(client.pollInterval())
	}
	return fmt.Errorf("cluster change %s did not complete within %s", changeID, timeout)
}

func (client *orchestrationRESTClient) request(ctx context.Context, method string, path string, payload any, expectedStatus int, response any) (int, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, body)
	if err != nil {
		return 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if client.credentials != nil {
		headers := map[string]string{}
		if err := client.credentials.ApplyCredentials(ctx, headers); err != nil {
			return 0, err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode != expectedStatus {
		return resp.StatusCode, &httpStatusError{StatusCode: resp.StatusCode, Body: responseBody}
	}
	if response != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, response); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

type changeResponse struct {
	ChangeID string `json:"changeId"`
}

type restoreStatus struct {
	Status   string `json:"status"`
	ChangeID string `json:"changeId"`
}

type restTopology struct {
	LastCompletedChangeID string `json:"lastCompletedChangeId"`
}
