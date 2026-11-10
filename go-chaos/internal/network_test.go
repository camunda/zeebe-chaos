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

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NetworkDebugImageDefaultsToBusybox(t *testing.T) {
	// given
	t.Setenv(networkDebugImageEnv, "")
	k8Client := K8Client{}

	// when
	image := networkDebugImage(k8Client)

	// then
	assert.Equal(t, defaultNetworkDebugImage, image)
}

func Test_NetworkDebugImageUsesMirroredBusyboxInSaaS(t *testing.T) {
	// given
	t.Setenv(networkDebugImageEnv, "")
	k8Client := K8Client{SaaSEnv: true}

	// when
	image := networkDebugImage(k8Client)

	// then
	assert.Equal(t, saasNetworkDebugImage, image)
}

func Test_NetworkDebugImageCanBeOverridden(t *testing.T) {
	// given
	t.Setenv(networkDebugImageEnv, "registry.example.com/netdebug:1")
	k8Client := K8Client{SaaSEnv: true}

	// when
	image := networkDebugImage(k8Client)

	// then
	assert.Equal(t, "registry.example.com/netdebug:1", image)
}
