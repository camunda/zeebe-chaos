---
layout: posts
title:  "Stress testing Camunda"
date:   2025-11-27
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - performance
  - stress-testing
authors: zell
---

# Chaos Day Summary

In today's chaos experiment we focused on stress testing Camunda 8 platform under high load conditions. We simulated a large number of concurrent process instances to evaluate the system's performance and reliability.

Due to our recent work in supporting [load tests for different versions](https://github.com/camunda/camunda/issues/38829), we were able to compare how different Camunda versions handle stress.

**TL;DR;** We found that Camunda 8.7.x performs best under high load, followed by main branch and 8.6.x. The latest version 8.8.x showed lower throughput, but with increased resources it was able to improve performance. Latency was lowest (best) in 8.8.x with increased resources.

<!--truncate-->

## Chaos Experiment

On a weekly basis we run [endurance tests](https://github.com/camunda/camunda/blob/main/docs/testing/reliability-testing.md#variations) to validate the stability of our systems. This time, we decided to push the limits further by increasing the load significantly and observing how Camunda handles the stress.

Additionally, we wanted to see how far we can go, and how this differs between versions. Therefore, we compared the performance of Camunda 8.8.x and 8.7.x under identical stress conditions.

### Setup

Details on the setup can be read in our  [reliability testing](https://github.com/camunda/camunda/blob/main/docs/testing/reliability-testing.md#setup) documentation.

Important to know is that the architecture has changed slightly over the versions. In 8.8.x we have a single Camunda application deployed, while in earlier versions we have a split between broker and gateway architecture. This change has implications on how the system handles load and scales.

![setup](setup.png)

### Load Generation

![setup-load](setup-load-test.jpg)

We have a custom [load generation application](https://github.com/camunda/camunda/tree/main/load-tests/load-tester) (split into starter and worker application), which we deploy separately from Camunda. The starter creates process instances at a configurable rate, while the worker completes the corresponding service tasks.

![one-task](one_task.png)

We will start simple, with a process that has a single service task. This is not very realistic, but gives us a good sense of maximum load, as this is one of the smallest process (including a service task) we can model. Reducing the used feature set to a small amount allows easy comparison between tests, as fewer variations and outside factors can influence test results. Additionally, it is a model we use in [our endurance test](https://github.com/camunda/camunda/blob/main/docs/testing/reliability-testing.md#normal-artificial-load) as well, allowing us to compare it and know where to start of. Our endurance tests normally have a load of 150 process instances per second (PI/s). Where the [payload is rather small ~0.5 KB](https://github.com/camunda/camunda/blob/main/load-tests/load-tester/src/main/resources/bpmn/typical_payload.json). In our stress test the starter application will create process instances at a rate of 300 per second, while we will have six worker applications deployed processing the service tasks.


### 8.8.x Results

#### Single Service Task

After setting up the load test environment and starting the load generation, we monitored the system's performance metrics, including CPU usage, memory consumption, latency (gateway response time and process instance execution time) and throughput (how many process instances, tasks and flow-node instances were completed per second).

![88-one-task-results](88-one-task.png)

Looking at the dashboard we can see that we reach the limit of our cluster. We have high back pressure (and cluster load near 100%). Our system is heavily CPU throttled (~100% CPU steal). This means that the system is not able to keep up with the incoming load of 300 PI/s.

![88-one-task-latency](88-latency.png)

Interesting (or important) to note is that our backpressure mechanism allows us to keep the latency always steady and low. But not that low as compared to other versions, we will see later.

##### Increasing resources

Out of interest I increased the resources of the cluster (increasing CPU to 3 cores; memory increase was not necessary as we were not reaching our limit). 

![88-one-task-3-cpu](88-one-task-3-cpu.png)

When increasing the CPU resources, adding one core, we were able to increase the throughput by ~37% (220/160=1.375). We are still not able to handle the full load of 300 PI/s.

![88-one-task-3-cpu-latency](88-one-task-latency-3-cpu.png)

The p50 latency has been decreased significantly, while the p99 is similar to before. In general, we are seeing only one pod being CPU throttled. Likely related to the fact that we not properly distribute our load across the gateways, see issue [9870](https://github.com/camunda/camunda/issues/9870).

### 8.7.x Results

#### Single Service Task

The results for 8.7.x are quite different. Here we can see that we are able to handle much higher load ~ 246 PI/s. The backpressure is lower, and the CPU usage (and throttling) is not as high as in 8.8.x.

Surprisingly, the memory usage is higher in 8.7.x compared to 8.8.x. Short research showed that the broker got in 8.7 [4GB of memory assigned](https://github.com/camunda/camunda/blob/stable/8.7/zeebe/benchmarks/camunda-platform-values.yaml#L163), and [25% is used by the JVM](https://github.com/camunda/camunda/blob/stable/8.7/zeebe/benchmarks/camunda-platform-values.yaml#L91). While in 8.8 we have [2GB assigned to the Camunda application](https://github.com/camunda/camunda/blob/main/load-tests/camunda-platform-values.yaml#L76). Additionally, in 8.8 we reduced the RocksDB memory (as [experiment](https://github.com/camunda/camunda/issues/31706#issuecomment-2944455152)) to [64 MB per partition](https://github.com/camunda/camunda/blob/main/load-tests/camunda-platform-values.yaml#L149) (instead of previously 500MB). This should explain the difference.

![87-one-task-results](87-one-task.png)

The latency is also lower in 8.7.x compared to 8.8.x. The p50 latency is around 170ms for gateway requests and 240ms for the PI execution, while the p99 latency goes up to 700ms under high load.
![87-one-task-latency](87-one-task-latency.png)

### 8.6.x Results

#### Single Service Task

The results for 8.6.x are comparable to 8.8.x with more resources, but still lower than 8.7.x. We are able to handle around 220 PI/s with similar backpressure and CPU throttling as in 8.8.x with 3 CPU cores.

![86-one-task-results](86-one-task.png)


Even the latency is comparable to first 8.8.x test, while the p99 is much higher.

![86-one-task-results](86-one-task-latency.png)


### Main

Out of interest and for better comparison I also ran the same test on the main branch (which will become 8.9.x). Here the results are better than 8.8.x with 3 CPU cores. We are reaching ~230 PI/s with similar latency characteristics. Still, it is not as good as 8.7.x.

![main-one-task-results](main-one-task.png)


Looking at the latency we can see that it is comparable to the 8.7.x test results.

![main-one-task-results-latency](main-one-task-latency.png)

## Summary of Results

In terms of throughput 8.7.x performs best, followed by main and 8.6.x. Increasing resources in 8.8.x helps, but it is still not able to match the performance of 8.7.x. Just looking at 8.7 and 8.8 this means a decrease of ~35% in throughput (160 / 246 = 0.65).

The latency is lowest in 8.8.x with increased resources, followed by main and 8.7.x. 8.6.x has the highest latency among the tested versions.

| Version  | Throughput (PI/s) | p50 PI execution Latency (ms) | p99 PI execution Latency (ms) | CPU Throttling |
|----------|-------------------|-------------------------------|-------------------------------|----------------|
| 8.7.x    | ~**246**              | ~200                          | ~700                          | 80% one pod    |
| 8.6.x    | ~220              | ~400                          | ~960                          | 80+% all pods  |
| 8.8.x    | ~160              | ~370                          | ~700                          | 90+% all pods  |
| 8.8.x (3 CPU) | ~220              | ~**90**                           | ~**490**                          | 80% one pod    |
| Main     | ~230              | ~180                          | ~497                          | 90+% two pods  |


## Next Steps

Based on the results above we will likely continue our investigation into the performance differences between the versions. We will analyze the changes made in the architecture and codebase to identify potential bottlenecks or optimizations that could explain the observed performance variations. Likely, we will also have to look into the resource allocation and configuration settings to see if there are any differences that could impact performance.

Furthermore, we plan to explore the impact of different process designs and workloads on the system's performance.
A common use case for Camunda are straight through processes with service tasks. Therefore, we designed a simple BPMN process that consists of a start event, ten service tasks, some intermediate timer catch events, and an end event. The service tasks are configured to be handled by the worker application. In one of our next experiments, we will run the same stress test with this process model to see how the system handles more complex workflows under high load.

![ten_tasks](ten_tasks.png)



