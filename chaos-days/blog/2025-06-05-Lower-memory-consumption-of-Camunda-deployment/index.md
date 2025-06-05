---
layout: posts
title:  "Lower memory consumption of Camunda deployment"
date:   2025-06-05
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Chaos Day Summary

I'm back to finally do some load testing again. 

In the past months we have changed our architecture, to deploy instead all of our components as a separate deployment 
we have one single statefulset. This statefulset is running our single Camunda standalone application, 
combining all components together. 

![simpler deployment](simpler-deployment.png)

More details on this change we will share on a separate blog post. For simplicity, in our load tests (benchmark helm charts), we
 combined all the resources we had split over multiple deployment together, see related PR [#213](https://github.com/camunda/zeebe-benchmark-helm/pull/213).

We are currently running our test with the following resources per default:

```yaml
    Limits:
      cpu:     2
      memory:  12Gi
    Requests:
      cpu:      2
      memory:   6Gi
```

In today's Chaos day, I want to look into our memory consumption and whether we can reduce our used requests and limits.

**TL;DR;** We were able to reduce the used memory significantly. 

<!--truncate-->

## Checking weekly benchmarks

Before I started to experiment, and reduce it. I validated whether we actually have room for improvement. For that I check our
weekly load tests. These are tests we start every week, that are running for four weeks straight. These can be used as a good reference point (base).

I picked the mixed load test, which is running our realistic benchmark using more complex process model, covering more elements, etc.

![base general](base-general.png)

When we look at the general metrics, we can see it reaches on average ~100 task completions per second. As we use pre-emptive nodes it might happen that workers, starters or even the Camunda application is restarted in between.


## 1. Experiment: Reduce memory limits



As a first experiment I tried to reduce the gener

```yaml
    Limits:
      cpu:     2
      memory:  4Gi
    Requests:
      cpu:      2
      memory:   4Gi

```

### Expected

### Actual

## Found Bugs


