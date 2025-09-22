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

type RoutingState struct {
	MessageCorrelation MessageCorrelation `json:"messageCorrelation"`
	RequestHandling    RequestHandling    `json:"requestHandling"`
	Version            int                `json:"version"`
}

// MessageCorrelation (discriminator: strategy)
type MessageCorrelation struct {
	Strategy       string `json:"strategy"`
	PartitionCount int    `json:"partitionCount"`
}

// RequestHandling (discriminator: strategy)
type RequestHandling struct {
	Strategy string `json:"strategy"`
	// Only one of the following will be set, depending on the strategy
	*RequestHandlingAllPartitions
	*RequestHandlingActivePartitions
}

type RequestHandlingAllPartitions struct {
	PartitionCount int `json:"partitionCount"`
}

type RequestHandlingActivePartitions struct {
	BasePartitionCount         int   `json:"basePartitionCount"`
	AdditionalActivePartitions []int `json:"additionalActivePartitions"`
	InactivePartitions         []int `json:"inactivePartitions"`
}
