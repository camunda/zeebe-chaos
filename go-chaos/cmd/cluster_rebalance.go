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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	rebalancePath = "/cluster/v2/rebalance"

	rebalanceTimeout          = time.Minute * 15
	rebalanceRequestTimeout   = time.Second * 30
	rebalancePollInterval     = time.Second
	rebalanceRetryInterval    = time.Second * 5
	rebalanceUnreachableAfter = time.Second * 15

	maxErrorBodyBytes = 4096

	rebalanceResultCompleted   = "COMPLETED"
	rebalanceProgressCompleted = "COMPLETED"
)

func rebalanceCluster(ctx context.Context, flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}

	port, closePortForward := k8Client.MustGatewayPortForward(0, 8080)
	defer closePortForward()

	ctx, cancel := context.WithTimeout(ctx, rebalanceTimeout)
	defer cancel()

	client, err := newRebalanceClient(ctx, fmt.Sprintf("http://localhost:%d", port), makeClientCredentials(flags))
	if err != nil {
		return err
	}

	rebalanceID, err := client.trigger(ctx)
	if err != nil {
		return err
	}
	internal.LogInfo("Requested rebalance %d", rebalanceID)

	return client.awaitRebalance(ctx, rebalanceID)
}

type rebalanceClient struct {
	baseURL          string
	httpClient       *http.Client
	pollInterval     time.Duration
	retryInterval    time.Duration
	unreachableAfter time.Duration
}

func newRebalanceClient(ctx context.Context, baseURL string, credentials *internal.ClientCredentials) (*rebalanceClient, error) {
	if credentials == nil {
		return nil, errors.New("missing credentials: pass --authServer, --audience, --clientId and --clientSecret")
	}

	config := clientcredentials.Config{
		ClientID:       credentials.ClientId,
		ClientSecret:   credentials.ClientSecret,
		TokenURL:       credentials.AuthServer,
		EndpointParams: url.Values{"audience": []string{credentials.Audience}},
		AuthStyle:      oauth2.AuthStyleInParams,
	}

	tokenContext := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: rebalanceRequestTimeout})
	httpClient := config.Client(tokenContext)
	httpClient.Timeout = rebalanceRequestTimeout

	return &rebalanceClient{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		httpClient:       httpClient,
		pollInterval:     rebalancePollInterval,
		retryInterval:    rebalanceRetryInterval,
		unreachableAfter: rebalanceUnreachableAfter,
	}, nil
}

func (c *rebalanceClient) trigger(ctx context.Context) (int64, error) {
	for attempt := 1; ; attempt++ {
		balance, err := c.postRebalance(ctx)
		if err == nil {
			if balance.RunningRebalance == nil || balance.RunningRebalance.RebalanceID == 0 {
				return 0, errors.New("rebalance response did not contain a rebalance id")
			}
			return balance.RunningRebalance.RebalanceID, nil
		}

		reason, retryable := retryableTriggerError(err)
		if !retryable {
			if ambiguousTriggerError(err) {
				return 0, fmt.Errorf("could not confirm whether a rebalance was started, check GET %s: %w", rebalancePath, err)
			}
			return 0, err
		}

		internal.LogInfo("Cannot start a rebalance (%s), retrying (attempt %d)", reason, attempt)
		internal.LogVerbose("%s", err)
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("could not start a rebalance after %d attempts, %s: %w (%w)", attempt, reason, err, ctx.Err())
		case <-time.After(c.retryInterval):
		}
	}
}

func (c *rebalanceClient) awaitRebalance(ctx context.Context, rebalanceID int64) error {
	started := time.Now()
	var unreachableSince time.Time
	lastProgress := ""

	for {
		balance, err := c.getRebalance(ctx)
		if err != nil {
			var transportErr *rebalanceTransportError
			if errors.As(err, &transportErr) {
				if unreachableSince.IsZero() {
					unreachableSince = time.Now()
				}
				if unreachable := time.Since(unreachableSince); unreachable >= c.unreachableAfter {
					return fmt.Errorf("rebalance %d can no longer be polled, %s: %w", rebalanceID, describeUnreachable(c.baseURL, unreachable), err)
				}
			} else {
				unreachableSince = time.Time{}
				if !retryablePollError(err) {
					return err
				}
			}
			internal.LogInfo("Failed to query cluster balance: %s", err)
		} else {
			unreachableSince = time.Time{}

			completed := balance.LastCompletedRebalance
			running := balance.RunningRebalance
			switch {
			case completed != nil && completed.RebalanceID == rebalanceID:
				return rebalanceOutcome(completed)
			case running != nil && running.RebalanceID == rebalanceID:
				if progress := describeRebalanceProgress(running); progress != lastProgress {
					internal.LogInfo("Rebalance %d is running with %s", rebalanceID, progress)
					lastProgress = progress
				}
			default:
				return unobservedRebalanceError(rebalanceID, running, completed)
			}
		}

		internal.LogVerbose("Waiting %s before checking again", c.pollInterval)
		select {
		case <-ctx.Done():
			return fmt.Errorf("rebalance %d did not complete within %s: %w", rebalanceID, time.Since(started).Truncate(time.Second), ctx.Err())
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *rebalanceClient) postRebalance(ctx context.Context) (*clusterBalance, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+rebalancePath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, newRebalanceRequestError(http.MethodPost, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return nil, newRebalanceHTTPError(http.MethodPost, response)
	}

	var balance clusterBalance
	if err := json.NewDecoder(response.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode POST %s response: %w", rebalancePath, err)
	}

	return &balance, nil
}

func (c *rebalanceClient) getRebalance(ctx context.Context) (*clusterBalance, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+rebalancePath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, newRebalanceRequestError(http.MethodGet, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, newRebalanceHTTPError(http.MethodGet, response)
	}

	var balance clusterBalance
	if err := json.NewDecoder(response.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode GET %s response: %w", rebalancePath, err)
	}

	return &balance, nil
}

func rebalanceOutcome(completed *completedRebalance) error {
	summary := describeRebalancePartitions(completed.Partitions)

	if completed.Result != rebalanceResultCompleted {
		return fmt.Errorf("rebalance %d finished as %s: %s", completed.RebalanceID, completed.Result, summary)
	}

	for _, partition := range completed.Partitions {
		if partition.Result != "TRANSFERRED" && partition.Result != "ALREADY_LEADER" {
			return fmt.Errorf("rebalance %d finished as %s but %s was not rebalanced: %s", completed.RebalanceID, completed.Result, identifyRebalancePartition(partition), summary)
		}
	}

	internal.LogInfo("Rebalance %d completed successfully: %s", completed.RebalanceID, summary)
	return nil
}

func describeUnreachable(baseURL string, unreachable time.Duration) string {
	return fmt.Sprintf("%s unreachable for %s", baseURL, unreachable.Truncate(time.Millisecond))
}

func unobservedRebalanceError(rebalanceID int64, running *runningRebalance, completed *completedRebalance) error {
	switch {
	case completed != nil:
		return fmt.Errorf("rebalance %d was never observed completing: rebalance %d is now the last completed rebalance", rebalanceID, completed.RebalanceID)
	case running != nil:
		return fmt.Errorf("rebalance %d was never observed completing: rebalance %d is now running and none is reported completed", rebalanceID, running.RebalanceID)
	default:
		return fmt.Errorf("rebalance %d was never observed completing: the coordinator reports no completed rebalance", rebalanceID)
	}
}

func retryableTriggerError(err error) (string, bool) {
	var statusErr *rebalanceHTTPError
	if !errors.As(err, &statusErr) {
		return "", false
	}
	switch statusErr.StatusCode {
	case http.StatusServiceUnavailable:
		return "no rebalance coordinator is available", true
	case http.StatusConflict:
		return "a rebalance or a cluster configuration change is already in progress", true
	default:
		return "", false
	}
}

func ambiguousTriggerError(err error) bool {
	var transportErr *rebalanceTransportError
	if errors.As(err, &transportErr) {
		return true
	}

	var statusErr *rebalanceHTTPError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusBadGateway || statusErr.StatusCode == http.StatusGatewayTimeout
}

func retryablePollError(err error) bool {
	var statusErr *rebalanceHTTPError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func describeRebalanceProgress(running *runningRebalance) string {
	completed := 0
	for _, partition := range running.Partitions {
		if partition.Progress == rebalanceProgressCompleted {
			completed++
		}
	}
	return fmt.Sprintf("%d/%d partitions complete", completed, len(running.Partitions))
}

func identifyRebalancePartition(partition rebalancePartitionOperation) string {
	return fmt.Sprintf("partition %d of %s", partition.PartitionID, partition.PhysicalTenantID)
}

func describeRebalancePartitions(partitions []rebalancePartitionOperation) string {
	if len(partitions) == 0 {
		return "no partitions were part of the rebalance plan"
	}

	descriptions := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		result := partition.Result
		if result == "" {
			result = "none"
		}
		descriptions = append(descriptions, fmt.Sprintf("%s: progress %s, result %s", identifyRebalancePartition(partition), partition.Progress, result))
	}

	return strings.Join(descriptions, "; ")
}

func newRebalanceRequestError(method string, err error) error {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return &rebalanceAuthError{Method: method, Err: err}
	}
	return &rebalanceTransportError{Method: method, Err: err}
}

type rebalanceAuthError struct {
	Method string
	Err    error
}

func (e *rebalanceAuthError) Error() string {
	return fmt.Sprintf("%s %s: could not obtain a cluster-admin token: %s", e.Method, rebalancePath, e.Err)
}

func (e *rebalanceAuthError) Unwrap() error {
	return e.Err
}

type rebalanceTransportError struct {
	Method string
	Err    error
}

func (e *rebalanceTransportError) Error() string {
	return fmt.Sprintf("%s %s failed: %s", e.Method, rebalancePath, e.Err)
}

func (e *rebalanceTransportError) Unwrap() error {
	return e.Err
}

type rebalanceHTTPError struct {
	Method     string
	StatusCode int
	Body       string
}

func newRebalanceHTTPError(method string, response *http.Response) *rebalanceHTTPError {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes+1))
	if len(body) > maxErrorBodyBytes {
		body = append(body[:maxErrorBodyBytes], "…"...)
	}
	return &rebalanceHTTPError{Method: method, StatusCode: response.StatusCode, Body: string(body)}
}

func (e *rebalanceHTTPError) Error() string {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Sprintf("%s %s: HTTP 401, the token was not accepted: %s", e.Method, rebalancePath, e.Body)
	case http.StatusForbidden:
		return fmt.Sprintf("%s %s: HTTP 403, the credentials are not cluster-admin: %s", e.Method, rebalancePath, e.Body)
	default:
		return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, rebalancePath, e.StatusCode, e.Body)
	}
}

type clusterBalance struct {
	RunningRebalance       *runningRebalance   `json:"runningRebalance"`
	LastCompletedRebalance *completedRebalance `json:"lastCompletedRebalance"`
}

type runningRebalance struct {
	RebalanceID int64                         `json:"rebalanceId"`
	Partitions  []rebalancePartitionOperation `json:"partitions"`
}

type completedRebalance struct {
	RebalanceID int64                         `json:"rebalanceId"`
	Partitions  []rebalancePartitionOperation `json:"partitions"`
	Result      string                        `json:"result"`
}

type rebalancePartitionOperation struct {
	PartitionID      int32  `json:"partitionId"`
	PhysicalTenantID string `json:"physicalTenantId"`
	Progress         string `json:"progress"`
	Result           string `json:"result"`
}
