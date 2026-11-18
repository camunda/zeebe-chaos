---
layout: posts
title:  "Identical clusters, different results: the n2 CPU lottery"
date:   2026-11-19
categories:
  - chaos_experiment
tags:
  - performance
  - reliability
authors:
  - zell
---

# Chaos Day Summary

Our daily load test had a puzzling pattern: the same test, on the same build, with the same configuration, sometimes ran clean and sometimes got stuck at 40 to 50% rejected requests. This chaos day summarizes the investigation of that pattern (tracked in [camunda/camunda#68316](https://github.com/camunda/camunda/issues/68316)) and what we changed.

**TL;DR;** There was not one cause but three, and one of them is likely relevant to everyone running on GKE `n2` machines. The impact is listed last.

- **The host.** GKE `n2` nodes without a minimum CPU platform come in a fast and a slow class. Both report the same CPU model and flags. The slow class runs the same code at roughly 37% of the speed in a CPU benchmark. Google confirmed that the platform assignment is automatic and depends on availability, so it is effectively random.
- **The test setup.** Brokers throttled at 3 CPU, too few worker threads, too little starter CPU, and a write rate limit capped the result, so part of what we measured was our own setup.
- **The test itself.** The regression test ran at the capacity of the system, so one test mixed "did it regress" with "how much can it do", and normal spread looked like a regression.
- **The cost.** Stuck runs delivered about 25% less goodput on the same CPU. We now pin nodes to Ice Lake, run a more fitting setup, and use a rate below the capacity.

<!--truncate-->

## Chaos Experiment

We did not inject a fault here. The "experiment" was a systematic comparison: identical clusters, identical build, then differences in the cluster, the pods, and the nodes.

### Expected

Identical clusters should deliver identical throughput within a few percent. A cluster that sits at 40 to 50% rejected requests should have an event in its window, such as a restart or a leader change, or a configuration difference.

### Actual

#### How big was the difference?

In the last 14 daily runs, six were stuck (more than 35% rejected requests), four recovered (less than 15%), and four sat in between. The stuck ones delivered about 744 accepted requests per second against 994 for the recovered ones, and about 3718 records per second against 4967, while using the same CPU (8.7 against 8.6 cores). That is about 25% less goodput on the same hardware, so the cost is real and not a measurement artifact.

![Goodput of stuck and recovered daily runs on the same CPU](stuck-vs-recovered.svg)

#### Ruling things out, cheapest first

We worked through the same order every time, and it is worth keeping:

1. **Cluster level.** Backpressure (dropped requests against received requests), CPU throttling per broker, processing latency. A pod with CPU request equal to limit can throttle and then looks like slow code.
2. **Events in the window.** Pod replacement, restart, leader change. After a failover one broker carries extra partitions and saturates, so the window is disturbed even though the hosts are fine.
3. **Configuration.** The Helm values, node pool labels, and image tag against a good cluster.
4. **Per broker.** One broker rejected about 50% of its partitions while a healthy leader rejected about 22%.
5. **The host.** CPU model, clock, a CPU benchmark, steal time.

The first four steps explained some runs, but not all of them. The outlier broker had no event and a normal configuration, so we looked at the node.

#### Two classes of the same machine

We ran `sysbench` on fresh, idle `n2-standard-4` nodes and found two distinct classes. Spot was not the cause: idle on-demand nodes of our stable pool also sat in the slow class.

| | Fast class | Slow class |
|---|---|---|
| `sysbench` CPU events | 2568 to 2599 | 967 to 975 |
| `cpu MHz` in `/proc/cpuinfo` | exactly 2799.998 | 2800.19 to 2800.33 |
| CPU model, flags, platform reported by GCE | `Intel Cascade Lake` | `Intel Cascade Lake` |
| Steal time | 0 | 0 |

The slow class reached about 37% of the fast one in the CPU benchmark and about 70% in the memory benchmark. A `stress-ng` CPU run showed no difference, so the benchmark you pick matters.

Two things stand out:

- **Steal time stays 0.** A clean steal does not clear a host. The hypervisor does not report the difference.
- **The only visible hint is the clock value.** The slow nodes report a `cpu MHz` that is slightly above 2800, while the fast ones report exactly 2799.998. That is a cheap detector for fresh `n2` nodes:

```bash
grep -m1 'cpu MHz' /proc/cpuinfo
```

On the 12 fresh nodes we checked in detail, 5 were fast and 7 were slow. A scan of 57 brokers in our running clusters found 44% on slow hosts. Pinning the load test to one zone did not help: 0 of 5 clusters recovered, and 4 of 5 had a slow node. An AMD `c3d` pool showed no slow class in 6 nodes.

![Sysbench results of fast and slow nodes](sysbench-classes.svg)

#### Why does this happen?

The Google documentation lists two CPU platforms for `n2`: Cascade Lake and Ice Lake. By default Cascade Lake is used up to 80 vCPUs, and neither a larger machine nor vNUMA is a way to get Ice Lake. We asked Google, and they confirmed that without `min_cpu_platform` the setting stays `AUTOMATIC`, which gives Cascade Lake if it is available in the zone. The assignment depends on availability, so you can get either class on the same machine type.

With `min_cpu_platform` set to Ice Lake, all 14 nodes of a test pool were in the fast class. The `cpu MHz` reads about 2600.0, nothing like the other values, the platform is reported as Ice Lake, and `sysbench` reached 2502 to 2613 events, the same range as the fast Cascade Lake nodes.

We pinned all our `n2` node pools to Ice Lake on 2026-11-10. A node benchmark over all live nodes afterwards found no slow class.

Note that the `cpu MHz` detector is specific to Cascade Lake pools. On an Ice Lake pool, the value is about 2600 and the check does not separate classes.

#### The test itself

Two more things contributed, independent of the hosts:

- **The setup capped the result.** Several limits held the result back:
  - Brokers were throttled at 3 CPU. With three partitions, an embedded gateway and request retries, that left too little headroom.
  - The load generator had too few worker threads and too little starter CPU.
  - A write rate limit backpressured the setup without secondary storage.

  We now run 4 CPUs as request and limit for each broker on larger `n2-standard-8` nodes, 30 worker threads, a starter with up to 1 CPU, and the default write flow control without our own limit.
- **The rate sat at the capacity.** We measured a capacity of about 700 process instances per second for this setup. A regression test at that rate cannot tell a regression from noise. We now run the regression test at 650, which is stable and shows a 10% regression, and treat the capacity as a separate measurement. The latest daily run completed 648.5 process instances per second at that rate, with 2.2% backpressure. A faster build is not a finding for this test.

The records per CPU-second metric is useful to compare brokers inside one cluster, but it is only relative. We do not use it as an absolute efficiency number.

#### What we could not explain

One more outlier broker, about 10% less efficient per CPU-second with 28% CPU throttling, had a normal node benchmark and no event in its window. It was a single sample and we could not prove a cause. In a verification cluster, a single broker was slow for hours and recovered on its own. Our best guess is noisy neighbors, which is suspected but not proven. The margin below the capacity absorbs this for now. The evidence is gone unless it is captured during the phase, so if it shows up again, capture a node benchmark, steal, and CPU pressure right then.

## Conclusion

Identical clusters are only identical if the hosts are. A fresh GKE `n2` node can be fast or slow with the same CPU model, flags, and platform reported, and the difference cost our stuck runs about 25% of throughput.

- Set `min_cpu_platform` for `n2` pools, or benchmark a sample of fresh nodes before you trust numbers from a new pool, zone, or machine type.
- Pinning needs a node pool re-create and can reduce the zone capacity, so plan for it.
- Keep a regression test below the measured capacity, and measure the capacity separately.
- Check the cheap things first: backpressure, throttling, events, configuration, then the host.

We told the SaaS team, which does not set `min_cpu_platform` today, and they are following up on whether and where to apply it.

## Found Bugs and Follow-ups

- [camunda/camunda#68316](https://github.com/camunda/camunda/issues/68316): the parent issue of the investigation.
- [camunda/camunda#69692](https://github.com/camunda/camunda/issues/69692): re-measure the load test capacity on every release and compare it to the regression rate.
- Sizing guidance on the hardware assumptions is a candidate for the docs, see the sizing page in the best practices.
