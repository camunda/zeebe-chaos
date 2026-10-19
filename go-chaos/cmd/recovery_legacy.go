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

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// restoreFromBackupLegacy restores a cluster configured via 'backup setup' by scaling it down
// and running the restore application as an init container before the brokers start.
func restoreFromBackupLegacy(flags *Flags) error {
	k8Client, err := createK8ClientWithFlags(flags)
	if err != nil {
		return err
	}

	namespace := k8Client.GetCurrentNamespace()
	err = k8Client.PauseReconciliation()
	if err != nil {
		return err
	}
	initialScale, err := k8Client.ScaleZeebeCluster(0)
	if err != nil {
		return err
	}

	sfs, err := k8Client.GetZeebeStatefulSet()
	if err != nil {
		return err
	}

	deleteContainer := core.Container{
		Name:            "delete-data",
		Image:           "busybox",
		ImagePullPolicy: core.PullAlways,
		Command:         []string{"sh", "-c", "rm -rf /usr/local/zeebe/data/* && ls -lah /usr/local/zeebe/data"},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      "data",
				ReadOnly:  false,
				MountPath: "/usr/local/zeebe/data",
			},
		},
	}
	restoreContainer := core.Container{
		Name:            "restore-from-backup",
		Image:           sfs.Spec.Template.Spec.Containers[0].Image,
		ImagePullPolicy: core.PullAlways,
		Env:             restoreEnvFromSfs(flags, sfs),
		EnvFrom:         []core.EnvFromSource{{SecretRef: &core.SecretEnvSource{LocalObjectReference: core.LocalObjectReference{Name: "zeebe-backup-store-s3"}}}},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      "data",
				ReadOnly:  false,
				MountPath: "/usr/local/zeebe/data",
			},
		},
	}
	sfs.Spec.Template.Spec.InitContainers = []core.Container{deleteContainer, restoreContainer}

	_, err = k8Client.Clientset.AppsV1().StatefulSets(namespace).Update(context.TODO(), sfs, meta.UpdateOptions{})
	if err != nil {
		return err
	}

	// Scale up after adding init containers.
	_, err = k8Client.ScaleZeebeCluster(initialScale)
	if err != nil {
		return err
	}

	err = k8Client.AwaitReadiness()
	if err != nil {
		return err
	}

	return k8Client.ResumeReconciliation()
}

func restoreEnvFromSfs(flags *Flags, sfs *apps.StatefulSet) []core.EnvVar {
	zeebeEnv := sfs.Spec.Template.Spec.Containers[0].Env
	restoreEnv := make([]core.EnvVar, 0)
	for _, env := range zeebeEnv {
		// Filter out java tool options.
		// If we don't, restore app will create a gc.log file in the data folder which prevents restoring because the data
		// folder is not empty.
		if env.Name != "JAVA_TOOL_OPTIONS" {
			restoreEnv = append(restoreEnv, env)
		}
	}
	restoreEnv = append(restoreEnv,
		core.EnvVar{
			Name:  "ZEEBE_RESTORE",
			Value: "true",
		},
		core.EnvVar{
			Name:  "ZEEBE_RESTORE_FROM_BACKUP_ID",
			Value: flags.backupId,
		})
	return restoreEnv
}
