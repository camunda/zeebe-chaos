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

Metrics were captured over a steady-state window (≥4h after start) using PromQL against the cluster's central Prometheus.

The two workloads use **different variable payloads**, which matters for interpreting the prefix-filter configuration:

- the **realistic** payload has 15 named variables including three `customer`-prefixed ones (`customer`, `customerId`, `customer_claim_frequency`) alongside `disputeDetails`, `fraud_score_result`, and others — so the `customer` prefix filter keeps a meaningful subset.
- the **max** payload uses generic names (`var1`…`var14`, `businessKey`) with no `customer`-prefixed variables — so the `customer` prefix filter matches nothing and behaves identically to disabling variable export entirely. The prefix-filter result is therefore **not** comparable across the two workloads, and at max load the prefix-filter cluster is effectively a second "exporter variable off" data point.

### Validating the configuration at the data level

Before trusting the resource numbers, we confirmed each configuration actually does what it claims — not just that the Helm values rendered correctly, but that the data landing in Elasticsearch reflects the filter. A terms aggregation on the Zeebe variable record index shows which variable names reached ES:

```bash
curl -s -H 'Content-Type: application/json' \
  'localhost:9200/zeebe-record_variable*/_search' -d '{
    "size": 0,
    "track_total_hits": true,
    "aggs": { "names": { "terms": { "field": "value.name", "size": 50 } } }
  }'
```

Running this against each realistic cluster confirms the filtering (counts for one non-`customer` variable, `disputeDetails`, and one `customer`-prefixed variable, `customer`):

| Configuration | Total variable docs | `disputeDetails` | `customer` |
|---|---|---|---|
| Baseline | 11.4M | 16,001 | 1,615,967 |
| Optimize mode | 11.5M | 16,114 | 1,627,390 |
| Prefix filter (`customer`) | 1.67M | 0 | 1,637,615 |
| Exporter variable off | 0 | 0 | 0 |
| Exporter off + importer off | 0 | 0 | 0 |
| Importer off | 11.5M | 16,164 | 1,632,452 |

This confirms three things at the data level:

- **Exporter variable off** and **exporter off + importer off** write no variable records at all.
- The **prefix filter** drops every non-`customer` variable (`disputeDetails` → 0) while keeping the `customer`-prefixed ones — cutting variable documents by ~85% for this payload.
- **Importer off** leaves the full variable stream in Elasticsearch (it only stops Optimize from reading it), and **Optimize mode** does not filter variable export at the exporter at all. Both keep the same variable documents as baseline in the Zeebe index; their effect shows up instead in Optimize's own imported indices, which we look at next.

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

##### Where the disk actually goes: the Optimize index

The dominant consumer is not the Zeebe export but **Optimize's own imported indices**. Optimize stores process-instance data — including each instance's variables as nested documents — in `optimize-process-instance-*` indices. Measuring those directly per cluster shows where variable filtering pays off:

```bash
curl -s 'localhost:9200/_cat/indices/optimize-process-instance-*?h=index,docs.count,store.size&bytes=b'
```

| Configuration | Optimize PI index | Process-instance docs |
|---|---|---|
| Baseline | 56.3 GiB | 43.4M |
| Optimize mode | 61.7 GiB | 43.7M |
| Prefix filter (`customer`) | 17.4 GiB | 20.8M |
| Importer off | 1.9 GiB | 10.4M |
| Exporter variable off | 1.9 GiB | 10.4M |
| Exporter off + importer off | 1.9 GiB | 10.4M |

The doc count tells the story: without variable import the index holds ~10.4M documents (instances and flow nodes); baseline holds ~43.4M. The extra ~33M documents — and ~54 GiB — are imported **variables**. Consequently:

- **Disabling variable import or export collapses the Optimize index to ~3% of baseline** (56 GiB → 1.9 GiB), and it makes no difference whether you stop the export (`index.variable=false`) or just the import (`VARIABLE_IMPORT_ENABLED=false`) — both leave Optimize with the same variable-free index. The Zeebe-export side differs (the export-off configs also drop the Zeebe variable records measured above), but the Optimize index — the bulk of the cost — is identical.
- The **prefix filter** lands in between (~17 GiB), proportional to how much of the payload matches the prefix.
- **Optimize mode does not reduce the Optimize index** — here it was slightly larger than baseline, so `optimizeModeEnabled` is not a storage-reduction lever for variables.

{{Confirm these figures against the steady-state (8h) snapshot — the numbers above are a ~4.5h preview and are still growing.}}

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
