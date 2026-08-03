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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ShouldAcceptARebalanceThatReachedEveryPartition(t *testing.T) {
	// given
	completed := &CompletedRebalance{RebalanceId: 1, Outcome: "COMPLETED", Partitions: []PartitionRebalanceStatus{
		{Id: 1, Status: "TRANSFERRED"},
		{Id: 2, Status: "SKIPPED"},
	}}

	// when
	err := describeRebalanceOutcome(completed, false, false)

	// then
	assert.NoError(t, err)
}

func Test_ShouldRejectAPartitionTheRebalanceCouldNotMove(t *testing.T) {
	// given
	completed := &CompletedRebalance{RebalanceId: 1, Outcome: "COMPLETED", Partitions: []PartitionRebalanceStatus{
		{Id: 1, Status: "TRANSFERRED"},
		{Id: 2, Status: "FAILED", PhysicalTenantId: "default", Reason: "LAG_TOO_HIGH"},
	}}

	// when
	err := describeRebalanceOutcome(completed, false, false)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LAG_TOO_HIGH")
}

func Test_ShouldAcceptAFailedPartitionWhenOneIsExpected(t *testing.T) {
	// given
	completed := &CompletedRebalance{RebalanceId: 1, Outcome: "COMPLETED", Partitions: []PartitionRebalanceStatus{
		{Id: 2, Status: "FAILED", PhysicalTenantId: "default", Reason: "NOT_REPLICATING"},
	}}

	// when
	err := describeRebalanceOutcome(completed, false, true)

	// then
	assert.NoError(t, err)
}

func Test_ShouldRejectARebalanceThatDidNotComplete(t *testing.T) {
	// given
	completed := &CompletedRebalance{RebalanceId: 1, Outcome: "CANCELLED"}

	// when
	err := describeRebalanceOutcome(completed, false, false)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ended CANCELLED, expected COMPLETED")
}

func Test_ShouldExpectACancelledRebalanceWhenItWasCancelled(t *testing.T) {
	// given a rebalance the caller asked to stop leaves the partitions after
	// the in-flight one untouched, so they are reported PENDING
	completed := &CompletedRebalance{RebalanceId: 1, Outcome: "CANCELLED", Partitions: []PartitionRebalanceStatus{
		{Id: 1, Status: "TRANSFERRED"},
		{Id: 2, Status: "PENDING"},
	}}

	// when
	err := describeRebalanceOutcome(completed, true, false)

	// then
	assert.NoError(t, err)
}

func Test_ShouldRejectARebalanceTheCoordinatorNeverReported(t *testing.T) {
	// when a coordinator that took over mid-rebalance reports no history
	err := describeRebalanceOutcome(nil, false, false)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no completed rebalance")
}

func Test_ShouldOnlySendOverriddenSettings(t *testing.T) {
	// given
	flags := &Flags{
		rebalanceDryRun:              true,
		rebalanceLagThreshold:        -1,
		rebalanceMaxTransferAttempts: 1,
		rebalanceReplicationTimeout:  "PT5S",
	}

	// when
	request := rebalanceRequestFromFlags(flags)

	// then the settings left out fall back to what each leader is configured with
	assert.True(t, request.DryRun)
	assert.Nil(t, request.ReplicationLagThreshold)
	assert.Nil(t, request.LeaderWaitTimeout)
	require.NotNil(t, request.MaxTransferAttempts)
	assert.Equal(t, int32(1), *request.MaxTransferAttempts)
	require.NotNil(t, request.ReplicationTimeout)
	assert.Equal(t, "PT5S", *request.ReplicationTimeout)
}

func Test_ShouldAllowAZeroLagThresholdToBeOverridden(t *testing.T) {
	// given a threshold of zero is a meaningful override rather than an absent one
	flags := &Flags{rebalanceLagThreshold: 0, rebalanceMaxTransferAttempts: -1}

	// when
	request := rebalanceRequestFromFlags(flags)

	// then
	require.NotNil(t, request.ReplicationLagThreshold)
	assert.Equal(t, int64(0), *request.ReplicationLagThreshold)
}
