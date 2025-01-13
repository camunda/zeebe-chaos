---
layout: posts
title:  "Multiple Leader Changes"
date:   2020-10-13
categories: 
  - chaos_experiment 
  - broker
authors: zell
---

# Chaos Day Summary

Today I wanted to add new chaostoolkit experiment, which we can automate.
We already have experiments like restarting followers and leaders for a partition, but in the past what also caused issues was multiple restarts/leader changes
in a short period of time. This is the reason why I created [#39](https://github.com/camunda/zeebe-chaos/issues/39). 

<!--truncate-->

## Chaos Experiment: Multiple Leader Elections

In order to reduce the blast radius I setup an new Zeebe cluster with one partition (clusterplan: production-s). This makes it possible that we exactly restart the leader for that one partition.
Later we can also try it out with multiple partitions. In our automated environment it is anyway executed with multiple partitions.

### Steady State

All Brokers are ready and we are able to create new workflow instances on the partition one.

### Hypothesis

Even if we cause multiple leader changes due to broker restarts we should still be able to start new workflow instances on the corresponding partition.

### Method 

We requesting the Topology, determine the leader for partition one restart that corresponding node and wait until it is up again. We repeat that multiple times (three times).

### Result

The corresponding experiment was added via this [commit](https://github.com/camunda/zeebe-chaos/commit/11c3a96fc87991f649fb1559363ba335b2bf42a1).
We were able to prove that our hypothesis is true. we are able to handle multiple leader changes even in a short period of time.

#### Metrics

In the metrics we can see the behavior during the experiment and also we can see that it becomes healthy at the end.

![general.png](general.png)

![atomix.png](atomix.png)

I also run this with a cluster plan M cluster with the same results:

![multiple.png](multiple.png)

#### Chaostoolkit

```
(chaostk) [zell kubernetes/ ns:4ac065c1-a67e-4f47-8782-38a10d67515d-zeebe]$ chaos run multiple-leader-restart/experiment.json 
[2020-10-13 14:01:30 INFO] Validating the experiment's syntax
[2020-10-13 14:01:30 INFO] Experiment looks valid
[2020-10-13 14:01:30 INFO] Running experiment: Zeebe Leader restart multiple times experiment
[2020-10-13 14:01:30 INFO] Steady-state strategy: default
[2020-10-13 14:01:30 INFO] Rollbacks strategy: default
[2020-10-13 14:01:30 INFO] Steady state hypothesis: Zeebe is alive
[2020-10-13 14:01:30 INFO] Probe: All pods should be ready
[2020-10-13 14:01:30 INFO] Probe: Should be able to create workflow instances on partition one
[2020-10-13 14:01:32 INFO] Steady state hypothesis is met!
[2020-10-13 14:01:32 INFO] Playing your experiment's method now...
[2020-10-13 14:01:32 INFO] Action: Terminate leader of partition one
[2020-10-13 14:01:42 INFO] Pausing after activity for 5s...
[2020-10-13 14:01:47 INFO] Probe: All pods should be ready
[2020-10-13 14:02:32 INFO] Action: Terminate leader of partition one
[2020-10-13 14:02:41 INFO] Pausing after activity for 5s...
[2020-10-13 14:02:46 INFO] Probe: All pods should be ready
[2020-10-13 14:03:23 INFO] Action: Terminate leader of partition one
[2020-10-13 14:03:33 INFO] Pausing after activity for 5s...
[2020-10-13 14:03:38 INFO] Steady state hypothesis: Zeebe is alive
[2020-10-13 14:03:38 INFO] Probe: All pods should be ready
[2020-10-13 14:04:12 INFO] Probe: Should be able to create workflow instances on partition one
[2020-10-13 14:04:16 INFO] Steady state hypothesis is met!
[2020-10-13 14:04:16 INFO] Let's rollback...
[2020-10-13 14:04:16 INFO] No declared rollbacks, let's move on.
[2020-10-13 14:04:16 INFO] Experiment ended with status: completed
```

## Chaos Experiment: High Load

As mentioned last week @pihme has reported voluntarily that he wants to implement another chaos experiment.
He worked on #7, where we expect that even we put high load on the Zeebe cluster we will cause no leader changes. This was in the past an failure case, where high load disrupted the cluster.


### Steady State

All Brokers are ready and we can create workflow instances on all partitions. We store the begining topology for later.

### Hypothesis

We expect that even on high load we are not able to disrupt the cluster and that this will not cause any leader changes.

### Method

Put high load on the cluster for several minutes, via creating workflow instances

### Result

@pihme create a new PR to add the experiment [#41](https://github.com/camunda/zeebe-chaos/pull/41) 


#### Metrics

We see that we already put some load on the cluster but it is not enough to exhaust the request limits and reach back pressure.

![highload](highload.png)

We neeed to find a good way how put high load on the Zeebe cluster. We will continue on this.

## Participants

  * @pihme
  * @zelldon


