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

package cmd

import (
	"context"
	"os"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// setupBackup configures the legacy broker backup settings for clusters that are
// not provisioned with backup storage as part of their deployment.
func setupBackup(kubeConfigPath string, namespace string) error {
	k8Client, err := internal.CreateK8Client(kubeConfigPath, namespace)
	if err != nil {
		return err
	}
	namespace = k8Client.GetCurrentNamespace()
	if err := k8Client.PauseReconciliation(); err != nil {
		return err
	}
	if _, err := createBackupSecret(k8Client, namespace); err != nil {
		return err
	}
	if err := setupStatefulSetForBackups(k8Client, namespace); err != nil {
		return err
	}
	return setupGatewayForBackups(k8Client, namespace)
}

func setupStatefulSetForBackups(k8Client internal.K8Client, namespace string) error {
	sfs, err := k8Client.GetZeebeStatefulSet()
	if err != nil {
		return err
	}
	sfs.Spec.Template.Spec.Containers[0].EnvFrom = append(sfs.Spec.Template.Spec.Containers[0].EnvFrom, core.EnvFromSource{SecretRef: &core.SecretEnvSource{LocalObjectReference: core.LocalObjectReference{Name: "zeebe-backup-store-s3"}}})
	sfs.Spec.Template.Spec.Containers[0].Env = append(sfs.Spec.Template.Spec.Containers[0].Env,
		core.EnvVar{Name: "ZEEBE_BROKER_DATA_BACKUP_STORE", Value: "S3"},
		core.EnvVar{Name: "ZEEBE_BROKER_EXPERIMENTAL_FEATURES_ENABLEBACKUP", Value: "true"},
		core.EnvVar{Name: "MANAGEMENT_ENDPOINTS_WEB_EXPOSURE_INCLUDE", Value: "*"},
		core.EnvVar{Name: "MANAGEMENT_ENDPOINTS_BACKUPS_ENABLED", Value: "true"},
	)
	_, err = k8Client.Clientset.AppsV1().StatefulSets(namespace).Update(context.TODO(), sfs, meta.UpdateOptions{})
	return err
}

func setupGatewayForBackups(k8Client internal.K8Client, namespace string) error {
	selector := meta.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/component": "standalone-gateway"}}
	gateways, err := k8Client.Clientset.AppsV1().Deployments(namespace).List(context.TODO(), meta.ListOptions{LabelSelector: labels.Set(selector.MatchLabels).String()})
	if err != nil {
		return err
	}
	if len(gateways.Items) == 0 {
		selector = meta.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/component": "zeebe-gateway"}}
		gateways, err = k8Client.Clientset.AppsV1().Deployments(namespace).List(context.TODO(), meta.ListOptions{LabelSelector: labels.Set(selector.MatchLabels).String()})
		if err != nil {
			return err
		}
	}
	if len(gateways.Items) == 0 {
		return nil
	}
	gateway := gateways.Items[0]
	gateway.Spec.Template.Spec.Containers[0].Env = append(gateway.Spec.Template.Spec.Containers[0].Env,
		core.EnvVar{Name: "ZEEBE_BROKER_EXPERIMENTAL_FEATURES_ENABLEBACKUP", Value: "true"},
		core.EnvVar{Name: "MANAGEMENT_ENDPOINTS_WEB_EXPOSURE_INCLUDE", Value: "*"},
		core.EnvVar{Name: "MANAGEMENT_ENDPOINTS_BACKUPS_ENABLED", Value: "true"},
	)
	_, err = k8Client.Clientset.AppsV1().Deployments(namespace).Update(context.TODO(), &gateway, meta.UpdateOptions{})
	return err
}

func createBackupSecret(k8Client internal.K8Client, namespace string) (*core.Secret, error) {
	return k8Client.Clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &core.Secret{
		Type:       core.SecretTypeOpaque,
		ObjectMeta: meta.ObjectMeta{Name: "zeebe-backup-store-s3"},
		Data: map[string][]byte{
			"ZEEBE_BROKER_DATA_BACKUP_S3_BUCKETNAME": []byte(os.Getenv("ZEEBE_BROKER_DATA_BACKUP_S3_BUCKETNAME")),
			"ZEEBE_BROKER_DATA_BACKUP_S3_REGION":     []byte(os.Getenv("ZEEBE_BROKER_DATA_BACKUP_S3_REGION")),
			"ZEEBE_BROKER_DATA_BACKUP_S3_ACCESSKEY":  []byte(os.Getenv("ZEEBE_BROKER_DATA_BACKUP_S3_ACCESSKEY")),
			"ZEEBE_BROKER_DATA_BACKUP_S3_SECRETKEY":  []byte(os.Getenv("ZEEBE_BROKER_DATA_BACKUP_S3_SECRETKEY")),
		},
	}, meta.CreateOptions{})
}
