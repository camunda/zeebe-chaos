---
layout: posts
title:  "Old-Clients"
date:   2021-09-23
categories: 
  - chaos_experiment
  - clients
tags:
  - availability
authors: zell
---

# Chaos Day Summary

It has been awhile since the last post, I'm happy to be back.

In today's chaos day we want to verify the hypothesis from [zeebe-chaos#34](https://github.com/camunda/zeebe-chaos/issues/34) that old 
clients can't disrupt a running cluster.

It might happen that after upgrading your Zeebe to the newest shiny version, you might forget to 
update some of your workers or starters etc. This should normally not an issue since Zeebe is 
backwards compatible, client wise since 1.x. But what happens when older clients are used. Old 
clients should not have a negative effect on a running cluster.

**TLDR** Older clients (0.26) have no negative impact on a running cluster (1.2), and clients after 
1.x are still working with the latest version. 

<!--truncate-->

## Chaos Experiment

We will run a simple setup with, three nodes and three partitions (replication factor 3). The 
version we use is the latest release candidate (1.2.0). Normally we run a load of 200 process 
instances per second (pi/s) on our benchmarks. This time we will put a load of 100 pi/s to get
something running and start an old starter (v0.26.x) with the same frequency. Later we will scale
the old starter to see whether it makes any effect.

### Expected

We expect that we can start and complete the 100 pi/s, since we can normally run 200 pi/s.

### Actual

The cluster was first started with starters of the same version, and we saw a stable load of ~100 
process instances completed per second. After starting the old starters (with version 0.26.3), we
can't observe any difference. 

Interesting is even when scaling the starters up to 10 replicas, which means 1000 PI creations per 
second, it doesn't seem to make any effect. *Side note:* The 
[starters](https://github.com/camunda-cloud/zeebe/tree/develop/benchmarks/project) have been
modified, such they only start instances without deploying the model.

![old26-general](old26-general.png)

The drops we see in the processing are related to restart's.

The gateway and grpc metrics doesn't indicate that more requests are sent. 

![old26-grpc](old26-grpc.png)
![old26-gateway](old26-gateway.png)

If we take a look in the clients log, we can see that the request are failing because the RPC Method names have been changed between 0.26 and 1.0. 

```shell
java.util.concurrent.ExecutionException: io.grpc.StatusRuntimeException: UNIMPLEMENTED: Method not found: gateway_protocol.Gateway/CreateWorkflowInstance
	...
Caused by: io.grpc.StatusRuntimeException: UNIMPLEMENTED: Method not found: gateway_protocol.Gateway/CreateWorkflowInstance
	...
```

It seems this kind of "old" requests can be blocked quite early in the request chain to make no effect. 

In order to experiment a bit further I created a starter image with version 1.0 to see whether this still works with our newest release candidate 1.2.

We can see in the metrics right after starting the starter that the throughput goes up and we can reach our 200 pi/s.

![old10-general](old10-general.png)

We run the benchmark overnight, and we haven't seen any issues. Be aware that the throughput is calculated over the 24h which makes is lower than 200.

![general](general.png)

Furthermore, taking a look at the resource consumption, especially at the gateway, gives no evidence that something wrong is going on.

![res](res.png)

### Result

We were able to confirm the hypothesis written in [zeebe-chaos#34](https://github.com/camunda/zeebe-chaos/issues/34), that an old client can't disrupt a running cluster. 

## Found Bugs

None this time :) 

