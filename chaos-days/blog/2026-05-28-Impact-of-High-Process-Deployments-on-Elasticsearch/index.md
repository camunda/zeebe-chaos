---
layout: posts
title:  "Impact of High Process Deployments on Elasticsearch"
date:   2026-05-28
categories:
  - chaos_experiment
  - bpmn
tags:
  - availability
authors:
  - pranjal
  - zell
  - jon

---

# Chaos Day Summary

On this Chaos Day, we conducted an experiment to observe the impact on Elasticsearch when deploying a large number of process versions to a Camunda cluster, and how that pressure propagates through Optimize, the Zeebe Elasticsearch exporter, and ultimately back to the Camunda engine itself.

**TL;DR;** We discovered a 1:1 relationship between Optimize indices and deployed processes. Once the Elasticsearch cluster reaches its maximum normal shard limit (default `1000` per node, e.g. `3000` for a 3-node cluster), it stops creating new indices. The Zeebe engine remains unaffected initially. Our hypothesis is that the failure will cascade the next day, once the Zeebe Elasticsearch exporter attempts to create its new dated index and gets rejected — we expect to observe this and confirm in a follow-up. If confirmed, this would ultimately affect Camunda, causing unrecoverable backpressure.

<!--truncate-->

## Chaos Experiment

### Setup

To simulate high process deployments, we used [`c8ctl`](https://github.com/camunda/c8ctl) to deploy multiple versions of the simple [`one_task`](https://github.com/camunda/camunda/blob/main/load-tests/load-tester/src/main/resources/bpmn/one_task.bpmn) process. We deployed approximately `~2000` versions of this process and ran a task alongside it to observe the system's behavior under load. The experiment was run in the `c8-chaos-22` namespace.

### Expected

Deploying a large number of process definitions and versions should not degrade the availability of the core Camunda workflow engine (Zeebe). If reporting and archival systems (like Optimize) hit capacity limits, they should fail gracefully without blocking the Camunda engine's execution and progress.

### Actual

We observed that the number of Optimize indices grows in a 1:1 relationship with the number of deployed process definitions. As we ramped up deployments in the `c8-chaos-22` namespace, the Optimize index count grew linearly with each new process version, climbing from a baseline of ~20 to over `1400` indices within ~20 minutes:

![](optimize-indices-linear-growth.png)

We can observe this 1:1 pattern in production clusters as well, where each deployed process gets its own dedicated Optimize index:

![](production-cluster-example-optimize-index.png)

Each Optimize index is also configured with a default of `1` shard per index (see the [Optimize importer/archiver configmap](https://github.com/camunda/camunda-operator/blob/8a82eb615853421330684d765cfb7d577c836b2a/templates/optimize_configmap_importer_archiver.yaml#L32)), so the index count effectively equals the shard count contributed by Optimize.

Once the maximum Elasticsearch shard limit is consumed, the cluster blocks the creation of any new indices or shards. Optimize logs thousands of errors as it fails to import data.

![](error-logs-in-optimize.png)

Camunda continues to make progress temporarily because the archiver is decoupled and the current runtime indices already exist. The orchestration cluster keeps processing instances even after the shards have filled up:

![](orchestration-cluster-after-shards-filled.png)

#### The Critical Failure (Hypothesis)

So far, the engine itself is still progressing. However, we expect the real damage to surface the next day. Our hypothesis is the following: when the Elasticsearch exporter attempts to create a new dated index (e.g. for the daily Zeebe record indices), the request will be rejected by Elasticsearch because no new shards can be allocated. This should block the exporter entirely, which would cascade into unrecoverable backpressure on the Camunda engine. At that point, process execution would halt and manual intervention would be required to restore the cluster.

This part is not yet confirmed in this experiment, we will observe the cluster over the next day to verify the cascading failure and update this post with the findings.

## What We Learned

* **Optimize creates one index per deployed process model.** This 1:1 relationship is the root cause of the shard pressure. Combined with the default `1` shard per Optimize index ([source](https://github.com/camunda/camunda-operator/blob/8a82eb615853421330684d765cfb7d577c836b2a/templates/optimize_configmap_importer_archiver.yaml#L32)), the shard count grows linearly with the number of deployed processes and exhausts the cluster shard budget prematurely.
* **Elasticsearch's shard limit is a hard ceiling for the whole platform.** Once `cluster.max_shards_per_node` is reached, no component can create new indices — Optimize import and archiver fail first.
* **Camunda's engine stays healthy only as long as no new index needs to be created.** Runtime processing continues to work because the existing dated indices are still writable. The risk surfaces when a new dated index is needed (our hypothesis for the next-day cascade).
* **Decoupling helps, but only delays the impact.** The archiver being decoupled from the engine hot path bought us time, but it did not prevent the eventual cascade once the exporter needs a new index.

## Possible Improvements

* **Short-term:** Dynamically balance shards by adding an extra Elasticsearch node, increasing the `cluster.max_shards_per_node` limit, or deleting older indices (which results in data loss).
* **Preventative:** Implement a capacity limit to block customers from deploying process models beyond a threshold that would put the cluster shard budget at risk.
* **Long-term:** Refactor Optimize to use a different, more scalable data model that does not require a 1:1 index-to-process mapping (e.g. a single shared index keyed by process definition).
