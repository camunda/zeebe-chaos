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
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

func Test_RestoreEnvFromSfsDropsJavaToolOptionsAndSetsBackupID(t *testing.T) {
	sfs := &apps.StatefulSet{}
	sfs.Spec.Template.Spec.Containers = []core.Container{{Env: []core.EnvVar{
		{Name: "ZEEBE_BROKER_CLUSTER_SIZE", Value: "3"},
		{Name: "JAVA_TOOL_OPTIONS", Value: "-Xlog:gc*:file=/usr/local/zeebe/data/gc.log"},
	}}}

	env := restoreEnvFromSfs(&Flags{backupId: "1748937221"}, sfs)

	assert.Equal(t, []core.EnvVar{
		{Name: "ZEEBE_BROKER_CLUSTER_SIZE", Value: "3"},
		{Name: "ZEEBE_RESTORE", Value: "true"},
		{Name: "ZEEBE_RESTORE_FROM_BACKUP_ID", Value: "1748937221"},
	}, env)
}
