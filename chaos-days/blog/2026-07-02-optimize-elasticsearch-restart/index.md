---
layout: posts
title:  "optimize elasticsearch restart"
date:   2026-07-02
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Chaos Day Summary

**TL;DR;** 

<!--truncate-->

## Chaos Experiment


Most of the time, Optimize correctly recovers when Elasticsearch nodes restart.

We found some cases where Optimize doesn't, and we want to undertand these edge cases.


hypothesis:

1. Size of the shards (too big, slow ES)
2. OK: restart node with no shard
3. to test: restart node with at least one shard used by Optimize
    with / without replication
4. to test: restart the node holding the shard containing partition 2 and 3 (routing bug)


to test: how adding replicas help?


#### first

trying to reproduce the issue from yesterday

need to find the shards for zeebe-record-process-instance

1. for zeebe-record_process_8.10.0_2026-07-02: 3 primaries, no replicas

We are hitting the issue mentioned in https://github.com/camunda/camunda/issues/54601:

1. shard 0 is empty
2. shard 1 and 2 contains 

```
$ curl -s "localhost:9200/_cat/shards/zeebe-record_process-instance*?v=true"
index                                           shard prirep state     docs   store dataset ip            node
zeebe-record_process-instance_8.10.0_2026-07-02 0     p      STARTED      0    249b    249b 10.152.18.136 elastic-0
zeebe-record_process-instance_8.10.0_2026-07-02 1     p      STARTED 708701 108.4mb 108.4mb 10.152.4.5    elastic-2
zeebe-record_process-instance_8.10.0_2026-07-02 2     p      STARTED 353889  47.1mb  47.1mb 10.152.70.198 elastic-1
```

we can find which shard will be used based on the routing key: Camunda uses the partition ID as the routing key:

```
curl "localhost:9200/zeebe-record_process-instance_8.10.0_2026-07-02/_search_shards?routing=2"
```

giving
```json
http "localhost:9200/zeebe-record_process-instance_8.10.0_2026-07-02/_search_shards?routing=3" | jq .shards
[
  [
    {
      "state": "STARTED",
      "primary": true,
      "node": "oKkfytvPQEKiEQLEqbn76Q",
      "relocating_node": null,
      "shard": 1,
      "index": "zeebe-record_process-instance_8.10.0_2026-07-02",
      "allocation_id": {
        "id": "tmD1vSTuQ-uAwdshonUHnA"
      },
      "relocation_failure_info": {
        "failed_attempts": 0
      }
    }
  ]
]
```


We can see that:

```shell
for partitionID in 1 2 3; do
    shard=$(curl -s "localhost:9200/zeebe-record_process-instance_8.10.0_2026-07-02/_search_shards?routing=$partitionID" | jq '.shards[][].shard')
    echo "Partition ID $partitionID ⇒ shard=$shard"
done
```
gives:
```
Partition ID 1 ⇒ shard=2
Partition ID 2 ⇒ shard=1
Partition ID 3 ⇒ shard=1
```

so shard 0 is never used, shard 1 is used 2 times more than shard 2


we have a steady state (cf. screenshot)

restart node with shard=0; expect no impact

restart elastic-0:

```
$ curl -s "localhost:9200/_cat/shards/zeebe-record_process-instance*?v=true"           
index                                           shard prirep state      docs   store dataset ip            node
zeebe-record_process-instance_8.10.0_2026-07-02 0     p      STARTED       0    249b    249b 10.152.18.136 elastic-0
zeebe-record_process-instance_8.10.0_2026-07-02 1     p      STARTED 3262900 546.3mb 546.3mb 10.152.4.5    elastic-2
zeebe-record_process-instance_8.10.0_2026-07-02 2     p      STARTED 1629285 211.4mb 211.4mb 10.152.70.198 elastic-1
```

we confirmed we have replicas for all the optimize indices:
```
$ curl -s "localhost:9200/_cat/indices/optimize*?v=true"
health status index                                            uuid                   pri rep docs.count docs.deleted store.size pri.store.size dataset.size
green  open   optimize-process-instance-bankdisputehandling_v8 NJP28c2VTValgK_Pm0oNLA   3   1    3070686       633049      5.6gb          2.8gb        2.8gb
green  open   optimize-settings_v3                             Sm_dhf78QAy5Aqr4f91cww   1   1          0            0       498b           249b         249b
green  open   optimize-dashboard_v8                            P1z3F5HKSJCoptzv5XiYhg   1   1         10            0       21kb         10.5kb       10.5kb
green  open   optimize-dashboard-share_v4                      xzqcZbQXRye8c6IGH3qR4Q   1   1          0            0       498b           249b         249b
green  open   optimize-single-process-report_v11               jPRlutcNQEeNseET4RAFTQ   1   1          3            0      151kb         75.5kb       75.5kb
green  open   optimize-tenant_v3                               YgxSAjtnR_iV50rMUZ1TlQ   1   1          0            0       498b           249b         249b
green  open   optimize-report-share_v3                         pXydGPf7TBW4b0jzJMMgrA   1   1          0            0       498b           249b         249b
green  open   optimize-process-definition_v6                   nGxETXInRLSbQ7JTgKTR_g   1   1          2            0    424.8kb        212.4kb      212.4kb
green  open   optimize-collection_v5                           vYAuSvGYTD-U8GevLL-KVQ   1   1          0            0       498b           249b         249b
green  open   optimize-decision-definition_v5                  cA-_3YboT3Gb9VJ7heOSog   1   1          0            0       498b           249b         249b
green  open   optimize-timestamp-based-import-index_v5         -qNwD0w0QamUcxBLuWeWGg   1   1          0            0       498b           249b         249b
green  open   optimize-single-decision-report_v10              HRO-TnwjSl6nc-x_YTDtMA   1   1          0            0       498b           249b         249b
green  open   optimize-variable-label_v1                       yBOjeMYhRQOETGsuBlAW3Q   1   1          0            0       498b           249b         249b
green  open   optimize-combined-report_v5                      _AtBcQdeRleBsrKmrbSJGQ   1   1          0            0       498b           249b         249b
green  open   optimize-alert_v4                                qjCe3m6zQzC_fgE4ElQULw   1   1          0            0       498b           249b         249b
green  open   optimize-terminated-user-session_v3              xSzciMRQTZCg8B6-j86vaw   1   1          0            0       498b           249b         249b
green  open   optimize-external-process-variable_v2-000001     K5rkpnBWTG66BMy3IlDWSw   1   1          0            0       498b           249b         249b
green  open   optimize-process-overview_v2                     Khnz7wy-TNyFd0QKvpbw4Q   1   1          2            0      9.7kb          4.8kb        4.8kb
green  open   optimize-metadata_v3                             iAuJil4lRN681wlFZD6g3Q   1   1          1            0      9.5kb          4.7kb        4.7kb
green  open   optimize-instant-dashboard_v1                    dOpIbXIJSc-JGzzmKHh2yQ   1   1          0            0       498b           249b         249b
green  open   optimize-business-key_v2                         EHtHbEFaRDiqmb2R9gSaLw   1   1          0            0       498b           249b         249b
green  open   optimize-position-based-import-index_v3          emgDxywLThOI0R3ZKyJbHw   1   1         15            0    305.3kb        152.6kb      152.6kb
green  open   optimize-process-instance-refundingprocess_v8    HCRGR_aGS_eVLDphlT6mDg   3   1    1779172       196828      2.3gb          1.1gb        1.1gb
```

so we should not see any error in Optimize logs

deleted pod elastic-0:
```
k delete pod elastic-0
```


note for the DL team: if they change the routing mechanism, they should also sync with the data consumers (optimize?)


in the optimize logs, we can see some errors but related to "process" and not "process instance":

```
2026-07-02 11:46:56.486 CEST
optimize
Was not able to import next page, retrying after sleeping for 1500ms.
2026-07-02 11:46:56.478 CEST
optimize
Was not able to retrieve zeebe records of type process from partition 1
2026-07-02 11:46:56.478 CEST
optimize
Dynamically reducing import page size to 500 for next fetch attempt for type process from partition 1
```

we confirmed that the "process" index is indeed single shard, no replica and hosted on the restarted node:

```
$ curl -s "localhost:9200/_cat/shards/zeebe-record_process_*?v=true"
index                                  shard prirep state   docs   store dataset ip            node
zeebe-record_process_8.10.0_2026-07-02 0     p      STARTED    6 845.3kb 845.3kb 10.152.18.137 elastic-0
```

but this is not related to our investigation

effect on importing/exporting:

screenshot

exporting: a drop followed by spike:

* 4k -> 3.4k -> 4.5k -> back to 4k


hypothesis: an index in which we write in had a primary shard without replica hosted on elastic-0. Confirmed at least with job index:

```
$ curl -s "localhost:9200/_cat/shards/zeebe-record_job_*?v=true" 
index                              shard prirep state     docs   store dataset ip            node
zeebe-record_job_8.10.0_2026-07-02 0     p      STARTED      0    249b    249b 10.152.70.198 elastic-1
zeebe-record_job_8.10.0_2026-07-02 1     p      STARTED 614165 106.7mb 106.7mb 10.152.18.137 elastic-0      <===========
zeebe-record_job_8.10.0_2026-07-02 2     p      STARTED 307064  42.8mb  42.8mb 10.152.4.5    elastic-2
```

importing: small drop: 
* 3.8k -> 3.3kk then back to 3.8k

hypothesis:
* importing from "process" was not possible because of the missing single shard
* optimize started to backoff importing (just for this index, or generally)
* did optimize start to backoff all the activity with elasticsearch?

before the es node restart, we see the "Scheduling import round for ZeebeProcessInstanceImportMediator" log every ~1 second

at 11:46:56.478 we see the first reaction from Optimize about ES restarting and being unavailable
after that, there are several log lines indicating that Optimize backs off from importing:

```
Was not able to import next page, retrying after sleeping for 5063ms.
```

caused by:
```
org.elasticsearch.client.ResponseException: method [POST], host [http://elastic:9200], URI [/zeebe-record-process/_search?routing=1&typed_keys=true&request_cache=false], status line [HTTP/1.1 503 Service Unavailable]
{"error":{"root_cause":[{"type":"no_shard_available_action_exception","reason":null}],"type":"search_phase_execution_exception","reason":"all shards failed","phase":"query","grouped":true,"failed_shards":[{"shard":0,"index":"zeebe-record_process_8.10.0_2026-07-02","node":null,"reason":{"type":"no_shard_available_action_exception","reason":null}}]},"status":503}
```

all the back offs accumulate to ~30 seconds:

* retrying after sleeping for 10000ms.
* retrying after sleeping for 7595ms.
* retrying after sleeping for 5063ms.
* retrying after sleeping for 3375ms.
* retrying after sleeping for 2250ms.
* retrying after sleeping for 1500ms.


30 second gap for importing PIs:

```
2026-07-02 11:47:27.944 CEST io.camunda.optimize.service.importing.zeebe.mediator.ZeebeProcessInstanceImportMediator Records of type PROCESS_INSTANCE from partition 1 imported in page: 1000
2026-07-02 11:46:56.407 CEST io.camunda.optimize.service.importing.zeebe.mediator.ZeebeProcessInstanceImportMediator Records of type PROCESS_INSTANCE from partition 2 imported in page: 717
```

checking the thread metadata, it's a single thread ZeebeImportScheduler-1 that seems to import all of Optimize data
the thread is the one being throttled when it couldn't import the "process" records.


however importing is affected for ~3 minutes (in the metrics)


summary:

1. "process" importer is backed off because single shard unavailable
2. other importers are also impacted: single thread importer?



then it takes about 45 minutes to restore the original import page size on the "process" importer


### Expected

Optimize should correctly recover when Elasticsearch nodes restart.

### Actual

## Found Bugs


