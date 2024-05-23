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

package backend

import (
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/zeebe-io/zeebe-chaos/go-chaos/internal"
)

func ConnectToZeebeCluster(k8Client internal.K8Client) (zbc.Client, func(), error) {
	port, closeFn := k8Client.MustGatewayPortForward(0, 26500)

	zbClient, err := internal.CreateZeebeClient(port)
	if err != nil {
		return nil, closeFn, err
	}
	return zbClient, func() {
		zbClient.Close()
		closeFn()
	}, nil
}
