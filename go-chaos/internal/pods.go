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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"k8s.io/utils/ptr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

func (c K8Client) GetBrokerPods() (*v1.PodList, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: c.getBrokerLabels(),
	}

	list, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}

	return list, err
}

func (c K8Client) GetBrokerPodNames() ([]string, error) {
	list, err := c.GetBrokerPods()
	if err != nil {
		return nil, err
	}

	return c.extractPodNames(list)
}

func (c K8Client) extractPodNames(list *v1.PodList) ([]string, error) {
	pods := list.Items
	names := make([]string, len(pods))

	for idx, item := range pods {
		names[idx] = item.Name
	}

	return names, nil
}

func (c K8Client) GetGatewayPods() (*v1.PodList, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: c.getGatewayLabels(),
		// we check for running gateways, since terminated gateways can be lying around
		FieldSelector: "status.phase=Running",
	}

	list, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}

	return list, err
}

func (c K8Client) GetCorePods() (*v1.PodList, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: c.GetCoreLabels(),
		// we check for running gateways, since terminated gateways can be lying around
		FieldSelector: "status.phase=Running",
	}

	list, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}

	return list, err
}

func (c K8Client) GetGatewayPodNames() ([]string, error) {
	list, err := c.GetGatewayPods()
	if err != nil {
		return nil, err
	}

	if list != nil && len(list.Items) <= 0 {
		list, err = c.GetBrokerPods()
		if err != nil {
			return nil, err
		}
	}

	if list != nil && len(list.Items) <= 0 {
		list, err = c.GetCorePods()
		if err != nil {
			return nil, err
		}
	}

	return c.extractPodNames(list)
}

func (c K8Client) GetWorkerPods() (*v1.PodList, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: c.getWorkerLabels(),
		// we check for running workers, since terminated workers can be lying around
		FieldSelector: "status.phase=Running",
	}

	list, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}

	return list, err
}

func (c K8Client) TerminatePod(podName string) error {
	gracePeriodSec := int64(0)
	options := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriodSec}
	return c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Delete(context.TODO(), podName, options)
}

func (c K8Client) RestartPodWithGracePeriod(podName string, gracePeriodSec *int64) error {
	options := metav1.DeleteOptions{GracePeriodSeconds: gracePeriodSec}
	return c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Delete(context.TODO(), podName, options)
}

func (c K8Client) AwaitReadiness() error {
	return c.AwaitReadinessWithTimeout(5*time.Minute, 1*time.Second)
}

func (c K8Client) AwaitReadinessWithTimeout(timeout time.Duration, tickTime time.Duration) error {
	timedOut := time.After(timeout)
	ticker := time.Tick(tickTime)

	// Keep checking until we're timed out
	for {
		select {
		case <-timedOut:
			return errors.New(fmt.Sprintf("Awaited readiness of pods in namespace %v, but timed out after %v", c.GetCurrentNamespace(), timeout))
		case <-ticker:
			brokersAreRunning, err := c.checkIfBrokersAreRunning()
			if err != nil {
				LogVerbose("Failed to check broker status. Will retry. %v", err)
			}
			gatewaysAreRunning, err := c.checkIfGatewaysAreRunning()
			if err != nil {
				LogVerbose("Failed to check gateway status. Will retry. %v", err)
			}

			if brokersAreRunning && gatewaysAreRunning {
				return nil
			}
		}
	}
}

func (c K8Client) checkIfBrokersAreRunning() (bool, error) {
	pods, err := c.GetBrokerPods()
	if err != nil {
		return false, err
	}

	if len(pods.Items) <= 0 {
		return false, errors.New(fmt.Sprintf("Expected to find brokers in namespace %s, but none found.", c.GetCurrentNamespace()))
	}

	allRunning := true
	for _, pod := range pods.Items {
		if pod.Status.Phase != v1.PodRunning || !pod.Status.ContainerStatuses[0].Ready { // assuming there is only one container
			LogVerbose("Pod %s is in phase %s, and not ready. Wait for some seconds.", pod.Name, pod.Status.Phase)
			allRunning = false
			break
		}
	}

	return allRunning, nil
}

func (c K8Client) checkIfGatewaysAreRunning() (bool, error) {
	deployment, err := c.getGatewayDeployment()
	if err != nil {
		return false, err
	}

	if deployment.Status.UnavailableReplicas > 0 {
		LogVerbose("Gateway deployment not fully available. [Available replicas: %d/%d]", deployment.Status.AvailableReplicas, deployment.Status.Replicas)
		return false, nil
	}
	return true, nil
}

func (c K8Client) AwaitPodReadiness(podName string, timeout time.Duration) error {
	timedOut := time.After(timeout)
	ticker := time.Tick(1 * time.Second)

	// Keep checking until we're timed out
	for {
		select {
		case <-timedOut:
			return errors.New(fmt.Sprintf("Pod %s is not ready with in given timeout %v", podName, timeout))
		case <-ticker:
			// check status of pod on every tick (1 second)
			pod, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Get(context.TODO(), podName, metav1.GetOptions{})
			if err != nil {
				LogVerbose("Failed to get pod %s. Will retry", pod.Name)
			} else if pod.Status.Phase == v1.PodRunning && pod.Status.ContainerStatuses[0].Ready { // assuming there is only one container
				return nil
			} else {
				LogVerbose("Pod %s is in phase %s, but not ready. Wait for some seconds", pod.Name, pod.Status.Phase)
			}
		}
	}
}

func (c K8Client) RestartPod(podName string) error {
	return c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Delete(context.TODO(), podName, metav1.DeleteOptions{})
}

// MustGatewayPortForward creates a port forwarding to a zeebe gateway with the given port.
// Panics when port forwarding fails.
// localPort can be 0 to let the OS choose a random, free port.
// Returns the exposed local port and a function to close the port forwarding.
//
// https://github.com/gruntwork-io/terratest/blob/master/modules/k8s/tunnel.go#L187-L196
// https://github.com/kubernetes/client-go/issues/51#issuecomment-436200428
func (c K8Client) MustGatewayPortForward(localPort int, remotePort int) (int, func()) {
	names, err := c.GetGatewayPodNames()
	if err != nil {
		panic(err)
	}

	if len(names) <= 0 {
		panic(errors.New(fmt.Sprintf("Expected to find Zeebe gateway in namespace %s, but none found.", c.GetCurrentNamespace())))
	}

	portForwardCreateURL := c.createPortForwardUrl(names)
	portForwarder, err := c.createPortForwarder(localPort, remotePort, portForwardCreateURL)
	if err != nil {
		panic(err)
	}

	// Open the tunnel in a goroutine so that it is available in the background. Report errors to the main goroutine via
	// a new channel.
	errChan := make(chan error)
	go func() {
		errChan <- portForwarder.ForwardPorts()
	}()

	// Wait for an error or the tunnel to be ready
	select {
	case err = <-errChan:
		LogVerbose("\nError starting port forwarding tunnel: %s", err)
		panic(err)
	case <-portForwarder.Ready:
		ports, err := portForwarder.GetPorts()
		if err != nil {
			panic(err)
		}
		exposedLocalPort := ports[0].Local
		LogVerbose("Successfully created port forwarding tunnel from %d (local) to %d (remote)", exposedLocalPort, remotePort)
		return int(exposedLocalPort), func() {
			portForwarder.Close()
		}
	}
}

// Create the k8 port forwarder, with the given port and k8 client
func (c K8Client) createPortForwarder(localPort int, remotePort int, portForwardCreateURL *url.URL) (*portforward.PortForwarder, error) {
	// Construct the spdy client required by the client-go portforward library
	config, err := c.ClientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		LogVerbose("Error creating http client: %s", err)
		return nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", portForwardCreateURL)

	// Construct a new PortForwarder struct that manages the instructed port forward tunnel
	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	portforwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		LogVerbose("Error creating port forwarding tunnel: %s", err)
		return nil, err
	}
	return portforwarder, nil
}

// createPortForwardUrl constructs the Url to which is used to create the port forwarding
func (c K8Client) createPortForwardUrl(names []string) *url.URL {
	gatewayName := names[rand.Intn(len(names))]
	LogVerbose("Port forward to %s", gatewayName)
	restClient := c.Clientset.CoreV1().RESTClient()
	portForwardCreateURL := restClient.Post().
		Resource("pods").
		Namespace(c.GetCurrentNamespace()).
		Name(gatewayName).
		SubResource("portforward").
		URL()
	return portForwardCreateURL
}

func (c K8Client) ExecuteCommandViaDebugContainer(podName string, containerName string, debugImage string, cmd []string) error {
	pod, err := c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	name := "debug-" + rand.String(6)
	debugContainer := v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:    name,
			Image:   debugImage,
			Command: cmd,
			SecurityContext: &v1.SecurityContext{
				RunAsNonRoot: ptr.To(false),
				RunAsUser:    ptr.To(int64(0)),
				RunAsGroup:   ptr.To(int64(0)),
				Privileged:   ptr.To(true),
			},
		},
		TargetContainerName: containerName,
	}
	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, debugContainer)
	_, err = c.Clientset.CoreV1().Pods(c.GetCurrentNamespace()).UpdateEphemeralContainers(context.TODO(), pod.Name, pod, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	LogVerbose("Debug container %s is running command %v", name, cmd)
	return nil
}

func (c K8Client) ExecuteCmdOnPod(cmd []string, pod string) error {
	if Verbosity {
		return c.ExecuteCmdOnPodWriteIntoOutput(cmd, pod, os.Stdout)
	}
	return c.ExecuteCmdOnPodWriteIntoOutput(cmd, pod, io.Discard)
}

func (c K8Client) ExecuteCmdOnPodWriteIntoOutput(cmd []string, pod string, output io.Writer) error {
	LogVerbose("Execute %+q on pod %s", cmd, pod)

	req := c.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name(pod).
		Namespace(c.GetCurrentNamespace()).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	config, err := c.ClientConfig.ClientConfig()
	if err != nil {
		return err
	}

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: output,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	})
	if err != nil {
		return err
	}

	return nil
}
