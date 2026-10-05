---
layout: posts
title:  "Testing broker performance for progressively lower RocksDB cache allocations"
date:   2025-10-31
categories: 
  - chaos_experiment
  - broker
  - rocksdb 
authors: rodrigo
---

# Chaos Experiment Summary

In this experiment, we aim to better understand the relationship between the RocksDB cache size and the performance of the broker. **We want to find out how low we can set the RocksDB cache size before performance degradation occurs.**

This topic is relevant because the RocksDB cache is a significant part of the memory allocation for a Zeebe broker, and understanding the minimum requirements can help optimize resource usage, especially in scenarios with many partitions or limited resources.

The discussion of this topic arose during the implementation of shared RocksDB cache among partitions, where we tried to understand how much memory we needed to allocate for default values. The experiments conducted here serve merely as a sanity check to validate our assumptions and to provide insights for documentation and best practices, since use cases vary widely from user to user.

With the introduction of shared RocksDB cache, we now have three different memory allocation strategies for the RocksDB cache memory in the brokers:

- When using `PARTITION`, the total memory allocated to RocksDB will be the configured number of partitions multiplied by the configured memory limit.
- When using `BROKER`, the total memory allocated to RocksDB will be equal to the configured memory limit.
- When using `FRACTION`, Camunda will allocate the RocksDB memory based on a fraction of the total memory.

:::note
When using the `PARTITION` strategy, the number of partitions used in the
calculation is the one configured and not necessarily the current number of
partitions in the cluster. This can differ when using dynamic partition scaling.
Therefore, it is recommended that when using the `PARTITION` strategy and dynamic scaling, you update the configured number of partitions after scaling operations.

The `PARTITION` allocation strategy is the default for 8.8 and 8.9, and `FRACTION` will become the default in 8.10.
:::

## Chaos Experiment

### Setup

* **Topology:** For the cluster, we use the typical setup of 3 brokers with 3 partitions, and 3 replicas per partition. The total RAM allocated to the cluster is 6 GB (2 GB per broker), which is a common configuration for our benchmarks.
* **Memory Allocation Strategy:** We used the `BROKER` allocation strategy with a 204.8 MB memory limit, which corresponds to 10% of the total available RAM (standard for our benchmarks).
* **Workload:** We use our standard Camunda benchmark tests. The workload consists of a simple process with a single service task. Our benchmark starts 150 process instances per second.
* **Zeebe Image:** We used a Zeebe image without minimum cache size validation (the current default is 32 MB per partition). This allows us to set the cache size to very low values and observe the effects on performance and backpressure.

### Experiment

The experiment consisted of progressively reducing the RocksDB memory limit until we could observe backpressure. Roughly reducing the memory limit by half in each step. We monitored the backpressure and the processing performance of the cluster at each step. To do this we need to change the configured limit and restart the cluster, this causes some backpressure at the start, and we wait until it stabilizes before we can determine if the new limit is sufficient or not. We considered the new limit sufficient if, after the initial backpressure caused by the restart, it eventually dropped back to 0% and stayed there while processing the workload.

It was also necessary to rebalance the cluster between memory configuration adjustments to ensure optimal and consistent processing conditions, specifically maintaining one leader partition per broker.

### Results

We only started to see consistent backpressure after reducing the limit below 10 MB per broker, which, surprisingly, is around 3.3 MB per partition (where two are replicas). With this memory value we observed backpressure with high fluctuations but generally less than 20%, and the cluster was still able to process the workload, albeit with some performance degradation. As expected the reason for the backpressure was backlog of unprocessed records in the partitions.

This is significantly lower than the current default minimum cache size of 32 MB per partition, which suggests that the requirements for short process instances with a simple process model can be quite low.

### Caveats/User Recommendations

A caveat is that these are not long-lived process instances. Results in real-world scenarios, where there are thousands of active process instances at any given moment, may have quite different requirements, and we expect that the minimum viable cache size would be higher in those cases.

### Future Work

As stated before, the goal of this experiment was to understand the general relationship between cache size and performance, and not to find the optimal cache size for a specific use case. Therefore, future work could involve testing with different workloads, such as long-running process instances or more complex process models, to see how the minimum viable cache size changes in those scenarios.

## Participants

* @rodrigo