// Copyright 2025 Camunda Services GmbH
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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(api *httptest.Server) *rebalanceClient {
	return &rebalanceClient{
		baseURL:          api.URL,
		httpClient:       api.Client(),
		pollInterval:     time.Millisecond,
		retryInterval:    time.Millisecond,
		unreachableAfter: 10 * time.Millisecond,
	}
}

func balanceBody(body string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}
}

func awaitWithTimeout(client *rebalanceClient, rebalanceID int64, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.awaitRebalance(ctx, rebalanceID)
}

type rebalanceAPI struct {
	posts int
	gets  int
}

func (a *rebalanceAPI) serve(getBody string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			a.posts++
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"runningRebalance":{"rebalanceId":7}}`))
		case http.MethodGet:
			a.gets++
			_, _ = writer.Write([]byte(getBody))
		}
	}
}

func Test_RebalanceClientRequiresCredentials(t *testing.T) {
	// when
	client, err := newRebalanceClient(context.Background(), "http://localhost:8080", nil)

	// then
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--clientSecret")
}

func Test_TriggerRetriesUnavailableCoordinatorThenSucceeds(t *testing.T) {
	// given
	var attempts int
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		if attempts < 3 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"runningRebalance":{"rebalanceId":7}}`))
	}))
	defer api.Close()
	client := newTestClient(api)

	// when
	rebalanceID, err := client.trigger(context.Background())

	// then
	require.NoError(t, err)
	assert.Equal(t, int64(7), rebalanceID)
	assert.Equal(t, 3, attempts)
}

func Test_AwaitRebalanceIgnoresOlderCompletedRebalance(t *testing.T) {
	// given
	older := `{"runningRebalance":{"rebalanceId":7},"lastCompletedRebalance":{"rebalanceId":6,"result":"COMPLETED"}}`
	matching := `{"lastCompletedRebalance":{"rebalanceId":7,"result":"COMPLETED"}}`
	var gets int
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gets++
		body := older
		if gets > 2 {
			body = matching
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
	defer api.Close()
	client := newTestClient(api)

	// when
	err := awaitWithTimeout(client, 7, time.Minute)

	// then
	require.NoError(t, err)
	assert.Equal(t, 3, gets)
}

func Test_AwaitRebalanceRetriesTransientPollFailures(t *testing.T) {
	// given
	var polls int
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		polls++
		if polls < 2 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"lastCompletedRebalance":{"rebalanceId":7,"result":"COMPLETED"}}`))
	}))
	defer api.Close()
	client := newTestClient(api)

	// when
	err := awaitWithTimeout(client, 7, time.Minute)

	// then
	require.NoError(t, err)
	assert.Equal(t, 2, polls)
}

func Test_AwaitRebalanceFailsWhenTheClusterBecomesUnreachable(t *testing.T) {
	// given
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := newTestClient(api)
	api.Close()

	// when
	err := awaitWithTimeout(client, 7, time.Minute)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebalance 7 can no longer be polled")
	assert.Contains(t, err.Error(), "unreachable for")
}

func Test_AwaitRebalanceStopsWhenCallerCancels(t *testing.T) {
	// given
	ctx, cancel := context.WithCancel(context.Background())
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cancel()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"runningRebalance":{"rebalanceId":7}}`))
	}))
	defer api.Close()
	client := newTestClient(api)

	// when
	err := client.awaitRebalance(ctx, 7)

	// then
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func Test_AwaitRebalanceFailsWhenRebalanceIsNotTracked(t *testing.T) {
	// given
	api := httptest.NewServer(balanceBody(`{}`))
	defer api.Close()
	client := newTestClient(api)

	// when
	err := awaitWithTimeout(client, 7, time.Minute)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was never observed completing")
}

func Test_RebalanceOutcome(t *testing.T) {
	transferred := []rebalancePartitionOperation{
		{PartitionID: 1, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "TRANSFERRED"},
		{PartitionID: 2, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "ALREADY_LEADER"},
	}
	partitionSkipped := []rebalancePartitionOperation{
		{PartitionID: 1, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "TRANSFERRED"},
		{PartitionID: 2, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "LAG_TOO_HIGH"},
	}

	tests := map[string]struct {
		completed     *completedRebalance
		wantErr       string
		wantPartition string
	}{
		"every partition transferred or already leader succeeds": {
			completed: &completedRebalance{RebalanceID: 7, Result: rebalanceResultCompleted, Partitions: transferred},
		},
		"a skipped partition still succeeds": {
			completed: &completedRebalance{RebalanceID: 7, Result: rebalanceResultCompleted, Partitions: partitionSkipped},
		},
		"a failed rebalance fails": {
			completed:     &completedRebalance{RebalanceID: 7, Result: "FAILED", Partitions: transferred},
			wantErr:       "rebalance 7 finished as FAILED",
			wantPartition: "partition 1 of default: progress COMPLETED, result TRANSFERRED",
		},
		"a cancelled rebalance fails": {
			completed:     &completedRebalance{RebalanceID: 7, Result: "CANCELLED", Partitions: partitionSkipped},
			wantErr:       "rebalance 7 finished as CANCELLED",
			wantPartition: "partition 2 of default: progress COMPLETED, result LAG_TOO_HIGH",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// when
			err := rebalanceOutcome(tc.completed)

			// then
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Contains(t, err.Error(), tc.wantPartition)
		})
	}
}

func Test_RebalanceNoWaitReturnsOnceAccepted(t *testing.T) {
	// given
	api := &rebalanceAPI{}
	server := httptest.NewServer(api.serve(`{"runningRebalance":{"rebalanceId":7}}`))
	defer server.Close()
	client := newTestClient(server)

	// when
	err := client.rebalance(context.Background(), true)

	// then
	require.NoError(t, err)
	assert.Equal(t, 1, api.posts)
	assert.Equal(t, 0, api.gets)
}

func Test_RebalanceWaitSucceedsWhenCompletedWithSkippedPartition(t *testing.T) {
	// given
	api := &rebalanceAPI{}
	server := httptest.NewServer(api.serve(`{"lastCompletedRebalance":{"rebalanceId":7,"result":"COMPLETED","partitions":[
		{"partitionId":1,"physicalTenantId":"default","progress":"COMPLETED","result":"TRANSFERRED"},
		{"partitionId":2,"physicalTenantId":"default","progress":"COMPLETED","result":"NO_LEADER"}]}}`))
	defer server.Close()
	client := newTestClient(server)

	// when
	err := client.rebalance(context.Background(), false)

	// then
	require.NoError(t, err)
	assert.Equal(t, 1, api.posts)
	assert.Equal(t, 1, api.gets)
}

func Test_RebalanceWaitFailsWhenRebalanceFailed(t *testing.T) {
	// given
	api := &rebalanceAPI{}
	server := httptest.NewServer(api.serve(`{"lastCompletedRebalance":{"rebalanceId":7,"result":"FAILED","partitions":[
		{"partitionId":1,"physicalTenantId":"default","progress":"COMPLETED","result":"UNREACHABLE"}]}}`))
	defer server.Close()
	client := newTestClient(server)

	// when
	err := client.rebalance(context.Background(), false)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebalance 7 finished as FAILED")
	assert.Contains(t, err.Error(), "partition 1 of default: progress COMPLETED, result UNREACHABLE")
}

func Test_AwaitBalancedSucceedsOnceBalanced(t *testing.T) {
	// given
	var gets int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gets++
		state := "BALANCING"
		if gets > 2 {
			state = "BALANCED"
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"state":"` + state + `"}`))
	}))
	defer server.Close()
	client := newTestClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// when
	err := client.awaitBalanced(ctx)

	// then
	require.NoError(t, err)
	assert.Equal(t, 3, gets)
}

func Test_AwaitBalancedFailsWithSummaryOnTimeout(t *testing.T) {
	// given
	server := httptest.NewServer(balanceBody(`{"state":"UNBALANCED",
		"partitions":[
			{"partitionId":1,"physicalTenantId":"default","currentLeader":"1","desiredLeader":"0","state":"UNBALANCED"},
			{"partitionId":2,"physicalTenantId":"default","currentLeader":null,"desiredLeader":"1","state":"UNBALANCED"}],
		"lastCompletedRebalance":{"rebalanceId":7,"result":"CANCELLED","partitions":[
			{"partitionId":1,"physicalTenantId":"default","progress":"PENDING"}]}}`))
	defer server.Close()
	client := newTestClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// when
	err := client.awaitBalanced(ctx)

	// then
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "cluster did not become BALANCED")
	assert.Contains(t, err.Error(), "last observed state UNBALANCED")
	assert.Contains(t, err.Error(), "partition 1 of default: state UNBALANCED, leader 1, desired leader 0")
	assert.Contains(t, err.Error(), "partition 2 of default: state UNBALANCED, leader none, desired leader 1")
	assert.Contains(t, err.Error(), "last completed rebalance: rebalance 7 finished as CANCELLED (partition 1 of default: progress PENDING, result none)")
}
