---
layout: posts
title:  "Coalescing Raft flushes"
date:   2026-07-28
categories:
  - chaos_experiment
  - performance
tags:
  - performance
authors: lena
---

# Chaos Day Summary

Every record Zeebe replicates is `fsync`ed before it counts as durable. Today we benchmarked
[a change](https://github.com/camunda/camunda/pull/58844) that keeps that guarantee but stops
issuing the `fsync`s we don't need.

**TL;DR;** Coalescing removes 23–68% of all `fsync`s at identical durability. Where flushing is the
bottleneck that is **+31% throughput**; on a fast disk under load it buys no throughput but **4.9×
better commit latency**; on an idle-ish workload it does nothing; and on a CPU-starved broker it
costs more than it returns. Two findings stand out beyond the flag itself: high flush latency comes
from a *saturated* disk rather than from the disk type, and the leader never actually coalesces — its
entire win is deduplication, which we can get in the default flusher from three lines.

<!--truncate-->

## What the change does

The Raft log is flushed in two places, and `flush-coalesced` changes them differently. The asymmetry
explains most of the results below.

The **leader** flushes whenever the commit index advances, because it counts itself in the quorum
([`RaftContext:568`](https://github.com/camunda/camunda/blob/a0e42be9225f2f572fe0803cc519d1d7f48763a4/zeebe/atomix/cluster/src/main/java/io/atomix/raft/impl/RaftContext.java#L568)).
A single `fsync` makes the *whole* appended tail durable, and the leader appends far ahead of the
commit index, so most of these flushes ask for an index an earlier `fsync` already covered.
Coalescing skips those
([`CoalescedFlusher:73`](https://github.com/camunda/camunda/blob/a0e42be9225f2f572fe0803cc519d1d7f48763a4/zeebe/atomix/cluster/src/main/java/io/atomix/raft/storage/log/CoalescedFlusher.java#L73)).
When a flush *is* needed the leader still blocks its Raft thread on the result.

The **follower** flushes before acknowledging an append, and there the Raft thread is genuinely
released while the `fsync` runs
([`PassiveRole:926`](https://github.com/camunda/camunda/blob/a0e42be9225f2f572fe0803cc519d1d7f48763a4/zeebe/atomix/cluster/src/main/java/io/atomix/raft/roles/PassiveRole.java#L926)),
so appends arriving during a flush are covered by the next one. But the leader only keeps
[two appends in flight per follower](https://github.com/camunda/camunda/blob/a0e42be9225f2f572fe0803cc519d1d7f48763a4/zeebe/atomix/cluster/src/main/java/io/atomix/raft/partition/RaftPartitionConfig.java#L39),
capping follower-side batching at ~2 appends per `fsync`.

Unlike `flush-delay` and "flushing disabled", durability is untouched: appends are still only
acknowledged, and commits still only advance, once a covering `fsync` succeeded.

## Chaos Experiment

Clusters on the benchmark cluster with **no secondary storage** and **Optimize disabled**
(`secondary-storage-type=none`, `enable-optimize=false`), so only the engine and Raft are in the
picture. Workload is the `max` scenario: a minimal one-task process over gRPC, 3 brokers /
3 partitions / replication factor 3. Both arms of every pair are created together, in the same
availability zone, from the same image lineage — the only differences within a pair are the image and
`CAMUNDA_CLUSTER_RAFT_FLUSHCOALESCED=true`.

Three disk types: zonal `pd-ssd`, zonal `pd-standard`, and regional `pd-standard` (every write
replicated synchronously across two zones — the "network-attached storage" case the change targets).
Brokers are 3 CPU / 2 Gi unless stated otherwise.

### Expected

1. **Fewer `fsync`s** — direct flushing issues one per commit advance and one per follower append, and
   most are redundant.
2. **Higher throughput where flushing is the bottleneck** — the change targets disks with ~30 ms flush
   latency, where a partition is capped near `1/fsync_latency` commit advances per second.
3. **Little throughput gain on fast disks**, where the win should instead show up as fewer `fsync`s.
4. **Follower batching capped at ~2×** by `maxAppendsPerFollower`; leader deduplication uncapped.

### Actual

`pd-standard` is not a slow disk. Freshly started it flushes in ~2.5 ms, exactly like `pd-ssd`. It
only becomes slow once *saturated*, and then queueing pushes flush latency to ~13 ms median and ~49 ms
p99. **High flush latency is a property of load, not of the disk you buy** — which gave us three
flush-latency tiers instead of the two we planned.

Each row is the mean of six consecutive 10-minute windows, `[min-max]` across them:

|          Tier          |  Arm  |   fsync p99   |      PI/s      |     fsync/s      | rec/fsync | dropped req/s |   gateway p99   | CPU cores |
|------------------------|-------|---------------|----------------|------------------|-----------|---------------|-----------------|-----------|
| `pd-ssd`               | main  | 5.0           | 299.9          | 3505 [3317-3591] | 4.9       | 0.8 [0-3]     | 152 [25-377]    | 6.0       |
| `pd-ssd`               | coal. | 7.1 [5-13]    | 298.3 [295-300]| 2117 [1707-2339] | 8.1       | 14.1 [0-47]   | 344 [69-699]    | 6.8       |
| regional `pd-standard` | main  | 18.8 [8-25]   | 267.5 [236-300]| 1749 [912-3261]  | 10.9      | 188 [0-352]   | 681 [50-952]    | 4.7       |
| regional `pd-standard` | coal. | 47.2 [10-86]  | 298.4 [294-300]| 1061 [342-1877]  | 25.7      | 17.8 [0-37]   | 396 [39-704]    | 5.2       |
| `pd-standard`, sat.    | main  | 48.9          | 113.6 [113-114]| 487 [486-488]    | 13.3      | 671 [668-673] | 2433            | 2.3       |
| `pd-standard`, sat.    | coal. | 238.1         | 148.5 [148-149]| 154 [153-154]    | 55.1      | 617 [615-620] | 2191 [2084-2305]| 2.7       |

**Expectation 1 holds everywhere.** 23–68% fewer `fsync`s, 1.6–4.1× more records packed into each one,
reproduced in every tier at every load level.

**Expectation 2 holds, and it is the largest effect.** On the saturated `pd-standard` tier — the only
one where flushing is unambiguously the bottleneck — throughput goes from **113.6 to 148.5 PI/s,
+31%**, on 68% fewer `fsync`s, with `[min-max]` ranges that do not overlap. The brokers there use 2.3
of 9 available cores: nothing is CPU-bound, the disk is the wall, and removing redundant `fsync`s moves
it. The regional tier points the same way with a smaller margin (267.5 → 298.4 PI/s, 10× fewer dropped
requests), though both arms sit at the saturation edge and the direct arm oscillates there.

**Expectation 3 holds.** On `pd-ssd` both arms sit at the offered 300 PI/s; the change buys headroom,
not throughput.

**Expectation 4 holds almost exactly.** Splitting `fsync` rate by Raft role on the saturated tier:

|   Role   | direct | coalesced |  reduction  |
|----------|--------|-----------|-------------|
| leader   | 55.2/s | 2.4/s     | **22.6×**   |
| follower | 53.5/s | 24.4/s    | **2.19×**   |

The follower lands within 10% of the theoretical ceiling of 2 set by `maxAppendsPerFollower`, while
the leader's deduplication is unbounded. Raising that constant is where the remaining follower-side win
lives.

#### The leader never coalesces

The leader's flush is `flushSync` — `flush(index).join()` — and all three callers of the flusher run on
the Raft thread. While the leader's `fsync` is in flight, the one thread that could enqueue another
request is parked, so at most one request is ever pending and group commit is structurally unreachable
there.

The leader's entire 22.6× reduction therefore comes from the **deduplication fast path** alone.
Deduplication is what the leader needs and costs nothing; the extra thread is what the follower needs
and is capped at ~2×. So the two halves are separable, and we tested that: three lines adding the same
check to the *default* direct flusher, coalescing left off. Age-matched arms, eight windows each, both
flat, 300 PI/s with zero drops:

|            Arm            |     `fsync`/s    | rec/`fsync` |    CPU cores     |
|---------------------------|------------------|-------------|------------------|
| PR, flag **off**          | 3485 [3446-3519] | 4.91        | 6.67 [6.53-6.77] |
| PR + dedup, flag **off**  | 2736 [2669-2767] | 6.25        | 6.68 [6.59-6.78] |

Deduplication alone removes **21.5%** of all `fsync`s at **identical CPU** — the CPU ranges overlap
almost completely. We predicted ~2858 `fsync`/s from the role split beforehand, so the mechanism model
holds.

#### Commit latency

On brokers with **6 CPUs** rather than 3, at 300 PI/s:

|    Arm    | commit p50 | commit p99 | commit mean | AppendEntries p99 | log append p99 |
|-----------|------------|------------|-------------|-------------------|----------------|
| `main`    | 4.82 ms    | 18.4 ms    | 5.39 ms     | 9.82 ms           | 4.96 ms        |
| dedup     | **3.68 ms**| **9.92 ms**| **4.55 ms** | 9.81 ms           | 4.95 ms        |
| coalesced | 4.80 ms    | 9.98 ms    | 5.19 ms     | 9.77 ms           | 4.96 ms        |

**Commit p99 halves**, and it is stable — three independent 5-minute windows gave `main`
19.03 / 18.15 / 17.89 against dedup's 9.89 / 9.92 / 9.94 — and it replicates on the unrelated 3-CPU
pair (40.98 → 24.13 ms, −41%).

Only commit latency moves; AppendEntries RTT and log append latency are flat. That is the
leader/follower asymmetry again: commit advance is the leader's path, where dedup lets most advances
skip their `fsync`, while AppendEntries RTT is gated on the *follower's* `fsync`, whose flush requests
are always for freshly appended records so the fast path rarely fires.

The p50/p99 split separates the two strategies. Dedup improves the median (4.82 → 3.68 ms) because the
flushes it elides cost nothing. Coalescing leaves the median alone (4.80 ms) because a needed flush
still blocks the leader, now behind a thread hand-off — it caps the tail only. Dedup gets both.

#### Under load, with CPU headroom

Three arms on 6-CPU brokers (`cpu-thread-count: 5`, `io-thread-count: 4`), offered load ramped by
scaling starter replicas, three 5-minute windows per level:

| rate |    arm    |  PI/s | `fsync`/s | rec/`fsync` | AppendEntries p50 | commit mean | CPU cores |
|------|-----------|-------|-----------|-------------|-------------------|-------------|-----------|
| 300  | `main`    | 300.0 | 3593      | 4.76        | 2.84 ms           | 5.39 ms     | 5.64      |
| 300  | dedup     | 300.0 | 3016      | 5.67        | 2.74 ms           | 4.55 ms     | 5.70      |
| 300  | coalesced | 300.0 | 2323      | 7.36        | 2.77 ms           | 5.19 ms     | 5.87      |
| 600  | `main`    | 600.0 | 3826      | 8.94        | 16.81 ms          | 30.93 ms    | 9.87      |
| 600  | dedup     | 600.3 | 3035      | 11.33       | 14.85 ms          | 13.35 ms    | 10.33     |
| 600  | coalesced | 600.2 | 2953      | 11.58       | **3.65 ms**       | **6.36 ms** | 10.57     |
| 900  | `main`    | 619.3 | 3623      | 9.74        | —                 | —           | 10.66     |
| 900  | dedup     | 611.3 | 2900      | 12.01       | —                 | —           | 11.39     |
| 900  | coalesced | 607.0 | 2655      | 13.04       | —                 | —           | 11.60     |

**The throughput ceiling does not move.** At 900 PI/s offered, all three saturate at ~615 PI/s
(619.3 / 611.3 / 607.0, `main` marginally highest) and shed ~1250 requests/s to backpressure at 11 of
18 cores with no exporter backlog. On `pd-ssd` the wall is flow control, not flushing.

**The latency win is large, and only appears under load.** At 600 PI/s coalescing gives **4.6× better
AppendEntries p50** (16.81 → 3.65 ms) and **4.9× better mean commit latency** (30.93 → 6.36 ms). At
300 PI/s the identical comparison shows nothing (2.84 vs 2.77 ms). The `fsync` column explains why:
`main`'s flush rate plateaus at ~3600–3800/s regardless of load, so once the offered rate outruns that,
appends queue behind flushes. Coalescing does the same work in 2953 flushes and never forms the queue.

#### What it costs

**`fsync` p99 inflates in every tier**, because each flush carries several times more data — 4.9× on
saturated `pd-standard` (49 → 238 ms), ~2.5× on regional (19 → 47 ms). You are trading many short
flushes for few long ones. That is inherent, and it matters if you care about the tail of an individual
flush rather than end-to-end latency.

**On CPU-starved brokers coalescing is a net loss.** On 3-CPU pods it cost 13–19% more CPU and roughly
3× worse gateway p99 at identical throughput. That is contention, not the flush path: profiling both
arms at identical work put the thread hand-off at under 4% of the CPU increase and showed GC *falling*,
with the increase spread diffusely and the coalesced arm CFS-throttled 3.3× more often. Give the same
comparison 6 CPUs and it vanishes — the three arms land within 1.7% of each other with identical
gateway p99. The extra flush thread per partition needs somewhere to run.

**Durability** we did not measure and cannot from these metrics. It holds by construction — commits
only advance and appends are only acknowledged once a covering `fsync` completed — and across every
coalesced cluster we saw zero flush failures and zero flush-induced step-downs.

### Conclusion

`flush-coalesced` does what it claims: same durability, 23–68% fewer `fsync`s. What it buys depends on
which resource you are short of:

* **Flush-latency-bound** (saturated `pd-standard`, `fsync` p99 ~49 ms): **+31% throughput**.
* **Fast disk, real load** (`pd-ssd` at 600 PI/s with CPU headroom): no extra throughput, **4.9× better
  commit latency**.
* **Fast disk, light load**: nothing either way.
* **CPU-starved broker**: harmful — 13–19% CPU and 3× gateway p99.

So "off by default" is right, but not because the feature is only for slow disks. It needs **CPU
headroom to be free** and only pays back **under load**. Enable it if flushing is your bottleneck or
your commit latency matters at high throughput; leave it off on tight pods or light workloads. And
measure `atomix_journal_flush_time_seconds` under *your* load rather than reasoning from the disk type
you bought — our "slow" `pd-standard` and "fast" `pd-ssd` were indistinguishable until the disk
saturated.

Two follow-ups the data points at directly:

1. **Add the deduplication fast path to the default direct flusher** — −21.5% `fsync`s at identical
   CPU, commit p99 halved, commit p50 down 24%, from three lines with no threads and no asynchrony. It
   is also the only variant that improves the median rather than just the tail.
2. **Revisit `maxAppendsPerFollower`** — after (1), follower-side coalescing is the only remaining
   benefit of the flag, and it is hard-capped at ~2×. Pure config, so measure before writing code.
