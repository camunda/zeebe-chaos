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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseBackupID(t *testing.T) {
	backupID, err := parseBackupID("1748937221")

	require.NoError(t, err)
	assert.Equal(t, int64(1748937221), backupID)
}

func Test_ParseBackupIDRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "invalid"} {
		_, err := parseBackupID(value)
		assert.Error(t, err, value)
	}
}

func Test_TakeRuntimeBackupUsesRESTAPI(t *testing.T) {
	var requestBody map[string]int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v2/backups/runtime", request.URL.Path)
		require.NoError(t, json.NewDecoder(request.Body).Decode(&requestBody))
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"backupId":1748937221}`))
	}))
	defer server.Close()

	client := &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client()}
	err := client.takeBackup(PRIMARY_STORAGE, 1748937221)

	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"backupId": 1748937221}, requestBody)
}

func Test_WaitForBackupUsesRESTAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v2/backups/runtime/1748937221", request.URL.Path)
		_, _ = writer.Write([]byte(`{"state":"COMPLETED"}`))
	}))
	defer server.Close()

	client := &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client()}
	err := client.waitForBackup(PRIMARY_STORAGE, 1748937221, time.Second)

	require.NoError(t, err)
}

func Test_WaitForBackupKeepsPollingWhileNotFinished(t *testing.T) {
	for _, transient := range []string{"IN_PROGRESS", "INCOMPLETE", "DELETED"} {
		// given
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if calls.Add(1) == 1 {
				_, _ = writer.Write([]byte(`{"state":"` + transient + `"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"state":"COMPLETED"}`))
		}))
		client := &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client(), pollEvery: time.Millisecond}

		// when
		err := client.waitForBackup(PRIMARY_STORAGE, 1748937221, time.Second)

		// then
		server.Close()
		require.NoError(t, err, transient)
		assert.Equal(t, int32(2), calls.Load(), transient)
	}
}

func Test_WaitForBackupFailsOnFailedOrMissing(t *testing.T) {
	for _, state := range []string{"FAILED", "DOES_NOT_EXIST"} {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{"state":"` + state + `","failureReason":"boom"}`))
		}))
		client := &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client(), pollEvery: time.Millisecond}

		// when
		err := client.waitForBackup(PRIMARY_STORAGE, 1748937221, time.Second)

		// then
		server.Close()
		assert.ErrorContains(t, err, state)
		assert.ErrorContains(t, err, "boom")
	}
}

func Test_WaitForBackupFailsWhenBackupNotFound(t *testing.T) {
	// given
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client(), pollEvery: time.Millisecond}

	// when
	err := client.waitForBackup(PRIMARY_STORAGE, 1748937221, time.Second)

	// then
	var httpErr *httpStatusError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}
