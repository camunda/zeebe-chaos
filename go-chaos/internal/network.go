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

package internal

import (
	"os"
	"strings"
)

const (
	defaultNetworkDebugImage = "busybox:1.38.0"
	saasNetworkDebugImage    = "europe-docker.pkg.dev/camunda-saas-registry/vendor/busybox:1.38.0"
	networkDebugImageEnv     = "ZBCHAOS_NETWORK_DEBUG_IMAGE"
	deleteUnreachableRoutes  = `set -o pipefail; ip route show | { grep '^unreachable' || true; } | while read -r _ ip _; do ip route del unreachable "$ip" || exit 1; done`
)

func networkDebugImage(k8Client K8Client) string {
	if image := os.Getenv(networkDebugImageEnv); image != "" {
		return image
	}
	if k8Client.SaaSEnv {
		return saasNetworkDebugImage
	}
	return defaultNetworkDebugImage
}

func MakeIpUnreachableForPod(k8Client K8Client, podIp string, podName string) error {
	cmd := []string{"ip", "route", "replace", "unreachable", podIp}
	return runIpCommand(k8Client, podName, strings.Join(cmd, " "))
}

func RemoveUnreachableRoutes(k8Client K8Client, podName string) error {
	return runIpCommand(k8Client, podName, deleteUnreachableRoutes)
}

func runIpCommand(k8Client K8Client, podName string, cmd string) error {
	return k8Client.ExecuteCommandViaDebugContainerAndAwait(podName, "", networkDebugImage(k8Client), []string{"sh", "-c", cmd})
}
