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
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	template "text/template"

	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// k8Deployments holds our static k8 manifests, which are copied with the go:embed directive
//
//go:embed manifests/*
var k8Deployments embed.FS

func (c K8Client) getGatewayDeployment() (*v12.Deployment, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: c.getGatewayLabels(),
	}
	deploymentList, err := c.Clientset.AppsV1().Deployments(c.GetCurrentNamespace()).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}

	// here it is currently hard to distingush between not existing and embedded gateway;
	// since we don't use embedded gateway in our current chaos setup I would not support it right now here
	if deploymentList == nil || len(deploymentList.Items) <= 0 {
		return nil, errors.New(fmt.Sprintf("Expected to find standalone gateway deployment in namespace %s, but none found! The embedded gateway is not supported.", c.GetCurrentNamespace()))
	}
	return &deploymentList.Items[0], err
}

func (c K8Client) CreateWorkerDeploymentDefault() error {
	return c.CreateWorkerDeployment("zeebe", 1, &ClientCredentials{})
}

func (c K8Client) CreateWorkerDeployment(dockerImageTag string, pollingDelayMs int, credentials *ClientCredentials) error {
	templateFile, err := k8Deployments.ReadFile("manifests/worker.yaml")
	if err != nil {
		return err
	}
	// the template name must be the filename
	tmpl, err := template.New("worker.yaml").Parse(string(templateFile))
	if err != nil {
		return err
	}
	pollingDelayStr := strconv.FormatInt(int64(pollingDelayMs), 10) + "ms"
	workerBuilder := new(strings.Builder)
	replacements := TemplateReplacements{
		ImageTag:          dockerImageTag,
		PollingDelay:      pollingDelayStr,
		ClientCredentials: *credentials,
	}
	err = tmpl.ExecuteTemplate(workerBuilder, "worker.yaml", replacements)
	if err != nil {
		return err
	}
	fmt.Println("Template execute is ", workerBuilder.String())
	workerBytes := []byte(workerBuilder.String())

	LogVerbose("Deploy worker deployment to the current namespace: %s", c.GetCurrentNamespace())

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(workerBytes), 0)
	deployment := &v12.Deployment{}
	err = decoder.Decode(deployment)
	if err != nil {
		return err
	}

	container := &deployment.Spec.Template.Spec.Containers[0]
	if !c.SaaSEnv {
		// We are in self-managed environment
		// We have to update the service url such that our workers can connect
		// We expect that the used helm release name is == to the namespace name
		replaceJavaOptions(container, "zeebe-service:26500", fmt.Sprintf("%s-zeebe-gateway:26500", c.GetCurrentNamespace()))
	} else {
		// Required to schedule a pod on SaaS
		// https://github.com/camunda-cloud/team-sre/blob/main/docs/gke_taints_tolerations.md
		//
		// tolerations:
		//	 - effect: NoSchedule
		//	   key: components.gke.io/camunda-managed-components
		//	   operator: Equal
		//	   value: "true"
		deployment.Spec.Template.Spec.Tolerations = []v1.Toleration{
			{
				Effect:   v1.TaintEffectNoSchedule,
				Operator: v1.TolerationOpEqual,
				Key:      "components.gke.io/camunda-managed-components",
				Value:    "true",
			},
		}
	}

	_, err = c.Clientset.AppsV1().Deployments(c.GetCurrentNamespace()).Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		if err.Error() == "deployments.apps \"worker\" already exists" {
			LogInfo("Workers have already deployed, update deployment.")
			_, err = c.Clientset.AppsV1().Deployments(c.GetCurrentNamespace()).Update(context.TODO(), deployment, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}
	}
	return err
}

type TemplateReplacements struct {
	PollingDelay string
	ImageTag     string
	ClientCredentials
}

// Replaces a given string for a substitution in the JAVA_OPTIONS env var
func replaceJavaOptions(container *v1.Container, value string, subst string) {
	envVar := container.Env[0]
	envVar.Value = strings.Replace(envVar.Value, value, subst, 1)
	container.Env[0] = envVar
}
