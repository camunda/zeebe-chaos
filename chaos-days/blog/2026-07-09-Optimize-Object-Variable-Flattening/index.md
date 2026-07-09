---
layout: posts
title:  "Confirming Optimize's Object Variable Flattening Cost With a Controlled A/B Test"
date:   2026-07-09
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
Charts: screenshots from the `chvrsl2` "Camunda Performance - Optimize investigation" dashboard
(dashboard.benchmark.camunda.cloud), both namespaces:
- Panel 81 "Data size in secondary Storage" (piechart) — Optimize/Zeebe/Camunda disk split, both configs
- Panels 85/86/88 (root PI / PI / variables created) alongside panel 81, to show the sizing-per-unit-of-work numbers
Raw numbers for all of these are in the "Actual" and "Cross-validating with Grafana" sections below.
-->

# Chaos Day Summary

In a [previous Chaos Day](https://camunda.github.io/zeebe-chaos/2026/06/10/Impact-of-Optimize-on-Camunda/) and its [variable-filtering follow-up](https://camunda.github.io/zeebe-chaos/2026/06/25/Impact-of-Optimize-Variable-Filtering/), we measured Optimize's Elasticsearch overhead against Self-Managed load tests. Running the same kind of test against a Camunda SaaS cluster turned up something we didn't expect: Optimize's disk footprint there looks nothing like what we'd measured on Self-Managed. This Chaos Day tracks that discovery down to its root cause and confirms it with a controlled experiment.

**TL;DR;** A week-long test against a SaaS Advanced 4x cluster showed Optimize's indices taking up only ~7-10% of total Elasticsearch disk, versus ~59-100% on our Self-Managed weekly load test running the exact same workload. The cause: Optimize's `includeObjectVariableValue` flag (env `CAMUNDA_OPTIMIZE_ZEEBE_INCLUDE_OBJECT_VARIABLE`) defaults to `true` and **flattens every JSON object variable into one stored variable per property, plus the raw serialized object itself**. Camunda SaaS explicitly disables this; the public Self-Managed Helm chart does not, so any Self-Managed deployment that hasn't touched this setting silently pays for it. We confirmed this is the *entire* explanation, not just a correlation, with an isolated A/B test that changes only this one flag: **Optimize's ES disk share dropped from 62.8% to 7.6%, an 8.3x reduction**, for the same workload. The number that matters most for capacity planning: **total secondary storage per root process instance dropped from 5.85 MB to 2.62 MB, a 2.24x reduction** — that's the actual "how much more disk do I need to buy" answer, net of the Zeebe/Camunda storage this flag doesn't touch. We're disabling it in our own load tests, and there's an open discussion about changing Optimize's shipped default to match SaaS.

<!--truncate-->

## Chaos Experiment

### How we got here

While setting up a SaaS test comparable to our Self-Managed weekly load test (Advanced 4x, closest match to our Self-Managed hardware), we expected similar behavior to what we'd already measured: Optimize being the dominant Elasticsearch disk consumer, eventually filling the disk without a tighter ILM policy than SaaS's defaults (30 days for Operate/Tasklist, 180 for Optimize, vs. our load test's 1-3 days).

What we found instead, after a week:

| | SaaS (Advanced 4x) | Self-Managed (weekly load test) |
|---|---|---|
| Optimize's share of total ES disk | ~7-10% | ~59-100% |

Same realistic workload (~1 root PI/s, 50 sub-process instances per root), same process definitions, wildly different Optimize disk footprint. The batch size and page-fetch metrics also differed between the two, hinting at a configuration gap somewhere, but nothing obviously explained a footprint difference this large.

### Finding the flag

During investigation we detected the object variable handling. Our `realisticPayload.json` load-test payload includes a `customer` variable that's a JSON object (five string fields: `firstname`, `lastname`, `email`, `phone`, `address`) and a `disputeDetails` variable, also an object. Optimize's [object variable flattening](https://docs.camunda.io/docs/next/self-managed/components/optimize/configuration/object-variables/) feature, controlled by `includeObjectVariableValue`, turns each object variable into one stored Optimize variable per property, plus the raw serialized value. That's a plausible source of a large, silent multiplier.

Checking the code confirmed it:
- Optimize's own shipped default ([`service-config.yaml`](https://github.com/camunda/camunda/blob/main/optimize/util/optimize-commons/src/main/resources/service-config.yaml)) is `true`.
- In our SaaS environment this is explicitly overriden to `false`, a deliberate scalability decision made for C8 SaaS, apparently inherited from a feature originally built for Camunda 7.
- The public Self-Managed Helm chart sets no equivalent override, so it silently inherits `true`, including our own load tests, which never touched this setting either.

A first live comparison (SaaS cluster vs. a Self-Managed weekly load test cluster running the identical scenario) measured the impact directly:

| Process | Metric | Self-Managed | SaaS | Ratio |
|---|---|---|---|---|
| `bankDisputeHandling` | variables / instance | 1,221 | 208 | 5.9x |
| `bankDisputeHandling` | variable value bytes / instance | 59,174 | 1,880 | **31.5x** |
| `refundingProcess` | variables / instance | 15 | 2 | 7.5x |
| `refundingProcess` | variable value bytes / instance | 635 | 13 | **48.8x** |

Strong evidence, but not yet proof: the Self-Managed and SaaS environments differ in more than just this one flag (hardware, ILM/retention policy, exporter batch config). We opened [camunda/camunda#57127](https://github.com/camunda/camunda/issues/57127) to track changing Optimize's shipped default, and a [load-test PR](https://github.com/camunda/camunda/pull/57190) to stop our own load tests from silently paying this cost, but wanted a cleaner experiment before calling the root cause confirmed.

### Expected

If object variable flattening is really the *entire* explanation, then toggling only that one flag (everything else held identical) should reproduce the same magnitude of difference we saw between the very differently-configured SaaS and Self-Managed environments.

### Actual: the controlled A/B test

We deployed two namespaces on the `realistic` scenario, identical except for one environment variable:

| | `default-flatten-obj` | `no-flatten-obj` |
|---|---|---|
| `CAMUNDA_OPTIMIZE_ZEEBE_INCLUDE_OBJECT_VARIABLE` | unset (Optimize's shipped default, `true`) | `false` (matches SaaS) |

![general-overview](general-overview.png)

Same `realistic` scenario, same 1 root-PI/s rate (confirmed via `zeebe_process_instance_creations_total`), same `historyCleanup` config (`ttl=P1D`, cleanup enabled), same partition count, same age (~3h) at measurement time.

![disk-consumption](disk-consumption.png)


Based on the disk consumption we can see that with the default behavior of flattening object variables, Optimize's share of total ES disk is ~65%, while with flattening disabled it drops to ~7.6%. The per-instance variable counts and value bytes also match the earlier SaaS-vs-Self-Managed ratios almost exactly. The data was taken from Elasticsearch directly.

**Result:**

| Metric | flatten=`true` | flatten=`false` | Ratio |
|---|---|---|---|
| Optimize's share of total ES disk | 62.8% | 7.6% | **8.3x** |
| `bankDisputeHandling` index size (per instance) | 3.07 MB | 145 KB | ~21x |
| `refundingProcess` index size (per instance) | 25.1 KB | 1.18 KB | ~21x |
| `bankDisputeHandling` sampled instance: vars / value bytes | 1,222 / 59,144 | 208 / 1,828 | 5.9x / 32.4x |
| `refundingProcess` sampled instance: vars / value bytes | 15 / 629 | 2 / 13 | 7.5x / 48.4x |

The per-instance ratios are nearly identical to the earlier SaaS-vs-Self-Managed numbers (5.9x/31.5x and 7.5x/48.8x there, vs. 5.9x/32.4x and 7.5x/48.4x here) despite this test controlling away every other difference between those two environments. That upgrades the finding from "strongly correlated" to **causally confirmed**: this one flag, in isolation, fully explains the SaaS-vs-Self-Managed Optimize disk gap.

Total Elasticsearch disk usage over the ~3h test window climbed at ~1.52%/hour with flattening on, vs. ~0.58%/hour with it off (~2.6x — smaller than the 8.3x Optimize-share figure because this measure includes non-Optimize indices, which are identical between the two and dilute the ratio).

### Cross-validating with live Grafana metrics

We also added four new panels to our internal ["Camunda Performance - Optimize investigation" dashboard](https://dashboard.benchmark.camunda.cloud/d/chvrsl2/camunda-performance-optimize-investigation): root process instances created, (child) process instances created, service tasks created, and variables created — each an `increase()` over the dashboard's time range on the relevant Zeebe metric. Worth checking whether these agree with what we measured directly in Elasticsearch, since if they do, they replace a manual `kubectl port-forward` + `curl` audit with a live, reusable dashboard.

| Panel | Prometheus (window matching each namespace's age) | Elasticsearch | Agreement |
|---|---|---|---|
| Root PI created | 23,457 | 22,130 (`bankDisputeHandling` top-level count) | Real ~6% gap — see below |
| PI created (call-activity spawns) | 1,096,185 | 1,096,855 (`refundingProcess` top-level count) | Matches, within elapsed-time noise |
| Service tasks created | 2,224,784 | — | Query bug, see below |
| Variables created | 14,578,993 | 14,639,146 (`intent=CREATED` only, via a terms aggregation on `zeebe-record_variable`) | Matches almost exactly |

Three of four check out well. Two things worth calling out:

- **The root-PI gap is real, not noise, and it's a finding in its own right.** Prometheus counts instances the moment Zeebe creates them; Elasticsearch counts instances Optimize has actually imported. Those track closely for `refundingProcess`, but `bankDisputeHandling` — the process with the far heavier per-instance document (1,358 nested Lucene docs/instance with flattening on, vs. 6-17 for `refundingProcess`) — runs a persistent ~1,300-instance import backlog. Object variable flattening doesn't just cost disk; it visibly slows Optimize's import for the processes it hits hardest.
- **The "Service tasks created" panel has a bug**: its query hardcodes `[24h]` instead of `[$__range]`, so it ignores the dashboard's time-range picker entirely. Not yet fixed.

The dashboard also already had a panel doing the exact disk-share breakdown we'd been computing by hand: "Data size in secondary Storage" splits primary ES disk into Optimize/Zeebe-export/Camunda-exporter shares. It matched our manual `_cat/indices` calculation within ~1% (61.5% vs. 62.8%).

Combining the creation-count panels with that disk breakdown gives population-wide bytes-per-unit numbers, computed from the *entire* cluster's history rather than one sampled instance:

- **Optimize bytes / root PI created:** 1.74 MB (flatten on) vs. 91.7 KB (flatten off) → **19.4x**
- **Optimize bytes / variable created:** 2,924 B (flatten on) vs. 147.6 B (flatten off) → **19.8x**

Both land in the same range as the single-sampled-instance ~21x ratio measured directly in Elasticsearch above — a useful cross-check, since these two methods (population-wide Prometheus counters vs. one sampled ES document) are independent of each other.

#### The number that actually matters for sizing

Optimize's disk share explains *why* the gap exists, but it's not the number to size a cluster against — it doesn't net out the storage that this flag never touches (Zeebe's raw export, the Camunda Exporter's Operate/Tasklist indices). The number that does:

```promql
sum(kubelet_volume_stats_used_bytes{namespace=~"$namespace", persistentvolumeclaim=~"elastic.*"})
/
(sum(increase(zeebe_element_instance_events_total{namespace=~"$namespace", action="activated", type="PROCESS"}[$__range]))
 - sum(increase(zeebe_element_instance_events_total{namespace=~"$namespace", action="activated", type="CALL_ACTIVITY"}[$__range])))
```

Total *actual on-disk* Elasticsearch bytes (via `kubelet_volume_stats_used_bytes`, which includes the replica — cross-checked against `elasticsearch_indices_store_size_bytes_primary` × 2, agreeing within ~2%), divided by root process instances created:

| | flatten=`true` | flatten=`false` | Ratio |
|---|---|---|---|
| Total secondary storage / root PI | **5.85 MB** | **2.62 MB** | **2.24x** |
| Total PVC bytes used | 136.1 GiB | 59.0 GiB | 2.31x |

This is the direct answer to "how much more disk do I need to provision for the same workload" — smaller than the 8.3-19.8x Optimize-specific ratios because it's diluted by the fixed Zeebe/Camunda baseline, but it's the number that actually drives a capacity-planning decision. We sanity-checked it against the component breakdown above: Optimize + Zeebe-export + Camunda-exporter per root PI sums to 5.93 MB (flatten on) and 2.72 MB (flatten off), both within rounding of the directly-measured figures.

### A closed-form formula for the variable-count multiplier

Pulling the raw `variables[]` array from a sampled `refundingProcess` document in each namespace (the simpler of our two processes — one service task, no nested sub-processes) let us go one step further than an empirical ratio.

With flattening **on**, the document has 15 variables:
```
disputePosition.name, disputePosition.transactionDate, disputePosition._id, disputePosition,
customer.lastname, loopCounter, disputeId, customer.address, disputePosition.amount, customer,
disputePosition.currency, customer.email, disputePosition.index, customer.firstname, customer.phone
```
With flattening **off**, it has 2:
```
disputeId, loopCounter
```

The extra variables aren't job-completion noise — the `refunding` worker completes with an empty payload. They come from the BPMN model: the call activity invoking `refundingProcess` is a multi-instance construct over `disputeDetails.disputePositions`, with `propagateAllChildVariables="false"` and `propagateAllParentVariables="false"`, and only two explicit input mappings (`customer`, `disputeId`). Even so, the multi-instance loop's own local variables — `disputePosition` (the loop item, itself a 6-field object) and `loopCounter` — are visible in the child process instance's scope. Those propagation flags only govern parent↔child *output* propagation, not the loop's local scope; it's an easy thing to miss if you only read the explicit `zeebe:input` mappings.

So `refundingProcess`'s real variable set is two primitives (`disputeId`, `loopCounter`) and two object variables (`customer`, 5 fields; `disputePosition`, 6 fields). That gives a formula that matches both documents exactly:

```
StoredVariables(flatten=false) = P                      (object variables dropped entirely — not even stored unflattened)
StoredVariables(flatten=true)  = P + O + ΣF_i            (each object variable → 1 raw + F_i child-field variables)
```

where `P` = primitive variable count, `O` = object/JSON variable count, `F_i` = field count of object variable `i`.

For `refundingProcess`: `P=2`, `O=2`, `ΣF = 5 + 6 = 11`.
- flatten=`true`: `2 + 2 + 11 = 15` — matches the live document exactly.
- flatten=`false`: `2` — matches exactly.
- Ratio: `15 / 2 = 7.5x` — matches the measured ratio exactly.

Combined with the per-value storage overhead established in the [variable-filtering post](https://camunda.github.io/zeebe-chaos/2026/06/25/Impact-of-Optimize-Variable-Filtering/) (nested-doc indexing + up to 6 secondary representations per stored value, empirically ~5-11x depending on field type), this gives a two-layer sizing model:

```
Total disk multiplier ≈ A_flatten × A_per_value
A_flatten = (P + O + ΣF_i) / P
```

The useful part: `A_flatten` is computable directly from a process's BPMN model and payload schema — no load test required — as long as object variables reachable through multi-instance loop scope are counted, not just the ones in explicit `zeebe:input` mappings.

### Validating the formula against the bigger process

`refundingProcess` is the simple case: one service task, no nesting. `bankDisputeHandling` is far more complex (24 unique flow node ids, nested sub-processes, its own multi-instance constructs), and reconciling it exactly needed two additions the simple case didn't exercise. Pulling the exact variable names (not just counts) from both namespaces' sampled documents:

| Variable | Where it's set | Occurrences (N) | Type | Fields (F) | Per-occurrence count | Total stored (flatten=`true`) | Contributes to P (flatten=`false`)? |
|---|---|---|---|---|---|---|---|
| `loopCounter` | MI loop counter, 2 constructs × 50 iterations | 100 | primitive | — | 1 | 100 | yes (100) |
| `correlationKey` | "Vendor fraud claim validation" (MI, ×50) + "Document Request Process" (×1) | 51 | primitive | — | 1 | 51 | yes (51) |
| `disputeId` | call activity input mapping, per iteration | 50 | primitive | — | 1 | 50 | yes (50) |
| `type` | send-task local input, path-dependent | 2 | primitive | — | 1 | 2 | yes (2) |
| `vendor_claim_frequency` | fraud subprocess output | 1 | primitive | — | 1 | 1 | yes (1) |
| `isRefund` | DMN/gateway output | 1 | primitive | — | 1 | 1 | yes (1) |
| `isHighFraudRatingConfidence` | DMN/gateway output | 1 | primitive | — | 1 | 1 | yes (1) |
| `customerId` | root start variable | 1 | primitive | — | 1 | 1 | yes (1) |
| `customer_claim_frequency` | fraud subprocess output | 1 | primitive | — | 1 | 1 | yes (1) |
| **Primitives subtotal** | | | | | | **208** | **P = 208** |
| `customer` | root (×1) + call-activity input mapping per iteration (×50) | 51 | object | 5 | 1+5=6 | 306 | no (dropped) |
| `disputePosition` | MI loop item, 2 constructs × 50 iterations | 100 | object | 6 | 1+6=7 | 700 | no (dropped) |
| `disputeDetails` family | root object: raw + `disputePositions._listSize` + `disputeId` + `disputeAmount.{amount,currency}` + `disputeStartDate` | 1 | object (nested + 1 list field) | — | 6 | 6 | no (dropped) |
| `fraud_score_result` | top-level list variable: raw + `_listSize` | 1 | list | — | 2 | 2 | no (dropped) |
| **Total** | | | | | | **1222** | **208** |

`1222 / 208 = 5.875 ≈ 5.9x` — matches the measured ratio exactly.

The `6` and `7` per-occurrence figures are `1 + F`: `customer` has 5 fields (`firstname`/`lastname`/`email`/`phone`/`address`) → `1+5=6`; `disputePosition` has 6 fields (`_id`/`index`/`name`/`amount`/`currency`/`transactionDate`) → `1+6=7`. The two additions this process required:

1. **Scope repetition.** A variable set inside a multi-instance loop occurs once *per iteration*, not once. This process has two independent 50-iteration multi-instance constructs both looping over `disputeDetails.disputePositions` (an embedded sub-process and the call activity spawning `refundingProcess`), so `loopCounter` and `disputePosition` each occur 100 times (50+50). Generalized, the formula sums over every variable-defining *scope* `s`, weighted by how many times that scope executes (`n_s`):
   ```
   StoredVariables(flatten=false) = Σ_s n_s × P_s
   StoredVariables(flatten=true)  = Σ_s n_s × (P_s + O_s + ΣF_i,s)
   ```
2. **`F_i` is a recursive leaf count, and flattening has no depth limit.** `disputeDetails` looked like it didn't fit `1+F` — a mix of a list field, a nested object, and two plain fields — until reading `ObjectVariableService.flattenJsonObjectVariableAndAddToResult` ([source](https://github.com/camunda/camunda/blob/main/optimize/backend/src/main/java/io/camunda/optimize/service/importing/engine/service/ObjectVariableService.java#L156-L169)): it delegates to a generic `JsonFlattener` (`FlattenMode.KEEP_ARRAYS`) that recurses through nested JSON to arbitrary depth, emitting one entry per *leaf* — a primitive, or an array (arrays are never expanded element-by-element; any array, at any depth, collapses into a single `_listSize` marker instead). `flattenAsMap()` only ever emits leaves, never intermediate object nodes — which is why the 2-level-nested `disputeDetails.disputeAmount` recurses straight to `disputeDetails.disputeAmount.{amount,currency}` with no separate entry for `disputeAmount` itself. Redefine `F_i` as "recursive leaf count, where an array at any depth counts as one leaf" and `disputeDetails` fits perfectly: `disputePositions` (1) + `disputeId` (1) + `disputeAmount.{amount,currency}` (2) + `disputeStartDate` (1) = 5 leaves, `1+5=6`.

   **That also means there's no ceiling on how expensive one object variable can get.** Optimize's flattening cost isn't bounded by any config on Optimize's side — it's determined entirely by the shape of whatever JSON the process happens to pass in. A deeply nested object with several fields at each level multiplies out combinatorially (depth × branching factor), with nothing in this code path to stop it. Arrays are the one shape that doesn't compound this way — a `_listSize` marker costs the same one entry whether the array has 5 elements or 5,000 — but a payload built from deeply nested plain objects, with no arrays at all, has no equivalent protection. The customer's payload shape, not anything Camunda controls server-side, determines the worst case here.

### Why `A_per_value` resists a code-only answer

It's tempting to read `A_per_value` straight off the code: `addValueMultifields` creates up to 6 field mappings per stored variable (exact keyword, lowercase keyword, n-gram text, best-effort date/long/double). But that's a *mapping count*, not a *byte multiplier* — the 6 mappings don't cost equally. A numeric value's date/long/double attempts succeed cheaply; a long string's n-gram field emits roughly 10x its length in tokens, which is the dominant cost. Elasticsearch/Lucene also compresses repeated values, so the real byte cost depends on the indexed *data* (length, cardinality) as much as the mapping count — information the code alone doesn't give you.

We tried to shortcut this with data we already had, by comparing the raw `zeebe-record_variable` index against Optimize's index in the same two clusters. That doesn't give a clean number either: `zeebe-record_variable` is an **append-only log of every variable update** over the test's lifetime, while Optimize's variable array is a **snapshot** of the latest value per instance — a whole-index comparison conflates "how many updates happened" with "cost per stored value." For what it's worth, both namespaces' `zeebe-record_variable` indices were nearly identical in size (~7.7-7.9M docs, ~1.1-1.2GB primary each), confirming the flag doesn't touch the raw exporter — useful as a sanity check, but not for deriving `A_per_value`.

What would actually work: a dedicated benchmark with exactly one process, one variable, no other noise, run once per type (string of known length, number, boolean, date) — isolating the byte delta to purely the storage mechanism for that type, the same way this post's A/B test isolated `A_flatten` from a range into an exact number. Not yet run.

## What We Learned

- **A correlation across two differently-configured environments can look identical to full causation, and it's worth checking.** The SaaS-vs-Self-Managed comparison was already compelling (5.9x-7.5x variable count, 31.5x-48.8x bytes), but those two environments differ in hardware, retention policy, and exporter config too. The isolated A/B test reproduced the same numbers almost exactly while controlling all of that away — the stronger and cheaper experiment to run when you can.
- **Optimize's object variable flattening, not cardinality, drove our earlier "~29x" figure.** We'd previously attributed a large Optimize storage multiplier to high-cardinality string variables; re-checking the actual benchmark payload showed the variables involved are constants repeated across every instance. The real driver is the same flattening mechanism confirmed here.
- **Variables can reach a process instance's scope through paths you won't find in explicit input mappings.** The multi-instance loop item variable reaching the child process despite both propagation flags being `false` is exactly this kind of thing — worth remembering the next time a variable count doesn't match what the io mappings alone would predict.
- **SaaS already runs with this disabled; Self-Managed customers who haven't touched this setting are silently paying for it**, and our own load tests were one of them until now.
- **Object variable flattening has no depth limit, which makes it a genuinely open-ended cost, not just a fixed multiplier.** It's not "objects cost ~6x more" — it recurses through arbitrarily nested JSON, so cost scales with the *payload's own shape* (depth × branching factor), something Camunda's server-side config has no say over. Arrays are the exception (they collapse to one marker regardless of length), but a deeply nested object with no arrays at all has nothing to cap it. That makes this a sizing risk that's hard to bound in advance for any given customer's process design, not just a config knob to flip.
- **The Optimize-specific ratios (8.3-19.8x) explain the mechanism; the total-disk-per-root-PI ratio (2.24x) is the number to size against.** Diagnosing *why* is different from sizing *how much* — the second question needs the denominator netted against everything the flag doesn't touch.
- **A dashboard's "increase over this Zeebe metric" panels can replace a lot of manual `kubectl port-forward` + `curl` auditing, but they're not a free substitute for reading ES directly.** They matched well here, and even surfaced a real finding we wouldn't have otherwise seen this clearly (Optimize's import lag on the heavier process) — but only because we cross-checked them against ES first and caught one panel with a query bug the same way.

### Possible Improvements / Recommendations

- Change Optimize's shipped default for `includeObjectVariableValue` to `false`, matching what SaaS already runs at scale: [camunda/camunda#57127](https://github.com/camunda/camunda/issues/57127).
- Disable object variable flattening in our own load tests by default: [camunda/camunda#57190](https://github.com/camunda/camunda/pull/57190).
- Sizing guide already updated with this mechanism and the controlled measurement: [camunda-docs#9326](https://github.com/camunda/camunda-docs/pull/9326).
- Extend `A_per_value` from an empirical range into a per-field-type table with a dedicated synthetic benchmark.
- Fix the "Service tasks created" panel's hardcoded `[24h]` range on the `chvrsl2` dashboard.
