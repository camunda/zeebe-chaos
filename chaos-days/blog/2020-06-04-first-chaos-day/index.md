---
layout: posts
title:  "First Chaos Day!"
date:   2020-06-04
categories: 
  - chaos_experiment
  - broker
authors: zell
---

# Chaos Day Summary

 First Chaos day :tada:

 * Documented failure cases for exporter (already some exist, it seemed) gave me a new idea for ZEP
 * Introduced Peter to our Chaos Repository, discussed a bit about the hypothesis backlog, reopened the Chaos Trello board where we will organize ourselves
 * Run a chaos experiment, where we put high CPU load on the Leader [#6](https://github.com/camunda/zeebe-chaos/issues/6)

<!--truncate-->

## Chaos experiment:

 * We wanted to decrease the blast radius with only one partition, but we found an bug where this seemed not to be possible [#4664](https://github.com/zeebe-io/zeebe/issues/4664)
 * We run the experiment with 2 partitions and put really high CPU load on the Leader (internally in the pod), we expected that this will not impact the complete cluster. That at most we have a leader change because the leader is not able to send heartbeats in time. After removing the cpu load we should be back on our throughput base line, where we start and complete around 70 - 80 workflow instances per second.
 * The results where quite promising we had no leader change at all. The leader was able to send heartbeats in time and the backpressure did a good job and drop more requests. After reducing the cpu load we went back to our steady state. 

![img](result-chaos.png)

## Participants

 * @pihme
 * @zelldon
