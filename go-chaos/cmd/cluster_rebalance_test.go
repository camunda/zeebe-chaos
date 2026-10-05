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
	partitionNotRebalanced := []rebalancePartitionOperation{
		{PartitionID: 1, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "TRANSFERRED"},
		{PartitionID: 2, PhysicalTenantID: "default", Progress: "COMPLETED", Result: "PHYSICAL_TENANT_DISABLED"},
	}

	tests := map[string]struct {
		completed     *completedRebalance
		wantErr       string
		wantPartition string
	}{
		"every partition transferred or already leader succeeds": {
			completed: &completedRebalance{RebalanceID: 7, Result: rebalanceResultCompleted, Partitions: transferred},
		},
		"a partition with another result fails": {
			completed:     &completedRebalance{RebalanceID: 7, Result: rebalanceResultCompleted, Partitions: partitionNotRebalanced},
			wantErr:       "rebalance 7 finished as COMPLETED but partition 2 of default was not rebalanced",
			wantPartition: "partition 2 of default: progress COMPLETED, result PHYSICAL_TENANT_DISABLED",
		},
		"a non-completed aggregate result fails": {
			completed:     &completedRebalance{RebalanceID: 7, Result: "FAILED", Partitions: transferred},
			wantErr:       "rebalance 7 finished as FAILED",
			wantPartition: "partition 1 of default: progress COMPLETED, result TRANSFERRED",
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
