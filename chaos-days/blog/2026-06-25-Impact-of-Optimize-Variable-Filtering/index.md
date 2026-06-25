---
layout: posts
title:  "Reducing Optimize's Elasticsearch Overhead: Variable Filtering Strategies Compared"
date:   2026-06-25
categories:
  - chaos_experiment
  - bpmn
tags:
  - performance
  - optimize
  - elasticsearch
authors:
  - zell
---

<!--
DRAFT SKELETON — data-independent prose is written; every {{placeholder}} is filled
from the 8h steady-state measurement (/tmp/optimize-metrics-rerun-8h.md).
Charts (real-*.png / max-*.png) are captured from Grafana after the 8h check.
Update `date` to the publish date before merging.
-->

# Chaos Day Summary

In a [previous Chaos Day](https://camunda.github.io/zeebe-chaos/2026/06/10/Impact-of-Optimize-on-Camunda/) we measured what Optimize costs a cluster: at a realistic workload it drove **3.4x higher Elasticsearch CPU** and **~4x more ES disk** than running without it, even with throughput and backpressure unaffected. That post ended with an open question — *can variable handling be tuned to reduce the impact?* This Chaos Day answers it.

We ran **twelve** load tests on Camunda 8.9.9 — six variable-filtering configurations, each at both a **realistic** and a **max** workload — and compared throughput, latency, CPU, memory, and ES disk across all of them. All twelve ran in parallel on identical infrastructure and the same Helm chart, started together so their Elasticsearch footprints are directly comparable.

**TL;DR;** {{headline: the most effective lever and by how much — e.g. "Disabling Optimize's variable import cuts ES disk to ~X% of baseline and ES CPU to ~Y%, with no throughput cost at realistic load." Then the surprising finding and the one practical recommendation.}}

<!--truncate-->

## Chaos Experiment

All twelve clusters ran in parallel on the same benchmark infrastructure with `orchestration-tag=8.9.9`, the same `camunda-platform-8.9` Helm chart (pinned to a single chart revision so index-replica settings are identical), and Optimize enabled everywhere. Each cluster was started fresh with an empty Elasticsearch so disk figures reflect only this run's accumulation, and all were started together so equal-age ES disk comparisons are valid.

### Configurations tested

Each configuration changes only how process **variables** are exported to Elasticsearch and/or imported by Optimize:

| Name | What it changes | Key setting |
|---|---|---|
| Baseline | Default — all variables exported and imported | _(none)_ |
| Importer off | Optimize skips importing variables (still exported to ES) | `CAMUNDA_OPTIMIZE_ZEEBE_VARIABLE_IMPORT_ENABLED=false` |
| Prefix filter | Only variables named `customer*` are exported | `zeebe.broker.exporters.elasticsearch.args.index.variableNameInclusionStartWith[0]=customer` |
| Exporter variable off | No variable records exported at all | `zeebe.broker.exporters.elasticsearch.args.index.variable=false` |
| Exporter off + importer off | Both of the above (belt-and-suspenders) | `index.variable=false` + `VARIABLE_IMPORT_ENABLED=false` |
| Optimize mode | Exporter writes only what Optimize needs | `zeebe.broker.exporters.elasticsearch.args.index.optimizeModeEnabled=true` |

Each configuration was run at two workloads, mirroring the previous post:
- **realistic** — a complex process model at a sustainable production rate (~1 PI/s), each instance with multiple tasks, sub-processes, and variables; representative of real customer workloads.
- **max** — driven at 300 PI/s to push the engine to its throughput ceiling and surface backpressure.

Metrics were captured over a steady-state window (≥4h after start) using PromQL against the cluster's central Prometheus. The realistic payload {{contains / does not contain}} `customer`-prefixed variables — relevant to interpreting the prefix-filter result.

### Expected

We expected the configurations that remove variable data from Elasticsearch to reduce ES disk and CPU, with the deepest cuts where variables never reach ES at all. The open questions: how much does each lever actually save, whether disabling only the Optimize *importer* (leaving variables in ES) saves storage at all, and whether any of this recovers throughput at max load.

### Actual

#### Realistic workload: Resource consumption

*This is the primary story — at a realistic workload throughput is not constrained, so the differences land entirely in Elasticsearch resources.*

![CPU — realistic scenario](real-cpu.png)

| Metric (cores) | Baseline | Importer off | Prefix filter | Exporter var off | Exp off + imp off | Optimize mode |
|---|---|---|---|---|---|---|
| ES CPU | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| Camunda CPU | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |

{{Prose: ES CPU spread from baseline to the cheapest config; note Camunda broker CPU stays flat.}}

![Disk — realistic scenario](real-disk.png)

| Metric | Baseline | Importer off | Prefix filter | Exporter var off | Exp off + imp off | Optimize mode |
|---|---|---|---|---|---|---|
| ES index primary (GiB) | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| ES disk used (GiB) | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |

{{Prose: the disk ranking. Key question answered here — does "importer off" (variables still exported) save disk, or does the saving only come from stopping the export? This is where the surprising finding, if any, lands.}}

![General overview — realistic scenario](real-general.png)

Memory (ES and total) was {{flat / …}} across all six configurations, so it is not a tuning lever.

#### Realistic workload: Throughput and latency

At a realistic workload all six configurations held ~1 PI/s with zero backpressure — variable filtering does not change correctness or latency here.

| Metric | Baseline | Importer off | Prefix filter | Exporter var off | Exp off + imp off | Optimize mode |
|---|---|---|---|---|---|---|
| Completed PI/s | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| Backpressure | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| Optimize variable import time (s) | {{}} | — | {{}} | — | — | {{}} |

#### Max workload: Throughput and backpressure

*At 300 PI/s the clusters are throughput-constrained, so here the question is which configuration sustains the most completed PI/s by freeing Elasticsearch write capacity.*

![General overview — max scenario](max-general.png)
![Throughput comparison — max scenario](max-throughput.png)

| Metric | Baseline | Importer off | Prefix filter | Exporter var off | Exp off + imp off | Optimize mode |
|---|---|---|---|---|---|---|
| Completed PI/s | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| Backpressure | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |

{{Prose: which config recovers the most throughput vs baseline, linking back to the -22% finding from the previous post.}}

#### Max workload: CPU and disk

![CPU — max scenario](max-cpu.png)
![Disk — max scenario](max-disk.png)

| Metric | Baseline | Importer off | Prefix filter | Exporter var off | Exp off + imp off | Optimize mode |
|---|---|---|---|---|---|---|
| ES CPU (cores) | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |
| ES disk used (GiB) | {{}} | {{}} | {{}} | {{}} | {{}} | {{}} |

{{Prose: at max load ES is the bottleneck; note whether the CPU/disk picture matches realistic or differs.}}

## Configuration Guide

For each lever: what it controls, the measured saving, and when to use it.

- **Importer off** (`CAMUNDA_OPTIMIZE_ZEEBE_VARIABLE_IMPORT_ENABLED=false`) — Optimize stops pulling variable records into its own indices; variables remain in the Zeebe export stream in Elasticsearch. ES effect: {{CPU X%, disk Y% of baseline}}. Trade-off: Optimize reports lose variable-level data. Use when you run Optimize but don't need variable analytics.
- **Exporter variable off** (`index.variable=false`) — variables are never written to Elasticsearch. ES effect: {{}}. Trade-off: variables are gone from ES entirely — Operate/Tasklist/Optimize cannot show them and cannot recover them retroactively. Use only when no component needs variable data.
- **Prefix filter** (`variableNameInclusionStartWith`) — only variables whose names match a prefix are exported. ES effect: {{scales with match rate}}. Use when a known, bounded set of variables (e.g. `customer*`) carries the analytics value and the rest is noise.
- **Exporter off + importer off** — belt-and-suspenders. ES effect: {{should match exporter-var-off}}. Use {{when…}}.
- **Optimize mode** (`optimizeModeEnabled=true`) — the exporter writes a reduced set tailored to Optimize. ES effect: {{}}. Use when Optimize is required and {{full variable data is / isn't needed}}.

### What We Learned

- {{key finding 1 — the headline disk/CPU lever}}
- {{key finding 2 — does importer-off alone save storage?}}
- {{key finding 3 — prefix filter behaviour}}
- {{key finding 4 — max-load throughput recovery}}
- Camunda broker CPU and memory are {{not}} meaningfully affected — variable filtering is an Elasticsearch-side lever.

### Possible Improvements / Recommendations

- {{recommendation prioritised by impact}}
- Update the [sizing guidance](https://docs.camunda.io/docs/next/components/best-practices/architecture/sizing-your-environment/) with these concrete variable-filtering numbers (camunda-docs#9118).
- {{…}}
