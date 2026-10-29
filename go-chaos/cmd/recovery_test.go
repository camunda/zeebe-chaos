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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRESTClient(t *testing.T, handler http.HandlerFunc) *orchestrationRESTClient {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &orchestrationRESTClient{baseURL: server.URL + "/v2", httpClient: server.Client()}
}

func Test_ParseMode(t *testing.T) {
	for _, value := range []string{"PROCESSING", "RECOVERING"} {
		mode, err := parseMode(value)
		require.NoError(t, err)
		assert.Equal(t, Mode(value), mode)
	}
}

func Test_ParseModeRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "recovering", "invalid"} {
		_, err := parseMode(value)
		assert.Error(t, err, value)
	}
}

func Test_ChangeModeUsesRESTAPI(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPatch, request.Method)
		assert.Equal(t, "/v2/mode", request.URL.Path)
		assert.Equal(t, "RECOVERING", request.URL.Query().Get("mode"))
		_, _ = writer.Write([]byte(`{"changeId":"7"}`))
	})

	// when
	changeID, err := client.changeMode(RECOVERING)

	// then
	require.NoError(t, err)
	assert.Equal(t, "7", changeID)
}

func Test_ChangeModeReturnsHTTPStatusError(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"title":"Forbidden"}`))
	})

	// when
	_, err := client.changeMode(PROCESSING)

	// then
	var httpErr *httpStatusError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	assert.JSONEq(t, `{"title":"Forbidden"}`, string(httpErr.Body))
}

func Test_RestoreUsesRESTAPI(t *testing.T) {
	// given
	var requestBody map[string][]int64
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v2/restore", request.URL.Path)
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&requestBody))
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"changeId":"8"}`))
	})

	// when
	changeID, err := client.restore(1748937221)

	// then
	require.NoError(t, err)
	assert.Equal(t, "8", changeID)
	assert.Equal(t, map[string][]int64{"backupIds": {1748937221}}, requestBody)
}

func Test_RestoreRejectsUnexpectedStatus(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusConflict)
	})

	// when
	_, err := client.restore(1748937221)

	// then
	var httpErr *httpStatusError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func Test_WaitForRestoreCompletes(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v2/restore", request.URL.Path)
		_, _ = writer.Write([]byte(`{"status":"COMPLETED","changeId":"8"}`))
	})

	// when
	err := client.waitForRestore("8", time.Second)

	// then
	require.NoError(t, err)
}

func Test_WaitForRestoreTreatsNotFoundAsCompleted(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	})

	// when
	err := client.waitForRestore("8", time.Second)

	// then
	require.NoError(t, err)
}

func Test_WaitForRestoreFailsOnTerminalStatus(t *testing.T) {
	for _, status := range []string{"FAILED", "CANCELLED"} {
		// given
		client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{"status":"` + status + `","changeId":"8"}`))
		})

		// when
		err := client.waitForRestore("8", time.Second)

		// then
		assert.ErrorContains(t, err, status)
	}
}

func Test_WaitForRestoreFailsOnOtherChange(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"status":"IN_PROGRESS","changeId":"9"}`))
	})

	// when
	err := client.waitForRestore("8", time.Second)

	// then
	assert.ErrorContains(t, err, "belongs to change 9 instead of 8")
}

func Test_WaitForRestoreFailsOnServerError(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	})

	// when
	err := client.waitForRestore("8", time.Second)

	// then
	var httpErr *httpStatusError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}

func Test_WaitForChangeCompletesWhenLastCompletedChangeMatches(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v2/topology", request.URL.Path)
		_, _ = writer.Write([]byte(`{"lastCompletedChangeId":"7"}`))
	})

	// when
	err := client.waitForChange("7", time.Second)

	// then
	require.NoError(t, err)
}

func Test_WaitForChangeTimesOut(t *testing.T) {
	// given
	client := newTestRESTClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"lastCompletedChangeId":"6"}`))
	})

	client.pollEvery = time.Millisecond

	// when
	err := client.waitForChange("7", 20*time.Millisecond)

	// then
	assert.ErrorContains(t, err, "cluster change 7 did not complete")
}
