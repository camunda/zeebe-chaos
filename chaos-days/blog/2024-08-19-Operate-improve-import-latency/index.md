---
layout: posts
title:  "Improve Operate import latency"
date:   2024-08-19
categories: 
  - chaos_experiment 
  - operate
tags:
  - performance
authors: zell
---

# Chaos Day Summary

[In our last Chaos Day](../2024-08-16-Operate-load-handling/index.md) we experimented with Operate and different load (Zeebe throughput). We observed that a higher load caused a lower import latency in Operate. The conclusion was that it might be related to Zeebe's exporting configuration, which is affected by a higher load.

In today's chaos day we want to verify how different export and import configurations can affect the importing latency. 

**TL;DR;** We were able to decrease the import latency by ~35% (from 5.7 to 3.7 seconds), by simply reducing the `bulk.delay` configuration. This worked on low load and even higher load, without significant issues.

<!--truncate-->

## Background 

_In the following I want to briefly explain a bit more the background of how exporting and importing play together. If you are already aware feel free to jump to the [next section](#chaos-experiment)._

---

To understand how the importing of Operate is affected and works, we first have to take a look at Zeebe.

Zeebe exports data to Elasticsearch via its Elasticsearch Exporter. The exporter collects data before sending it to Elasticsearch in bulk requests. The amount of data, which is collected in the exporter, is configurable and by default set to 1000 records per batch/bulk. Additionally, there is a memory limit which is taken into account that is set to 10 MB. When the bulk request is reaching that size, the request is sent as well. To cover cases of low load, there is a delay option, which is per default set to 5 seconds. This means, that every 5 seconds the bulk request is sent, even if it is not full.

This explains also the results from [our last Chaos Day](../2024-08-16-Operate-load-handling/index.md), where the import latency was around 5 seconds on a lower load. 

In the following, we have written down the sequence of steps a command has to take, and its resulting events until it is visible to the user in Operate. This should allow to better understand how and by what the import latency is affected, and what we might want to tune and experiment further.

```
User Command is sent to Gateway 
-->Gateway sents Command to the right Broker
---->Broker processes command and produces events
------>Events are exported by Broker to ES (worst case: 5s flush) 
-------->ES refreshes after one second
---------->Operate import processing/rewriting data
------------>ES refreshes after one second
-------------->Operate can query the data -> User can see the data 
```

About Elasticsearch and its default refresh configuration, etc. you can read [here](https://www.elastic.co/guide/en/elasticsearch/reference/current/tune-for-indexing-speed.html#_unset_or_increase_the_refresh_interval). 

Based on this, we know we have the following minimum delay: 
```
delay = 2 seconds (due to ES refresh)
      + (5 seconds from exporter on low load)
      + network delay 
      + processing delay 
      + Exporter and Operate data un/marshaling/processing
```

Today, we will experiment with the Elasticsearch exporter configurations to improve the import latency.

## Chaos Experiment

As we have seen [in a previous chaos day](../2024-08-16-Operate-load-handling/index.md) high load affects the importing latency positively. The thesis is that this is due to the export flush delay, which is mostly affecting the exporting on lower load.

Today we want to prove the following: 

> **Hypothesis**
> 
> When we set the exporting/flush delay to a lower value (ex. 1 second), we are improving the import latency for lower load scenarios without affecting the system negatively.

We can define the following `unknowns`, that we want to explore further as well:

 * It is not clear how lower flush delay affects the system on higher loads. 
 * It is not clear how smaller values (under 1 second) for the flush delay affect the system, no matter of high or low load.

### Expected

1. When we set the exporting/flush delay to a lower value (ex. 1 second), we are improving the import latency for lower load scenarios without affecting the system negatively.
2. When we set the exporting/flush delay to a lower value (ex. 1 second), we are improving the import latency for higher load scenarios, **but decreasing the import throughput**
3. When we set the exporting/flush delay to a small value (under 1 second), we are affecting the import throughput negatively

### Actual

As always, we set a base installation up to compare against. The load is moderate-to-low (15 PI/s). We can compare the data from the [last chaos day](../2024-08-16-Operate-load-handling/index.md) here as well.

<details>
<summary>Base: Helm install command</summary>
<pre>
helm install $(releaseName) $(chartPath) --render-subchart-notes \
 --set global.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebe.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebe.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebeGateway.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebeGateway.image.tag=ck-operate-benchmark-1ad8f375 \
 --set starter.rate=5 \
 --set worker.replicas=1 \
 --set timer.replicas=1 \
 --set timer.rate=5 \
 --set publisher.replicas=1 \
 --set publisher.rate=5 \
 --set camunda-platform.operate.enabled=true \
 --set camunda-platform.operate.image.repository=gcr.io/zeebe-io/operate \
 --set camunda-platform.operate.image.tag=ck-operate-benchmark \
 --set camunda-platform.elasticsearch.master.persistence.size=128Gi \
 --set camunda-platform.zeebe.retention.minimumAge=1d \
</pre>
</details>

We see similar results as on the [last Chaos day](../2024-08-16-Operate-load-handling/index.md#base).

![base-latency](base-latency.png)
![base-throughput](base-throughput.png)

We are able to import around 360 records per second, while Zeebe exports 413. Be aware that some are ignored by Operate.
A record has on average a delay of 5.69 seconds from being written by Zeebe to being imported by Operate (and written into the
end Elasticsearch index).

#### First experiment: Lower flush delay

> When we set the exporting/flush delay to a lower value (ex. 1 second), we are improving the import latency for lower load scenarios without affecting the system negatively.

To reduce the exporter flush delay we use the following configuration:

```yaml
exporters:
  elasticsearch:
    args:
      bulk:
        delay: 1
```

This can be set in our [benchmark-helm](https://github.com/camunda/zeebe-benchmark-helm) directly via: `--set zeebe.config.zeebe.broker.exporters.elasticsearch.args.bulk.delay=1`

<details>
<summary>Lower flush delay: Helm install command</summary>
<pre>
helm install $(releaseName) $(chartPath) --render-subchart-notes \
 --set global.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebe.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebe.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebeGateway.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebeGateway.image.tag=ck-operate-benchmark-1ad8f375 \
 --set starter.rate=5 \
 --set worker.replicas=1 \
 --set timer.replicas=1 \
 --set timer.rate=5 \
 --set publisher.replicas=1 \
 --set publisher.rate=5 \
 --set camunda-platform.operate.enabled=true \
 --set camunda-platform.operate.image.repository=gcr.io/zeebe-io/operate \
 --set camunda-platform.operate.image.tag=ck-operate-benchmark \
 --set camunda-platform.elasticsearch.master.persistence.size=128Gi \
 --set camunda-platform.zeebe.retention.minimumAge=1d \
 --set zeebe.config.zeebe.broker.exporters.elasticsearch.args.bulk.delay=1
</pre>
</details>


![lower-delay](lower-delay-base.png)
![lower-delay-throughput](lower-delay-base-load-throughput.png)

With setting the `bulk.delay` to one second, we were able to reduce the import latency by ~2 seconds, from 5.69 to 3.68 seconds.
That is a 35% decrease, while other factors stay the same. We can observe that the throughput stays the same (while of course, the load is rather moderate-to-low).

This proved our first hypothesis from above. :white_check_mark:

#### Second Experiment: Lower delay with higher load

> When we set the exporting/flush delay to a lower value (ex. 1 second), we are improving the import latency for higher load scenarios, **but decreasing the import throughput**

Similar to the first experiment we set the delay to one second, and increased the load in the same way as we did
[here](../2024-08-16-Operate-load-handling/index.md#high-load) before.

<details>
<summary>Lower flush delay with high load: Helm install command</summary>
<pre>
helm install $(releaseName) $(chartPath) --render-subchart-notes \
 --set global.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebe.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebe.image.tag=ck-operate-benchmark-1ad8f375 \
 --set camunda-platform.zeebeGateway.image.repository=gcr.io/zeebe-io/zeebe \
 --set camunda-platform.zeebeGateway.image.tag=ck-operate-benchmark-1ad8f375 \
 --set starter.rate=50 \
 --set worker.replicas=1 \
 --set timer.replicas=1 \
 --set timer.rate=50 \
 --set publisher.replicas=1 \
 --set publisher.rate=50 \
 --set camunda-platform.operate.enabled=true \
 --set camunda-platform.operate.image.repository=gcr.io/zeebe-io/operate \
 --set camunda-platform.operate.image.tag=ck-operate-benchmark \
 --set camunda-platform.elasticsearch.master.persistence.size=128Gi \
 --set camunda-platform.zeebe.retention.minimumAge=1d \
 --set zeebe.config.zeebe.broker.exporters.elasticsearch.args.bulk.delay=1
</pre>
</details>


![higher-load](lower-delay-high-load-latency.png)
![higher-load-throughput](lower-delay-high-load-throughput.png)

We can see that the latency has been increased a bit, versus the lower load benchmark, but it has improved compared to the
benchmark [the last chaos day](../2024-08-16-Operate-load-handling/index.md#high-load). :information: An interesting factor is that it seems that the throughput from Zeebe has changed as well, that in consequence increased the import throughput.

Looking into it further, we can see that the job and process instance creation and completion have changed by ~13-18 percent. Before we had around 130 process instance completion per second.

![backpressure-higher-load](backpressure-higher-load.png)

In the recent benchmark, we almost reach our target load (150 PI/s) with 147 process instance completions per second.

![backpressure-higher-load-lower-delay](backpressure-lower-delay-higher-load.png)

The reason seem to be the different backpressure. Backpressure has been decreased from ~20 % to 5-10%. This might be because our backpressure strategy has recently changed and now takes exporting into account. See also [related chaos day about this topic](../2024-07-25-Using-flow-control-to-handle-bottlenecked-exporting/index.md).

_**Update**_:

Looking into it further, the backpressure is not affected by the newest feature (as it was not enabled by default). This was discussed internally with the Zeebe team.

![higher-load-less-throughput-commit-latency](higher-load-less-throughput-commit-latency.png)

The slower benchmark, seem to have a degraded commit latency, which in consequence slows down the whole system. It is unclear right now, why this is.

The faster benchmark, with the configured exporting, has a much better commit latency. It is unlikely that the exporter configuration affected this part of the system. We will have to retry the both benchmarks.

![higher-load-higher-throughput](higher-load-higher-throughput-commit-latency.png)

_**Update 20-08-2024**_

We run additional benchmarks to verify the behavior on high load. This time we haven't seen any differences in terms
of processing performance in both benchmarks.

The benchmark without the configuration, reaches similar numbers (146 PI/s), as the other before.

![20-08-high-throughput](2024-08-20_high-load-throughput.png)

Benchmark with configuring the flush delay reaches comparable numbers. 

![20-08-high-throughput](2024-08-20_high-load-throughput-lower-delay.png)

During running the benchmarks we run into another issue, for which we opened the following [issue #21376](https://github.com/camunda/camunda/issues/21376).

##### Additional finding

An interesting additional finding has been done. When the Operate import fails or restarts (that can easily happen with preemptive nodes), then the importer backlog can be significant. This is especially an issue on higher constant load.

![import-delay](import-delay.png)

In our benchmark after the importer failed, it took ~20 minutes until the backlog was processed and the import latency was back to normal.

![recover-import-delay](import-delay-recover.png)

This shows that Operate, especially the importer is quite sensitive to restarts. This is likely to be changed and improved when
Operates importing mechanism is moved into Zeebe, as a separate exporter see [related GH issue](https://github.com/camunda/camunda/issues/16912).

On a lower load, the impact of an importer restart is negligible, as we can see below.

![no-impoact-low-load-restart](no-import-delay-restart-low-load.png)

#### Third experiment

> When we set the exporting/flush delay to a small value (under 1 second), we are affecting the import throughput negatively

We were not able to set the `bulk.delay` to a smaller value than 1 second, as the configuration only accepts longs. The values seem to be expected to be seconds. When setting it to zero, no improvement has been observed (versus one second).

## Potential improvements

* Allow to configure `bulk.delay` in non-second format (be able to specify the time/duration format)
* The `bulk.delay` configures a timer, which gets triggered with the given value. This means the flush can happen, even if flush was executed before causing flush with little buffers.
* Importing is highly affected by pod restarts, this can cause issues on higher load, due to a growing backlog. Making import idempotent, and scaling importers would help here.
* [Zeebe exporting latency can increase significantly without clear root cause #21376](https://github.com/camunda/camunda/issues/21376).



