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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"github.com/spf13/cobra"
)

// Rebalance status values reported by GET /actuator/cluster/rebalance.
const (
	RebalanceStatusIdle       = "IDLE"
	RebalanceStatusRunning    = "RUNNING"
	RebalanceStatusCancelling = "CANCELLING"
)

// A coordinated rebalance is leadership-only, so it never registers as a
// ClusterConfiguration change and `cluster wait` cannot follow it. It is
// followed by polling its own status instead.
type RebalanceStatusResponse struct {
	Status                 string                     `json:"status"`
	RebalanceId            *int64                     `json:"rebalanceId,omitempty"`
	DryRun                 bool                       `json:"dryRun"`
	Settings               *RebalanceSettings         `json:"settings,omitempty"`
	Partitions             []PartitionRebalanceStatus `json:"partitions,omitempty"`
	LastCompletedRebalance *CompletedRebalance        `json:"lastCompletedRebalance,omitempty"`
}

type CompletedRebalance struct {
	RebalanceId int64                      `json:"rebalanceId"`
	Outcome     string                     `json:"outcome"`
	DryRun      bool                       `json:"dryRun"`
	Partitions  []PartitionRebalanceStatus `json:"partitions,omitempty"`
}

type PartitionRebalanceStatus struct {
	Id               int32  `json:"id"`
	PhysicalTenantId string `json:"physicalTenantId"`
	CurrentLeader    *int32 `json:"currentLeader,omitempty"`
	DesiredLeader    *int32 `json:"desiredLeader,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
}

type RebalanceSettings struct {
	ReplicationLagThreshold *int64  `json:"replicationLagThreshold,omitempty"`
	ReplicationTimeout      *string `json:"replicationTimeout,omitempty"`
	MaxTransferAttempts     *int32  `json:"maxTransferAttempts,omitempty"`
	LeaderWaitTimeout       *string `json:"leaderWaitTimeout,omitempty"`
}

// What one rebalance runs under. Every setting is optional and the ones left
// out fall back to what each partition's leader is configured with.
type RebalanceRequest struct {
	DryRun                  bool    `json:"dryRun,omitempty"`
	ReplicationLagThreshold *int64  `json:"replicationLagThreshold,omitempty"`
	ReplicationTimeout      *string `json:"replicationTimeout,omitempty"`
	MaxTransferAttempts     *int32  `json:"maxTransferAttempts,omitempty"`
	LeaderWaitTimeout       *string `json:"leaderWaitTimeout,omitempty"`
}

func addRebalanceCommand(clusterCommand *cobra.Command, flags *Flags) {
	rebalanceCommand := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalances partition leadership and waits for the rebalance to finish",
		Long: `Triggers a coordinated leadership rebalance and polls until it reports finished.

Every partition not led by its highest-priority replica has its leadership transferred towards
that replica, one partition at a time. Fails if the rebalance does not finish within the timeout,
if it ends in any outcome other than COMPLETED, or if any partition it worked on ended FAILED.`,
		// A failing rebalance is a finding rather than a misuse of the command,
		// so its usage is not printed over the reason it failed.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rebalanceCluster(flags)
		},
	}
	clusterCommand.AddCommand(rebalanceCommand)

	rebalanceCommand.Flags().BoolVar(&flags.rebalanceDryRun, "dryRun", false,
		"report the plan the rebalance would carry out without pausing any partition")
	rebalanceCommand.Flags().IntVar(&flags.rebalanceTimeoutSec, "timeoutInSec", 300,
		"how long to wait for the rebalance to finish")
	rebalanceCommand.Flags().IntVar(&flags.rebalanceCancelAfterSec, "cancelAfterSec", 0,
		"cancel the rebalance this long after it starts; 0 never cancels. A cancelled rebalance is expected to end CANCELLED")
	rebalanceCommand.Flags().StringVar(&flags.rebalanceOutputPath, "outputPath", "",
		"write the final rebalance status as JSON to this file, for evidence of what each partition ended as")
	rebalanceCommand.Flags().BoolVar(&flags.rebalanceAllowFailedPartitions, "allowFailedPartitions", false,
		"accept partitions the rebalance could not move, for scenarios where a transfer is meant to be refused")
	rebalanceCommand.Flags().Int64Var(&flags.rebalanceLagThreshold, "replicationLagThreshold", -1,
		"override how far behind the desired leader may be for a transfer to be admitted, in bytes")
	rebalanceCommand.Flags().StringVar(&flags.rebalanceReplicationTimeout, "replicationTimeout", "",
		"override how long a paused partition waits for the desired leader to catch up, as an ISO-8601 duration")
	rebalanceCommand.Flags().Int32Var(&flags.rebalanceMaxTransferAttempts, "maxTransferAttempts", -1,
		"override how many times a leader prompts the desired leader to take over")
	rebalanceCommand.Flags().StringVar(&flags.rebalanceLeaderWaitTimeout, "leaderWaitTimeout", "",
		"override how long the coordinator waits for a leader to report, as an ISO-8601 duration")
}

func rebalanceCluster(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}

	// Any broker forwards to whichever member coordinates, so broker 0 is
	// addressed rather than resolved.
	port, closePortForward := k8Client.MustGatewayPortForward(0, 9600)
	defer closePortForward()

	accepted, err := triggerRebalance(port, rebalanceRequestFromFlags(flags))
	if err != nil {
		return err
	}
	internal.LogInfo("Rebalance %s accepted (dryRun: %t)", rebalanceIdOf(accepted), accepted.DryRun)

	timeout := time.Duration(flags.rebalanceTimeoutSec) * time.Second
	var cancelAfter time.Duration
	if flags.rebalanceCancelAfterSec > 0 {
		cancelAfter = time.Duration(flags.rebalanceCancelAfterSec) * time.Second
	}

	status, err := awaitRebalance(port, timeout, cancelAfter)
	if err != nil {
		return err
	}

	if flags.rebalanceOutputPath != "" {
		if writeErr := writeRebalanceStatus(flags.rebalanceOutputPath, status); writeErr != nil {
			// The rebalance itself is what is under test, so failing to keep
			// its evidence is reported without discarding the verdict below.
			internal.LogInfo("Failed to write rebalance status to %s: %s", flags.rebalanceOutputPath, writeErr)
		}
	}

	return describeRebalanceOutcome(
		status.LastCompletedRebalance, flags.rebalanceCancelAfterSec > 0, flags.rebalanceAllowFailedPartitions)
}

func rebalanceRequestFromFlags(flags *Flags) RebalanceRequest {
	request := RebalanceRequest{DryRun: flags.rebalanceDryRun}
	if flags.rebalanceLagThreshold >= 0 {
		request.ReplicationLagThreshold = &flags.rebalanceLagThreshold
	}
	if flags.rebalanceReplicationTimeout != "" {
		request.ReplicationTimeout = &flags.rebalanceReplicationTimeout
	}
	if flags.rebalanceMaxTransferAttempts >= 0 {
		request.MaxTransferAttempts = &flags.rebalanceMaxTransferAttempts
	}
	if flags.rebalanceLeaderWaitTimeout != "" {
		request.LeaderWaitTimeout = &flags.rebalanceLeaderWaitTimeout
	}
	return request
}

func triggerRebalance(port int, request RebalanceRequest) (*RebalanceStatusResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	url := rebalanceUrl(port)
	internal.LogInfo("Requesting rebalance %s with input %s", url, string(body))
	return sendRebalanceRequest(url, "POST", body)
}

func queryRebalanceStatus(port int) (*RebalanceStatusResponse, error) {
	return sendRebalanceRequest(rebalanceUrl(port), "GET", nil)
}

func cancelRebalance(port int) error {
	_, err := sendRebalanceRequest(rebalanceUrl(port), "DELETE", nil)
	return err
}

func rebalanceUrl(port int) string {
	return fmt.Sprintf("http://localhost:%d/actuator/cluster/rebalance", port)
}

func sendRebalanceRequest(url, method string, body []byte) (*RebalanceStatusResponse, error) {
	resp, err := sendHTTPJsonRequest(url, method, body)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed with code %d: %s", method, url, resp.StatusCode, string(responseBody))
	}

	// DELETE answers with a cancellation response rather than a status, so an
	// unmarshalled zero value is expected there and the caller ignores it.
	var status RebalanceStatusResponse
	if err := json.Unmarshal(responseBody, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// awaitRebalance polls until the coordinator reports itself idle, cancelling on
// the way when asked to. A rebalance is often over inside a second, so the
// poll interval is short enough to see a running one at all.
func awaitRebalance(port int, timeout time.Duration, cancelAfter time.Duration) (*RebalanceStatusResponse, error) {
	interval := time.Millisecond * 500
	started := time.Now()
	cancelled := cancelAfter <= 0
	var lastStatus *RebalanceStatusResponse

	for time.Since(started) < timeout {
		status, err := queryRebalanceStatus(port)
		if err != nil {
			// A coordinator that moved mid-rebalance answers 503 until the new
			// one is known, which is a state to poll through rather than fail on.
			internal.LogInfo("Failed to query rebalance status: %s", err)
			time.Sleep(interval)
			continue
		}
		lastStatus = status

		if !cancelled && time.Since(started) >= cancelAfter {
			internal.LogInfo("Cancelling the running rebalance after %s", time.Since(started).Round(time.Millisecond))
			if err := cancelRebalance(port); err != nil {
				return nil, err
			}
			cancelled = true
		}

		if status.Status == RebalanceStatusIdle {
			internal.LogInfo("Rebalance finished after %s", time.Since(started).Round(time.Millisecond))
			return status, nil
		}
		internal.LogVerbose("Rebalance %s is %s, %s",
			rebalanceIdOf(status), status.Status, describePartitionStates(status.Partitions))
		time.Sleep(interval)
	}

	return lastStatus, fmt.Errorf("rebalance did not finish within %s, last status was %s", timeout, statusOf(lastStatus))
}

// describeRebalanceOutcome turns what the coordinator recorded into a verdict.
// A partition it wanted to move and could not is reported FAILED, which is the
// difference between "tried and could not" and "nothing to do" and so is a
// failure of the experiment rather than a note.
func describeRebalanceOutcome(
	completed *CompletedRebalance, expectCancelled bool, allowFailedPartitions bool) error {
	if completed == nil {
		return fmt.Errorf("the coordinator reported no completed rebalance; it may have moved to another member mid-rebalance")
	}

	expectedOutcome := "COMPLETED"
	if expectCancelled {
		expectedOutcome = "CANCELLED"
	}

	failed := make([]string, 0)
	for _, partition := range completed.Partitions {
		if partition.Status == "FAILED" {
			failed = append(failed, fmt.Sprintf("partition %d of %s: %s",
				partition.Id, partition.PhysicalTenantId, partition.Reason))
		}
	}

	internal.LogInfo("Rebalance %d ended %s: %s",
		completed.RebalanceId, completed.Outcome, describePartitionStates(completed.Partitions))

	if completed.Outcome != expectedOutcome {
		return fmt.Errorf("rebalance %d ended %s, expected %s", completed.RebalanceId, completed.Outcome, expectedOutcome)
	}
	if len(failed) > 0 {
		if !allowFailedPartitions {
			return fmt.Errorf("rebalance %d could not move %d partitions: %v", completed.RebalanceId, len(failed), failed)
		}
		internal.LogInfo("Rebalance %d could not move %d partitions: %v", completed.RebalanceId, len(failed), failed)
	}
	return nil
}

func describePartitionStates(partitions []PartitionRebalanceStatus) string {
	if len(partitions) == 0 {
		return "no partitions reported"
	}
	counts := make(map[string]int, len(partitions))
	for _, partition := range partitions {
		counts[partition.Status]++
	}
	return fmt.Sprintf("%v", counts)
}

func writeRebalanceStatus(path string, status *RebalanceStatusResponse) error {
	formatted, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(formatted, '\n'), 0644)
}

func rebalanceIdOf(status *RebalanceStatusResponse) string {
	if status == nil {
		return "<unknown>"
	}
	if status.RebalanceId != nil {
		return fmt.Sprintf("%d", *status.RebalanceId)
	}
	if status.LastCompletedRebalance != nil {
		return fmt.Sprintf("%d", status.LastCompletedRebalance.RebalanceId)
	}
	return "<unknown>"
}

func statusOf(status *RebalanceStatusResponse) string {
	if status == nil {
		return "never reported"
	}
	return status.Status
}
