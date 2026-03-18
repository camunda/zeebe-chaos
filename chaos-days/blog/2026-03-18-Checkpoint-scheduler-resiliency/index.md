---
layout: posts
title: "Checkpoint scheduler resiliency"
date: 2026-03-17
categories:
  - chaos_experiment
  - broker
  - backups
  - rdbms
authors: panos
---

# Chaos Experiment Summary

With the introduction of the RDBMS support in Camunda 8.9, we needed a reliable and consistent mechanism to back up Zeebe’s primary storage. To address this, we introduced scheduled backups, which allow operators to configure a backup interval with the same processing guarantees the engine already provides, since backups are also supported by the logstream itself.

Since our goal is to achieve the highest possible RPO (Recovery Point Objective) without sacrificing processing throughput, we’ve made several improvements across the supported backup stores. This experiment measures where we currently stand in practice.

Within the bounds of this experiment, we compare backup-store performance across the three major cloud providers: Google Cloud Storage (GCS), AWS S3, and Azure Blob Storage.

## Scheduler introduction

To guarantee the continuity and correctness of primary storage backups it was required for such a cluster level service to be present. For this reason, the _Checkpoint Scheduler_ was introduced, which purpose is to be the time keeper of checkpoint creation in the cluster, fanning out the creation of checkpoints to all partitions.

The scheduler is always assigned to the broker with the lowest `id`, which is part of the replication cluster. Under normal operation that would mean that `camunda-0` pod is the one with the service registered. The scheduler's interval, while preconfigured, is dynamic and will adapt to network issues in a best effort to maintain the desired interval.

Right now, it supports two types of checkpoints:
- `MARKER`: Used as reference points for point-in-time restore operation
- `SCHEDULED_BACKUP`: Trigger a primary storage backup


Alongside the scheduler, the retention service will also be registered in the same node, if a backup retention schedule is configured. This service is responsible for deleting backups outside the configured window to reduce storage costs. Furthermore, too old backups are not that useful in a disaster recovery scenario.

The checkpoint scheduler and the retention mechanism can be configured via the available [options](https://docs.camunda.io/docs/next/self-managed/components/orchestration-cluster/zeebe/configuration/broker-config/#camundadataprimary-storagebackup).


## Chaos experiment

### Expected outcomes

The expectation of this experiment is to prove that the checkpoint and backup schedulers are resilient to network and topology changes that can occur during a cluster's lifespan.

### Setup

In this experiment we'll be using a standard Camunda 8.9 Kubernetes installation with the checkpoint scheduler and retnetion enabled.


#### Enabling the scheduler

To enable the checkpoing & backup schedulers, we supply the following configuration params for the `camunda` stateful set:

```
CAMUNDA_DATA_PRIMARYSTORAGE_BACKUP_CONTINUOUS=true
CAMUNDA_DATA_PRIMARYSTORAGE_BACKUP_CHECKPOINTINTERVAL=PT1M
CAMUNDA_DATA_PRIMARYSTORAGE_BACKUP_SCHEDULE=PT3M
CAMUNDA_DATA_PRIMARYSTORAGE_BACKUP_RETENTION_WINDOW=PT30M
CAMUNDA_DATA_PRIMARYSTORAGE_BACKUP_RETENTION_CLEANUPSCHEDULE=PT10M
```

Meaning that, we take a full Zeebe backup every 3 minutes and inject marker checkpoints into the log stream every 1 minute. We also want to maintain a rolling window of 30 minutes worth of backups and we check for backups to be deleted every 10 minutes.

Upon applying this configuration we see the following logs being produced, verifying the presence of the scheduler:

![logs-image](startup-logs.png)

You may notice that these logs are present on all brokers, this is intentional as the service being active is based on the intra-cluster discovery protocol which is updated in realtime. This means that when, for example, `camunda-0` is considered _gone_ `camunda-1` is ready to take over the service and pick up where it should.

Also, the first backup is already taken. As there was no previous present an immediate one is captured. With that in mind, if a cluster sustains a prolonged unhealthy state, more than the configured backup interval, the next backup will be immediately taken once the cluster reaches a healthy state.

And also [Grafana's dashboard](https://github.com/camunda/camunda/blob/7a24435ba60e341db9095d381ce510fa6794db5f/monitor/grafana/zeebe.json) related to the scheduler will start displaying proper data:

![startup-metrics](startup-metrics.png)

After the first 3 minutes pass, according to the backup interval, we should have a backup available

![first-backup-metrics](first-backup-metrics.png)

and the corresponding logs related to it with the matching timestamps

![first-backup-logs](first-backup-logs.png)

Inspecting the logs further, we see that the initiating node of that backup was the pod `camunda-0`, which is expected.

![first-backup-pod](first-backup-pod.png)


The checkpoints can also be verified by querying for Zeebe's internal state via the actuator. Notice in the following response that the `checkpointId` for the backups and for the active ranges match what's seen in the metrics as well. _Displaying a single partition to reduce the size_


```bash
curl localhost:9600/actuator/backupRuntime/state

{
  "checkpointStates": [
    {
      "checkpointId": 1773840131794,
      "checkpointType": "MARKER",
      "partitionId": 2,
      "checkpointPosition": 1695,
      "checkpointTimestamp": "2026-03-18T13:22:11.781+0000"
    }
  ],
  "backupStates": [
    {
      "checkpointId": 1773840011037,
      "checkpointType": "SCHEDULED_BACKUP",
      "partitionId": 2,
      "checkpointPosition": 1465,
      "firstLogPosition": 1,
      "checkpointTimestamp": "2026-03-18T13:20:13.001+0000"
    }
  ],
  "ranges": [
    {
      "partitionId": 2,
      "start": {
        "checkpointId": 1773839828707,
        "checkpointType": "SCHEDULED_BACKUP",
        "checkpointPosition": 1095,
        "firstLogPosition": 1,
        "checkpointTimestamp": "2026-03-18T13:17:10.852+0000"
      },
      "end": {
        "checkpointId": 1773840011037,
        "checkpointType": "SCHEDULED_BACKUP",
        "checkpointPosition": 1465,
        "firstLogPosition": 1,
        "checkpointTimestamp": "2026-03-18T13:20:13.001+0000"
      }
    }
  ]
}
```

#### Disconnecting a node

To simulate disconnecting a node from the Camunda Orchestration cluster, we can use a simple Kubernetes network policy:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: isolate-pod
  namespace: c8-pg-scheduler-ft
spec:
  podSelector:
    matchLabels:
      isolated: "true"
  policyTypes:
  - Ingress
  - Egress
```

This policy matches the given label `isolated` present on deployed pods, disconnecting a node just requires applying this label. For example `kubectl label pod camunda-0 isolated=true --overwrite` will result in the pod `camunda-0` being removed from the Orchestration cluster.

### Experiment

#### Disconnecting broker-0

Executing `kubectl label pod camunda-0 isolated=true --overwrite` causes broker 0 to be disconnected from the cluster. For the scheduling service, this effectively means that it should be registered on `camunda-1` broker. Sure enough, the logs confirm this

![handover-logs](handover-logs.png)

![handover-logs-pod](handover-logs-pod.png)

You can clearly see in the logs that the next expected backup is scheduled in less than the configured, 3 minutes. This is intentional, as mentioned before, the scheduler's interval is dynamically adapting to maintain the backup schedule.

### Disconnecting broker-1

Applying the `isolated` label on broker-1 cause the cluster to reach in an unhealthy state, since it has now suffered 2 node losses. The remaining node, `camunda-2` cannot form a cluster on it's unable to proceed in the startup sequence to start initiating backups.

### Rejoining brokers

Removing the label from the disconnected pods,

```bash
kubectl label pod camunda-0 isolated-
kubectl label pod camunda-1 isolated-
```

causes yet another handover, this time back to `camunda-0` node and the schedule's execution continues as expected.

![next-backup](next-backup.png)

![next-backup-execution](next-backup-executed-handover.png)


## Bonus: Retention

Leaving the cluster running long enough, causes the retention to kick in so it's metrics and results are also available in the dashboard. We can see that we have 3 backups deleted for each partition.

![retention](retention.png)

We also see the earliest backup still present that was not picked up by retention, `1773840375165`, looking in the logs we can also confirm it's capture time, `15:26`.

![retention-earliest](retention-earliest.png)

Since our backups started at `15:17`, we expect to have backups available at the following timestamps:

- 15:17
- 15:20
- 15:23
- 15:26
- 15:29...

Since retention was executed on `15:55` the reported amount of backups pruned is on-par with what's expected. Backups taken at `15:17`,`15:20` and `15:23` all satisfy being 30 minutes before the retention mechanism execution.


## Conclusion

In this experiment we've proved that the checkpoint and backup scheduler can maintain the configured backup interval while surviving broker disconnects and topology changes.
