---
layout: posts
title:  "Performance of Platform without Secondary Storage"
date:   2026-04-17
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - performance
authors:
  - zell
  - pranjal
  - jon
---

# Chaos Day Summary

In this Chaos Day, we conducted an experiment to evaluate the performance of our platform without the usage of the secondary storage. The goal was to understand how the system behaves under such conditions and find the maximum performance of a partition.

**TL;DR;** We observed that a cluster without secondary storage has a significantly higher throughput performance, as it is not throttled by the secondary storage, and can reach up to 400 PI/s without any issues. That is a factor of *1.7x* higher than the cluster with secondary storage.

<!--truncate-->

## Chaos Experiment

We wanted to understand the performance of the platform without secondary storage, and find the maximum performance of a partition. We ran two clusters, one with secondary storage and one without, and compared their performance under load. For better comparison and better performance we used gRPC. 

### Expected

We expected that the cluster without secondary storage would have a higher performance, as it would not have to write to the secondary storage, and would not be throttled by for example Elasticsearch (CamundaExporter).

What the maximum performance of a partition is, was not clear, but we expected that it would be higher than the cluster with secondary storage, which has a maximum performance of around 200 process instance completions per second (PI/s).

### Actual

As a base we started a load test with [our default configuration](https://github.com/camunda/camunda/blob/main/load-tests/camunda-platform-values.yaml) using gRPC and a max workload (300 PI/s).

#### Base: gRPC with secondary storage

We can see that while we receive 300 PI/s, the cluster with secondary storage is throttled at around 230 PI/s.

![](grpc-throughput.png)

The backlog of exporting to secondary storage is rather high, and causes us to have around ~40% backpressure, which is the reason for the throttling.

![](grpc-general.png)

Comparing to our daily tests this looks similar.

![](grpc-comparison-daily.png)

#### gRPC without secondary storage

At the beginning we had some issues how to properly configure the cluster without secondary storage. We found a useful documentation about how [to disable the secondary storage](https://docs.camunda.io/docs/self-managed/concepts/secondary-storage/no-secondary-storage/)

The important part is this (which does all the magic):

```yaml
global:
  noSecondaryStorage: true
```

Open question was whether Authorization would work. After experimenting with it, we found out that actually the right profiles are configured: `admin, broker, consolidated-auth` our initial authorizations are properly set (for our load tests), and the starter and worker are able to push data through the system.

The only issue was that the search for data availability was still enabled, which caused some failures (as obviously the REST API will not work without a secondary storage).

After disabling the search for data availability, we were able to run the load test without any issues. The load stabilized at 300 PI/s, and there was no backpressure.
We were able to increase to 400 PI/s, and it worked without any issues. When we increased to 600 PI/s, we started to see some backpressure, which indicates that we are reaching the limits of the system.

![](no-secondary-throughput.png)

The increased load first 400 PI/s later 600 PI/s, produced a bigger processing queue, causing backpressure at the end around ~40-50%. We can see that we are no longer able to complete 100% of process instances created.

![](no-secondary-overload.png)


Investigating a bit further we realized that we saw several JOB_TIMEOUT rejections, which could be either because Workers are too slow, or because we have too few workers. These job timeouts get rejected when job are already due, and actually want to time out the job but the completion still happens just before the actual timeout can be processed. Means either it is standing long in the queue of the worker (or due to the processing queue). We also saw some requestLimitExhaust rejections, which indicates that we are reaching the limits of the system.


![](no-sec-workers.png)


We disabled the flow control and throttling configs, and increased the number of workers. All of it didn't improve the performance, we observed some less timeout rejections at least.

In general the CPU and IOPS were at its limits, and we were not able to increase the performance further. We can see that the CPU is at 100% for the broker, and the IOPS are also at their limits, which indicates that we are reaching the limits of the system.

![](non-sec-cpu.png)
![](non-sec-write-iops.png)

We haven't increased the resources of the cluster, as we wanted to compare the performance with and without secondary storage with similar configuration.

### Conclusion

We can conclude that the cluster without secondary storage has a significantly higher throughput performance, as it is not throttled by the secondary storage, and can reach up to 400 PI/s without any issues.

That is a factor of *1.7x* higher than the cluster with secondary storage.

#### What we have learned:

 * We can run Camunda without secondary storage; which might be useful for users that have a certain performance requirement, and do not want to use a secondary storage. 
   * This has obviously some limitations/drawbacks, as several features (depending on the secondary storage) will not work. More you can find in our [documentation](https://docs.camunda.io/docs/self-managed/concepts/secondary-storage/no-secondary-storage/?configuration=helm#limitations-and-considerations).
 * Even without secondary storage we can use OIDC for Authentication, and Authorization