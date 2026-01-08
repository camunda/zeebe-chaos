---
layout: posts
title:  "Optimizing cluster sizing using a real world benchmark"
date:   2024-10-14
categories: 
  - chaos_experiment 
  - performance
tags:
  - performance
authors: rodrigo
---

# Chaos Day Summary

Our first goal of this experiment is to use a benchmarks to
derive new optimized cluster configuration that can handle
at least 100 tasks per second, while maintaining low backpressure and low latency.

For our experiment, we use a newly defined realistic benchmark (with a more complex process model). More about this in a separate blog post.

The second goal is to scale out optimized cluster configuration 
resources linearly and see if the performance scales accordingly.

**TL;DR;**

We used a realistic benchmark to derive a new 
cluster configuration based on previous requirements. 

When we scale this base configuration linearly we see that the performance
increases almost linearly as well, while maintaining low 
backpressure and low latency.

<!--truncate-->

## Chaos Experiment

### Expected

We expect that we can find a cluster configuration that can handle at 100 
tasks second to be significantly reduced in resources in relation to our 
smaller clusters (G3-S HA Plan) since these can process significantly above 
our initial target.

We also expect that we can scale this base configuration linearly, and that 
the processing tasks rate to grow initially a bit faster than linearly due to 
the lower relative overhead, and if we keep expanding further to flatten due 
to the partition count being a bottleneck.

### Actual

#### Minimal Requirements for our Cluster

Based on known customer usage, and our own previous experiments, we 
determined that the new cluster would need to create and complete a 
baseline of 100 tasks per second, or about 8.6 million tasks per day.

Other metrics that we want to preserve and keep track are the backpressure 
to preserve user experience, guarantee that exporting speed can keep up 
with the processing speed, write-to-import latency which tells us how long 
it takes for a record to be written to being imported by our other 
apps such as the operator.

#### Reverse Engineering the Cluster Configuration

For our new configurations the only resources that we are going to change 
are the ones relevant to the factors described above. These are the 
resources allocated to our zeebe-brokers, gateway and elasticSearch.

Our starting point in resources was the configuration for our G3-S HA Plan
as this already had the capability to significantly outperform the current 
goal of 100 tasks per second. 

The next step was to deploy our realistic benchmark, with a payload of 5 
customer disputes per instance and start 7 instances per second, this 
generated approximately 120 tasks per second (some buffer was added to guarantee performance).

After this we reduced the resources iteratively until we saw any increase 
in backpressure, given that no there was no backlog of records, and no 
significant increase in the write to import latency.

The results for our new cluster are specified bellow in the tables, where 
our starting cluster configuration is the G3-S HA Plan and the new 
configuration cluster is the G3 - BasePackage HA.

| G3-S HA                | CPU Limit | Memory Limit in GB |
|------------------------|-----------|--------------------|
| operate                | 2         | 2                  |
| operate.elasticsearch  | 6         | 6                  |
| optimize               | 2         | 2                  |
| tasklist               | 2         | 2                  |
| zeebe.broker           | 2.88      | 12                 |
| zeebe.gateway          | 0.9       | 0.8                |
| **TOTAL**              | **15.78** | **24.8**           |

| G3 - BasePackage HA   | CPU Limit | Memory Limit in GB |
|-----------------------|-----------|--------------------|
| operate               | 1         | 1                  |
| operate.elasticsearch | 3         | 4.5                |
| optimize              | 1         | 1.6                |
| tasklist              | 1         | 1                  |
| zeebe.broker          | 1.5       | 4.5                |
| zeebe.gateway         | 0.6       | 1                  |
| **TOTAL**             | **8.1**   | **13.6**           |


##### Reduction in Resources for our Optimized Cluster

|                       |   CPU Reduction (%) |   Memory Reduction (%) |
|:----------------------|--------------------:|-----------------------:|
| zeebe.broker          |             47.92   |                   62.5 |
| zeebe.gateway         |             33.33   |                  -25.0 |
| operate.elasticsearch |             50.00   |                   25.0 |


Total cluster reduction: 

|                       | G3-S HA | G3 - BasePackage HA | Reduction (%) |
|:----------------------|--------:|--------------------:|--------------:|
| CPU Limits            |   15.78 |                 8.1 |            49 |
| Memory Limits         |    24.8 |                13.6 |            45 |


The process of reducing the hardware requirements was donne initially by 
scaling down the resources of the zeebe-broker, gateway and elasticSearch. 
The other components were left untouched, as they had no impact in our key 
metrics, and were scaled down later in separate experiences to maintain 
user experience.

#### Scaling out the Cluster

Now for the scaling procedure we intend to see if we can linearly increase 
the allocated resources and having a corresponding performance increase, 
while keeping the backpressure low, low latency, and user experience.

For this we started with the G3 - BasePackage HA configuration and 
incremented the load again until we saw any increase in backpressure, 
capture our key metrics and repeated the process for the cluster 
configuration resources respectively multiplied by 2x, 3x, and 4x.

This means that the resources allocated for our clusters were:

|               |   Base 1x |   Base 2x |   Base 3x |   Base 4x |
|:--------------|----------:|----------:|----------:|----------:|
| CPU Limits    |       8.7 |      17.4 |      26.1 |      34.8 |
| Memory Limits |      14.9 |      29.8 |      44.7 |      59.6 |

The results in the table bellow show the performance of our several cluster 
configurations:

|                          | Base 1x | Base 2x | Base 3x | Base 4x |
|:-------------------------|--------:|--------:|--------:|--------:|
| Process Instances/s      |       7 |      12 |      23 |      27 |
| Tasks/s                  |     125 |     217 |     414 |     486 |
| Average Backpressure     |      2% |      2% |      3% |      6% |
| Write-to-Import Latency  |     90s |    120s |    150s |    390s |
| Write-to-Process Latency |   140ms |    89ms |   200ms |   160ms |
| Records Processed Rate   |    2500 |    4700 |    7800 |   11400 |
| Records Exported Rate    |    2100 |    3900 |    6500 |    9200 |

This first observations is that the performance scales particularly well by 
just adding more resources to the cluster, particularly for a linear 
increase of the resources the performance as measured by tasks completed 
increases slightly less than linearly (comparing the 1x and 4x task/s we 
get 388% the initial rate).  

This a very good result as it means that we can scale our system linearly 
(at least initially) to handle the expected increase in loads.

Importantly, the backpressure is kept low, and the write-to-import latency 
only increases significantly if we leave the cluster running at max rate 
for long periods of time. For slightly lower rates the write-to-import 
latency is kept in the single digits of seconds or lower tens. This might 
imply that a these sustained max rates, the amount records generated starts 
to be too much for either ElasticSearch or our web apps that import these 
records to handle. Some further investigation could be done here to 
investigate the bottleneck.

Another metric also relevant but not shown in this table is the backlog of 
records not exported, which kept at almost null through all the experiments 
conducted.

### Bugs found

During the initial tests, we had several OOM errors in the gateways pods. 
After some investigation, we found that this was exclusive to the Camunda 8.
6.0 version, which consumes more memory in the gateway than the previous 
versions. This explains why the gateway memory limits were the only 
resource that was increased in the new reduced cluster configuration. 

