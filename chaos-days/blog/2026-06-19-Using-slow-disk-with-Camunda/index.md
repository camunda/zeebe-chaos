---
layout: posts
title:  "Using slow disk with Camunda"
date:   2026-06-19
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Chaos Day Summary


In todays, Chaos day we wanted to experiment with slow disk related issues as we have recently run into some incidents realted to that. We want to understand and document how Camunda behaves in such scenarios. 

We have two main experiments planned: one about using slow disks for the primary storage and one for the secondary storage (in this case Elasticsearch).



**TL;DR;** 

<!--truncate-->




## Chaos experiment: Slow disk on primary storage

We have certain recommendations for disk types in our [documentation](https://docs.camunda.io/docs/next/self-managed/reference-architecture/kubernetes/#minimum-cluster-requirements), especially using SSDs, high-throughput low-latency disks, as Camunda is an IO tense application. What we miss as part of our documentation is to actual show case why this is important. 

In this experiment we want to understand how the Camunda cluster behaves when the primary storage disks are slow. How does this affect the performance and availability of the cluster.

We plan to setup an realistic load test, with a Camunda cluster using a standard hard disk for the primary storage.


### Expected 

We only support SSDs, as Camunda is in general IO intensive, and we expect that using slower disks will cause performance degradation. We expect that the system may become unresponsive, and we may see timeouts and increased latency in processing requests.

#### Actual

As we runnig our tests and setup in GCP, we make use of what GCP us offers as standard hard disks.

[GCP persistent disk types:](https://docs.cloud.google.com/compute/docs/disks/persistent-disks#disk-types) The standard storage class maps to the pd-standard GCP disk type:

```sh
$ k get storageclasses.storage.k8s.io standard -o yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard
allowVolumeExpansion: true
provisioner: kubernetes.io/gce-pd
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  type: pd-standard
```


We were setting up a [realistic load test](https://docs.camunda.io/docs/components/best-practices/architecture/sizing-benchmarks/#reference-benchmark-scenario) using the `c8-chaos-25-pd-standard` namespace, and we are using the `8.9.9` image as the last stable release. We have configured the cluster to use standard hard disks for the primary storage.


```diff
orchestration:
  ## @param core.pvcStorageClassName can be used to set the storage class name which should be used by the persistent volume claim.
  # It is recommended to use a storage class, which is backed with a SSD. Set to "-" to disable use of default storage class.
-  pvcStorageClassName: benchmark-ssd-zonal-v1
+  pvcStorageClassName: standard
```

We will compare this test with some of our release tests that use SSDs, to see the difference in performance and availability.

During the begin of the test the throughput looked promising, but after maybe 30 minutes we see a significant drop in throughput, and the latency increases significantly. Compared to the same test with SSDs, we see a significant performance degradation of around 50%.

![general](general.png)

We can see in the release test for 8.9 that at the start it struggled as well, because some load test applications restarted, but later it run stable with ~51 PI/s and 101 Tasks per second, while the test with standard disks run with ~25 PI/s and ~58 Tasks per second.

It is interesting to note that base on [GCP docummentation the disk throughput](https://docs.cloud.google.com/compute/docs/disks/performance#pd-ssd_12) looks similar. Some could wonder what is the difference here, and why we need to use SSDs. The difference is that the standard disks have a much higher latency, which can cause significant performance degradation for an IO intensive application like Camunda.

![disk-perf](disk-perf.png)

This can especially seen in the record write and commit latencies.

![commit-latencies](commit-latencies.png)

These latencies have a significant impact on the overall performance of the cluster. As a leader needs to replicate and commit first an command before it is allowed to process and response to it. A follower needs to write and flush such, before it can acknowledge the replication to the leader. This means that the latency of the disk directly impacts the latency of processing requests, and can cause significant performance degradation as we also see in the metrics.

![overall-processing](overall-processing.png)


Using slower disks is not impacting the general processing performance, but also disrubting the underlying RAFT cluster. Slower followers, we natuarlly lag behind the leader, and this will cause more snapshot replications. Depending on the disk latency this could even cause more severe issues, retry-loops on append for example.

Depending on the state size this could put more load on the network as well.

![raft-snapshot](raft-snapshot.png)
![non-committed](non-committed.png)

## Conclusion

We were able to show what kind of negative impact slow disks can have. We were able to reproduce significant performance degradation, increased latency with using simple HDDs. With this it is visible that not only disk throughput is important but also disk latency.

This is why we [recommend using SSDs](https://docs.camunda.io/docs/next/self-managed/reference-architecture/kubernetes/#minimum-cluster-requirements) for the primary storage of Camunda clusters, as it significantly improves the performance and availability of the cluster.

## Chaos experiment: Slow disk on secondary storage




### Expected

### Actual

## Found Bugs




