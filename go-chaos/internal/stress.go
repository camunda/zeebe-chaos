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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type StressType struct {
	IoStress  bool
	CpuStress bool
	MemStress bool
}

func PutStressOnPod(k8Client K8Client, timeoutSec string, podName string, stressType StressType) error {
	err := installStressOnPod(k8Client, podName)
	if err != nil {
		return err
	}

	stressCmd := []string{"stress", "--timeout", timeoutSec}

	if stressType.CpuStress {
		// Spawn N workers spinning on sqrt().
		stressCmd = append(stressCmd, "--cpu", "256")
	}

	if stressType.MemStress {
		// Spawn N workers spinning on malloc()/free(). Per default alloc 256MB per worker.
		stressCmd = append(stressCmd, "--vm", "4")
	}

	if stressType.IoStress {
		// Spawn N workers spinning on sync().
		stressCmd = append(stressCmd, "--io", "256")
	}

	err = k8Client.ExecuteCmdOnPod(stressCmd, podName)
	if err != nil {
		return err
	}

	return nil
}

func installStressOnPod(k8Client K8Client, podName string) error {
	err := k8Client.SetUserToRoot()
	if err != nil {
		return err
	}

	err = k8Client.AwaitReadiness()
	if err != nil {
		return err
	}

	// the -qq flag makes the tool less noisy, remove it to get more output
	err = k8Client.ExecuteCmdOnPod([]string{"apt", "-qq", "update"}, podName)
	if err != nil {
		return err
	}

	// the -qq flag makes the tool less noisy, remove it to get more output
	err = k8Client.ExecuteCmdOnPod([]string{"apt", "-qq", "install", "-y", "stress", "procps"}, podName)
	if err != nil {
		return err
	}
	return nil
}

func (c K8Client) SetUserToRoot() error {

	statefulSet, err := c.GetZeebeStatefulSet()
	if err != nil {
		return err
	}

	// We need to run the container with root to allow install tooling
	patch := []byte(`{
		"spec":{
			"template":{
				"spec":{
					"securityContext":{
						"runAsUser": 0,
						"runAsGroup": 0,
						"runAsNonRoot": false
					},
					"containers":[
						{
							"name": "zeebe",
							"securityContext":{
								"runAsUser": 0,
								"runAsGroup": 0,
								"runAsNonRoot": false
							}
						}]
				}
			}
		}
	}`)

	_, err = c.Clientset.AppsV1().StatefulSets(c.GetCurrentNamespace()).Patch(context.TODO(), statefulSet.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}
