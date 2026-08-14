---
layout: posts
title:  "Camunda 8.7 and beyond - performance and reliability"
date:   2026-08-14
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Chaos Day Summary

Over the last week we ran several endurance and stress tests against Camunda 8.7, 8.8 and 8.9 (using the latest patch versions). In this post we want to summarize the results of our experiments, explain the differences between the versions, and share our findings with the community.

**TL;DR;** 

<!--truncate-->

## Chaos Experiment

We have selected for each version the last patch version available at the time of the experiment.

- 8.7: tbd
- 8.8: tbd
- 8.9: tbd


For each version we ran the same set of experiments, which included:

- stress test - link
- endurance test over several days - link

For each version, all tests were run in the same environment, using the same Kubernetes cluster and similar setup: three broker nodes and Elasticsearch as secondary storage with three nodes. The only difference was the version of Camunda being tested and version specific configuration and setup changes (embedded vs standalone gateways, Operate and Tasklist deployments, etc.).

## Endurance Test Results

All versions were able to handle the load for the duration of the test (we were looking at the same time window). While the general load looks fine some differences have been observed in the deeper level (see in a later section). 

![](endurance/general.png)

While 8.7 has a [different architecture](https://docs.camunda.io/docs/8.8/reference/announcements-release-notes/880/whats-new-in-88/#orchestration-cluster) and deployment with standalone gateways and separate web applications, we can see that it experience a similar issue. The gRPC traffic is not well distributed across the gateways, which leads to some gateways being overloaded while others are idle.This depends on the setup in general, in our load tests, we run in the same Kubernetes cluster and namespace and use the Kubernetes service to load balance the traffic. 

In a real production setup, there is likely put an ingress in front to load balance the traffic across the gateways, which would likely lead to a better distribution of the load. Of course having such imbalance and using embedded gateways can cause some more friction. This is something we will look into with [#59565](https://github.com/camunda/camunda/issues/59565).

![](endurance/general-2.png)

The general processing is well distributed across the partitions, and here we can see no difference.

What is interesting is that even on the output side, meaning writing to the secondary storage, we see similar load and well distribution across the elasticsearch nodes. Here we expected some difference, as we harmonized the indicies, to reduce duplication.

![](endurance/general-3.png)

### Resources

The first real difference we can see in the area of used resources.

The 8.8 version seem to use a bit more CPU compared to 8.7 and 8.9, while the memory usage is similar across all versions. The CPU usage has some outliers (due to restarts) so this might be one explanation. With 8.9 we seem to at least recover this to be under 8.7, which is a good sign.

Still on interesting fact is that with 8.7 Elasticsearch seems to use more CPU while with the other versions Camunda and Elasticsearch are on par.

Memory wise we seem to have a similar usage across all versions, which is likely related to the usage of JVMs and assigning similar set of resources. As we moved resources from previous Operate and Tasklist deployments to Camunda statefulsets.

![](endurance/cpu-mem.png)

When we look at the JVM metrics it is interesting to see that in all versions Optimize is the application with the highest memory usage (excluding ES here).


The write IOPS metrics show that for 8.7 we seem to write more data into Elasticsearch. The IOPs are almost doubled compared to 8.8 and 8.9, which is likely related to re-architecture, introducing the Camunda Exporter vs. the previous Importer deployments (including the harmonization of the indicies and the reduced duplication of data).

As described before all versions managed to do the same amount of work, but with 8.7 we seem to need less IOPs on the Zeebe side ~2k while with 8.8 and 8.9 we need ~3k IOPs to do the same amount of work. This is unexpected and we will look into this in more detail.

![](endurance/disk.png)

Checking the disk usage of the secondary storage we would expect that 8.7 uses more disk space, as we have seen more IOPs on the Elasticsearch side and we have Operate and Tasklist running separate and writing in different indices.

If we look deeper into the details we can see that in 8.7 Operate runtime indices are constantly increasing, while in 8.8 see a more stable pattern (skiping 8.9 here). 

![](endurance/runtime-indices.png)

Here we can already see some reliability improvement of 8.8+ in action. Previously the archiver was a separate fragile component (either running inside the Operate deployment or as a separate deployment). This has been moved with the re-architecture to the Camunda Exporter as well, and received several improvements over the last releases.

[The archiver](https://docs.camunda.io/docs/8.7/self-managed/operate-deployment/importer-and-archiver/) is in charge of moving completed process instances from the runtime indices to the historic (dated) indices. In previous versions this component was prone to failures and could lead to a backlog of completed process instances in the runtime indices, increasing their size. When an index (or shard) in Elasticsearch reached a certain size, it can [slow down the cluster and cause issues](https://www.elastic.co/docs/deploy-manage/production-guidance/optimize-performance/size-shards).

Looking at the historic indices, we can see that 8.8 for example was growing much faster than 8.7.

![](endurance/2026-08-14_14-00.png)

The archiver metrics show that 8.7 is not able to keep up with the amount of completed process instances.


![](endurance/archiver.png)

### Latency

Now coming to a set of metrics where we can see a real difference between the versions. One part is the processing latency where we seem to have increase latency with 8.8 and 8.9, it doubled. But here needs to say that the Camunda applications now do more in one deployment (which was distributed before) which means there is higher contention between resources.

On the positive side the re-achitecture of the Camunda Exporter (in 8.8) has improved the latency of data available to the user, as we were reducing one step of write-and-reading before aggregating. More details about this can be read [here](https://camunda.com/blog/2025/02/one-exporter-to-rule-them-all-exploring-camunda-exporter/).

![](endurance/latency-1.png)

The [data availability metric](https://camunda.github.io/zeebe-chaos/2026/01/08/Experimenting-with-data-availability-metric) we use in our load tests is not available in 8.7, as it requires the REST API (which was added in 8.8 as well).

So we need to make use of some older Operate Importer metrics to approximate the data availability. The data availability is calculated (in 8.8) as the time between a process instance creation request is sent and beining available via the REST API. This means we need to look at the Importer latency, which is the time between a record being written in Zeebe and imported by Operate (including written to Elasticsearch). Here, we need to add the Elasticsearch flush interval, which is set on the Operate Indices to two seconds, this gives us roughly the time the data needs to be available/visible to the user (excluding the network time between client and server).

![](endurance/import-latency.png)

For 8.7 this means we are on average at ~3.5 seconds for the importing latency, plus two seconds for the Elasticsearch flush interval, which gives us a total of ~5.5 seconds. For 8.8 we are at ~1.5 seconds for the data availability in total, which is a significant improvement.



### Throughput

As mentioned earlier on the throughput side of things, we can't spot a difference between the versions. Interesting is that we even write the same amount of records, but have an increase in IOPs.

![](endurance/throughput.png)

## Stress tests

In the following section we will look at the results of our stress tests, which we run against all versions from above. We are putting a higher load on the system (300 PI/s with one single task), which is expected to lead to some failures and issues. The interesting part is here to see how the system behaves under stress and if we can spot any differences between the versions, like maximum throughput, latency, or other metrics.


![](stress/general.png)

In our general metrics we can already see a huge difference between the versions and how they handle the load. While 8.7 is almost able to handle the load, the completion rate is around 90% and backpressure at ~20%. The versions 8.8 and 8.9 are struggling with the incoming load it seems, we have a completion rate of ~60% and backpressure at ~60% as well. 

Again here we can see that load is not well distributed across the gateways, which leads to imbalance. Still, the load at the partitions level is well distributed. We can see that the processing throughput difference is ~50% between 8.7 and 8.8/8.9.

![](stress/general-1.png)


We can see how the [performance degrades overtime in newer versions](https://github.com/camunda/camunda/issues/46993), while 8.7 is able to keep up with the load. This is something which is likely related to the accumulated state in the Camunda engine.



![](stress/general-2.png)

Interesting is that on the secondary storage much more documents are indexed in 8.8 and 8.9, which is likely because Camunda Exporter is now more tightly coupled with the Camunda application and able to keep up. The Camunda application gives in newer version better backpressure to clients, allowing to reducing stress on keeping latencies stable.

We can see a stable exporting backlog, considering this in request backpressure was added with 8.8 as well.

![](stress/general-3.png)

### Resources

In 8.7 we can see that the Zeebe gateway is heavily throttled. In addition all versions put high load on Elasticsearch, which is expected as we are writing a lot of data into the secondary storage.

![](stress/resources.png)

The overall used resources is in 8.8+ under pressure much lower than with 8.7.

![](stress/resources-2.png)


### Latency


![](stress/latency.png)

On the first glance latency metrics look similar across the versions. Under pressure the data availability goes now up to ~40 seconds on newer version. 

![](stress/import-latency.png)

When we look deeper into the details we can see that 8.7 is NOT able to keep up with the load on the web application and importer side. The importer latency spikes regularly to more than 4 hours, when we look over a longer time frame this goes even up to 9 hours. This was exactly one of the most complains we had with pre 8.8 architecture, having slow updates on Operate and Tasklist.

![](stress/import-latency2.png)

This is what the new architecture gives us, a more stable and reliable system, backpressure to clients, allowing to keep latencies stable.


## Conclusion

We have run and investigated load tests for the different versions of Camunda 8.7, 8.8 and 8.9. 

We clearly saw that the re-architecture of Camunda 8.8+ has improved the reliability and stability of the system. In our endurance test we have seen how unreliable the previous archiver component was. Under stress, we saw data availability and resource consumption improve significantly.

All of this comes with the cost of some processing performance (throughput and latency).

There are certain areas which we will look into in the future to improve our system even further:

 - like the IOPs on the Zeebe side, which seem to be higher in 8.8+ than in 8.7, while doing the same amount of work
 - gRPC load balancing across gateways, which is not well distributed in our tests and likely in production as well. This is something we will look into with [#59565](https://github.com/camunda/camunda/issues/59565)
 - accumulation of state in the Camunda engine, which seems to lead to performance degradation over time. This is something we will look into with [#46993](https://github.com/camunda/camunda/issues/46993)
