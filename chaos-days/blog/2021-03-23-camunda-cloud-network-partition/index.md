---
layout: posts
title:  "Camunda Cloud network partition"
date:   2021-03-23
categories: 
  - chaos_experiment
  - camunda_cloud 
  - network
authors: zell
---

# Chaos Day Summary

This time [Deepthi](https://github.com/deepthidevaki) was joining me on my regular Chaos Day. :tada:

[In the second last chaos day](/2021-02-23-automate-deployments-dist/index.md) I created an automated chaos experiment, which verifies that the deployments are distributed after a network partition. Later it turned out that this doesn't work for camunda cloud, only for our helm setup. [The issue](https://github.com/zeebe-io/zeebe-cluster-testbench/issues/237) was that on our camunda cloud zeebe clusters we had no [NET_ADMIN](https://man7.org/linux/man-pages/man7/capabilities.7.html) capability to create ip routes (used for the network partitions). After discussing with our SRE's they proposed a good way to overcome this. On running chaos experiments, which are network related, we will patch our target cluster to add this capability. This means we don't need to add such functionality in camunda cloud and the related zeebe operate/controller. Big thanks to [Immi](https://github.com/hisImminence) and [David](https://github.com/Faffnir) for providing this fix.


**TL;DR;**

We were able to enhance the deployment distribution experiment and run it in the camunda cloud via testbench. We have enabled the experiment for Production M and L cluster plans. We had to adjust the rights for the testbench service account to make this work.

<!--truncate-->

## Chaos Experiment

We already had a [prepared chaos experiment](https://github.com/camunda/zeebe-chaos/blob/master/chaos-experiments/helm/deployment-distribution/experiment.json), but we needed to enhance that. Deepthi was so kind to create [PR](https://github.com/camunda/zeebe-chaos/pull/50) for that.

### Enhancement
The changes contain a new step before creating the network partition on the deployment distribution experiment, see [here](https://github.com/camunda/zeebe-chaos/blob/master/chaos-experiments/camunda-cloud/production-l/deployment-distribution/experiment.json#L25-L35).

```json
        {
            "type": "action",
            "name": "Enable net_admin capabilities",
            "provider": {
                "type": "process",
                "path": "apply_net_admin.sh"
            },
            "pauses": {
                "after": 180
            }
        }
```

The `apply_net_admin.sh` contains the following code:

```shell

#!/bin/bash
set -euo pipefail

scriptPath=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
source utils.sh

namespace=$(getNamespace)

CLUSTERID=${namespace%-zeebe}

kubectl patch zb "$CLUSTERID" --type merge --patch='{"spec":{"controller":{"reconcileDisabled":true}}}'
kubectl patch statefulset zeebe -n "$namespace" --patch "$(cat $scriptPath/net_admin_patch.yaml)"
```

It disables reconciliation for the target zeebe cluster and applies the following patch, which adds the `NET_ADMIN` capability:

```yaml
spec:
  template:
    spec:
      containers:
        - name: "zeebe"
          securityContext:
            capabilities:
              add:
                - "NET_ADMIN"
```

Big thanks to [Immi](https://github.com/hisImminence) and [David](https://github.com/Faffnir) for providing this fix.

After we applied the patch, we need to make sure that the all pods are restarted and have the requested change. This is the reason we wait after the action for some minutes. This was the easiest way for us to think of, but ideally we find a better way here to make sure the patch was applied and we can continue.

### Verification

We run this experiment with a Production L cluster (v1.0.0-alpha2) and it succeeded. This is quite nice, because this also contains already the rewritten [deployment distribution](https://github.com/camunda-cloud/zeebe/issues/6173) logic.

To verify whether the experiment really does something we checked the metrics and the logs. In the metrics we were not really able to tell that there was a network partition going on. There were no heart beats missing. The reason for that was that in the experiment the Leader for partition 3 (Node 1) and Leader for partition 1 (Node 0) have been disconnected. If we check the [partition distribution](https://github.com/camunda-cloud/zeebe/blob/develop/benchmarks/docs/scripts/partitionDistribution.sh) we can see that they have no partitions in common. 

```shell
$ ./partitionDistribution.sh 6 8 3
Distribution:
P\N|	N 0|	N 1|	N 2|	N 3|	N 4|	N 5
P 0|	L  |	F  |	F  |	-  |	-  |	-  
P 1|	-  |	L  |	F  |	F  |	-  |	-  
P 2|	-  |	-  |	L  |	F  |	F  |	-  
P 3|	-  |	-  |	-  |	L  |	F  |	F  
P 4|	F  |	-  |	-  |	-  |	L  |	F  
P 5|	F  |	F  |	-  |	-  |	-  |	L  
P 6|	L  |	F  |	F  |	-  |	-  |	-  
P 7|	-  |	L  |	F  |	F  |	-  |	-  

Partitions per Node:
N 0: 4
N 1: 5
N 2: 5
N 3: 4
N 4: 3
N 5: 3
```

Fortunately we saw in the logs that the Node 1, was retrying to send the deployments and at some point it succeeds.
```shell
2021-03-23 13:11:54.163 CET
Failed to receive deployment response for partitions [2, 4, 7, 8] (topic 'deployment-response-2251799813685304'). Retrying
2021-03-23 13:11:54.164 CET
Pushed deployment 2251799813685304 to all partitions.
2021-03-23 13:11:54.164 CET
Deployment CREATE command for deployment 2251799813685304 was written on partition 2
2021-03-23 13:11:54.164 CET
Deployment CREATE command for deployment 2251799813685304 was written on partition 4
2021-03-23 13:11:54.164 CET
Deployment CREATE command for deployment 2251799813685304 was written on partition 8
2021-03-23 13:11:54.165 CET
Deployment CREATE command for deployment 2251799813685304 was written on partition 7
2021-03-23 13:11:54.175 CET
Deployment 2251799813685304 distributed to all partitions successfully.
2021-03-23 13:11:54.763 CET
Failed to receive deployment response for partitions [2, 4, 7, 8] (topic 'deployment-response-2251799813685306'). Retrying
2021-03-23 13:11:54.764 CET
Pushed deployment 2251799813685306 to all partitions.
2021-03-23 13:11:54.764 CET
Deployment CREATE command for deployment 2251799813685306 was written on partition 4
2021-03-23 13:11:54.764 CET
Deployment CREATE command for deployment 2251799813685306 was written on partition 2
2021-03-23 13:11:54.765 CET
Deployment CREATE command for deployment 2251799813685306 was written on partition 8
2021-03-23 13:11:54.765 CET
Deployment CREATE command for deployment 2251799813685306 was written on partition 7
2021-03-23 13:11:54.773 CET
Deployment 2251799813685306 distributed to all partitions successfully.
```

### Testbench

After the experiment has succeeded and we had verified the execution we run this again on testbench.

We saw a non completing chaos worker after checking the chaostoolkit logs, we saw that it was still in the phase of disconnecting the nodes, which was the same issue as before [#237](https://github.com/zeebe-io/zeebe-cluster-testbench/issues/237).

```shell
[2021-03-23 12:40:24 INFO] [activity:161] Action: Enable net_admin capabilities
[2021-03-23 12:40:24 DEBUG] [process:53] Running: ['/home/chaos/zeebe-chaos/chaos-experiments/camunda-cloud/../scripts/apply_net_admin.sh']
[2021-03-23 12:40:24 WARNING] [process:66] This process returned a non-zero exit code. This may indicate some error and not what you expected. Please have a look at the logs.
[2021-03-23 12:40:24 DEBUG] [__init__:142] Data encoding detected as 'ascii' with a confidence of 1.0
[2021-03-23 12:40:24 DEBUG] [activity:180]   => succeeded with '{'status': 1, 'stdout': '', 'stderr': 'Error from server (Forbidden): zeebeclusters.cloud.camunda.io "cc108db7-768c-45cc-a6c5-098dc28f260c" is forbidden: User "system:serviceaccount:zeebe-chaos:zeebe-chaos-sa" cannot get resource "zeebeclusters" in API group "cloud.camunda.io" at the cluster scope\n'}'
[2021-03-23 12:40:24 INFO] [activity:198] Pausing after activity for 180s...
[2021-03-23 12:43:24 DEBUG] [__init__:355] No controls to apply on 'activity'
[2021-03-23 12:43:24 DEBUG] [__init__:355] No controls to apply on 'activity'
[2021-03-23 12:43:24 INFO] [activity:161] Probe: All pods should be ready
[2021-03-23 12:43:24 DEBUG] [process:53] Running: ['/home/chaos/zeebe-chaos/chaos-experiments/camunda-cloud/../scripts/verify-readiness.sh']
[2021-03-23 12:43:24 DEBUG] [__init__:142] Data encoding detected as 'ascii' with a confidence of 1.0
[2021-03-23 12:43:24 DEBUG] [__init__:142] Data encoding detected as 'ascii' with a confidence of 1.0
[2021-03-23 12:43:24 DEBUG] [activity:180]   => succeeded with '{'status': 0, 'stdout': 'pod/zeebe-0 condition met\npod/zeebe-1 condition met\npod/zeebe-2 condition met\npod/zeebe-3 condition met\npod/zeebe-4 condition met\npod/zeebe-5 condition met\n', 'stderr': "+ source utils.sh\n++ CHAOS_SETUP=cloud\n++ getNamespace\n+++ kubectl config view --minify --output 'jsonpath={..namespace}'\n++ namespace=cc108db7-768c-45cc-a6c5-098dc28f260c-zeebe\n++ echo cc108db7-768c-45cc-a6c5-098dc28f260c-zeebe\n+ namespace=cc108db7-768c-45cc-a6c5-098dc28f260c-zeebe\n+ '[' cloud == cloud ']'\n+ kubectl wait --for=condition=Ready pod -l app.kubernetes.io/app=zeebe --timeout=900s -n cc108db7-768c-45cc-a6c5-098dc28f260c-zeebe\n"}'
[2021-03-23 12:43:24 DEBUG] [__init__:355] No controls to apply on 'activity'
[2021-03-23 12:43:24 DEBUG] [__init__:355] No controls to apply on 'activity'
[2021-03-23 12:43:24 INFO] [activity:161] Action: Create network partition between leaders
[2021-03-23 12:43:24 DEBUG] [process:53] Running: ['/home/chaos/zeebe-chaos/chaos-experiments/camunda-cloud/../scripts/disconnect-leaders.sh']
```

In the logs we also saw that the patch didn't worked as expected with the used service account, so we need to fix here the permission and after that it should hopefully work. :crossed_fingers:

```shell
'Error from server (Forbidden): zeebeclusters.cloud.camunda.io "XXX" is forbidden: User "system:serviceaccount:zeebe-chaos:zeebe-chaos-sa" cannot get resource "zeebeclusters" in API group "cloud.camunda.io" at the cluster scope\n'
```

After checking with Immi, we were sure that we need to adjust the [serviceaccount roles](https://github.com/zeebe-io/zeebe-cluster-testbench/blob/develop/core/chaos-workers/deployment/service-account/zeebe-chaos-role.yaml#L9). After changing the apiGroups to `["*"]` the experiments are running in testbench and the patch can be applied. We can now see in the log the following:

```shell
[2021-03-23 14:21:24 DEBUG] [process:53] Running: ['/home/chaos/zeebe-chaos/chaos-experiments/camunda-cloud/../scripts/apply_net_admin.sh']
[2021-03-23 14:21:25 DEBUG] [__init__:142] Data encoding detected as 'ascii' with a confidence of 1.0
[2021-03-23 14:21:25 DEBUG] [activity:180]   => succeeded with '{'status': 0, 'stdout': 'zeebecluster.cloud.camunda.io/cc108db7-768c-45cc-a6c5-098dc28f260c patched\nstatefulset.apps/zeebe patched\n', 'stderr': ''}'
```

Thanks for participating [Deepthi](https://github.com/deepthidevaki).

#### Found Bugs

##### Re-connecting might fail

We realized during testing the experiment that the re-connecting might fail, because the pod can be rescheduled and then a ip route can't be delete since it no longer exist. [This is now fixed](https://github.com/camunda/zeebe-chaos/blob/master/chaos-experiments/scripts/connect-leaders.sh#L45-L48). We check for existence of the command `ip`, if this doesn't exist we know the pod was restarted and we ignore it.


*Before:*

```shell
function connect() {
 toChangedPod="$1"
 targetIp="$2"

 # update to have access to ip
 kubectl exec -n "$namespace" "$toChangedPod" -- apt update
 kubectl exec -n "$namespace" "$toChangedPod" -- apt install -y iproute2
 kubectl exec "$toChangedPod" -n "$namespace" -- ip route del unreachable "$targetIp"

}
```

*After:*

```shell

function connect() {
 toChangedPod="$1"
 targetIp="$2"

 if command -v ip
 then
     kubectl exec "$toChangedPod" -n "$namespace" -- ip route del unreachable "$targetIp"
 fi
}
```
