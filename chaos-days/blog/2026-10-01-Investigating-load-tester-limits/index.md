---
layout: posts
title:  "Investigating load tester limits"
date:   2026-10-01
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - performance
authors: 
  - jon
  - zell
---


<!-- Input from:
  - https://camunda.slack.com/archives/D02NM2V2EMR/p1790767067613759
  - https://camunda.slack.com/archives/C0A22S6M4TF/p1790841829085949

 -->

# Chaos Day Summary

In todays chaos day, we looked into an issue covering our load tester applications (described here https://github.com/camunda/camunda/issues/62783).

![](daily-results.png)

- We have on a daily basis load tests running.
- Since a while we ran now load tests against non-secondary storage setups, to max out the performance and observe the limits of our applications under test
- What is interesting on the results was that the results for REST and non-secondary storage was worse then with ES secondary storage, while for gRPC was the opposite: non-secondary storage performed better than with ES secondary storage. (this would be also the expected case - as the secondary storage normally slows down the system).


We already traced this to the starter when creating the issue as we observed that the starter was heavily CPU throttled.

In todays chaos days, we wanted to get into the root of this evil.

**TL;DR;** We found that the Starter was accumulating a lot of requests as backlog, causing high GC pressure, memory contention, and ultimately CPU throttling, which led to reduced throughput. Adding limits to the concurrency of scheduled tasks helped mitigate this issue, while staying within the open-model architecture.

<!--truncate-->

## Chaos Experiment


As a first step we investigated the daily load tests results, and validated whether we can get something out of the existing data.


Later, we ran several experiments to understand the behavior of our load tester applications with the non-secondary storage setups.

1. We ran and experiment with the "normal" maximum load (which we use normally with ES secondary storage) - 300 PIs
2. We ran an experiment with an increased load to reproduce the behavior we observed in our daily tests - 500 PIs

In addition to the initial experiments, we ran several more experiments with different fixes, etc.

### Investigation of daily load tests


Comparing gRPC and REST with non-secondary storage setups, we observed distinct performance patterns as shown in the daily server and starter metrics.

![](daily-server-metrics.png)


Looking at the starter metrics we saw nothing obviours, it looked like we were not hitting any obvious bottlenecks at the starter level. It was counting an expected rate of requests per second consistent with the load we applied.

![](daily-starter-metrics.png)


Checking the CPU throttling metrics, we observed that the starter was indeed heavily CPU throttled during the high load experiments.


![](daily-cpu-throttle.png)


#### Logs


When looking in the related logs we realized that the load tests were making use of REST read queries (to measure REST API performance) - which is not possible when non-secondary storage is configured.

```
io.camunda.client.api.command.ProblemException: Failed with code 403: 'Forbidden'. Details: 'class ProblemDetail {
    type: about:blank
    title: FORBIDDEN
    status: 403
    detail: This endpoint requires a secondary storage, but none is set. Secondary storage can be configured using the 'camunda.data.secondary-storage.type' property.
    instance: /v2/process-instances/6755399443006124
}'
	at io.camunda.client.impl.http.ApiCallback.handleErrorResponse(ApiCallback.java:153)
	at io.camunda.client.impl.http.ApiCallback.completed(ApiCallback.java:78)
	at io.camunda.client.impl.http.ApiCallback.completed(ApiCallback.java:34)
	at org.apache.hc.core5.concurrent.BasicFuture.completed(BasicFuture.java:148)
	at org.apache.hc.core5.concurrent.ComplexFuture.completed(ComplexFuture.java:72)
	at org.apache.hc.client5.http.impl.async.InternalAbstractHttpAsyncClient$2$1.completed(InternalAbstractHttpAsyncClient.java:321)
	at org.apache.hc.core5.http.nio.support.AbstractAsyncResponseConsumer$1.completed(AbstractAsyncResponseConsumer.java:101)
	at org.apache.hc.core5.http.nio.entity.AbstractBinAsyncEntityConsumer.completed(AbstractBinAsyncEntityConsumer.java:87)
	at org.apache.hc.core5.http.nio.entity.AbstractBinDataConsumer.streamEnd(AbstractBinDataConsumer.java:83)
	at org.apache.hc.core5.http.nio.support.AbstractAsyncResponseConsumer.streamEnd(AbstractAsyncResponseConsumer.java:142)
	at org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.streamEnd(HttpAsyncMainClientExec.java:283)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamHandler.dataEnd(ClientHttp1StreamHandler.java:285)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamDuplexer.dataEnd(ClientHttp1StreamDuplexer.java:376)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.onInput(AbstractHttp1StreamDuplexer.java:343)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1IOEventHandler.inputReady(AbstractHttp1IOEventHandler.java:64)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1IOEventHandler.inputReady(ClientHttp1IOEventHandler.java:41)
	at org.apache.hc.core5.reactor.InternalDataChannel.onIOEvent(InternalDataChannel.java:139)
	at org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)
	at org.apache.hc.core5.reactor.SingleCoreIOReactor.processEvents(SingleCoreIOReactor.java:193)
	at org.apache.hc.core5.reactor.SingleCoreIOReactor.doExecute(SingleCoreIOReactor.java:140)
	at org.apache.hc.core5.reactor.AbstractSingleCoreIOReactor.execute(AbstractSingleCoreIOReactor.java:92)
	at org.apache.hc.core5.reactor.IOReactorWorker.run(IOReactorWorker.java:44)
	at java.base/java.lang.Thread.run(Thread.java:1583)
```

_To note: The returned http error code, might not be really appropriate in this context, as it is caused by the lack of secondary storage rather than an actual forbidden request._

<!-- TODO: Link issue -->


After these exceptions we see also several timeout exceptions


```
io.camunda.client.api.command.ClientException: org.apache.hc.core5.util.DeadlineTimeoutException: Deadline: 2026-09-30T03:34:45.481+0000, -116 MILLISECONDS overdue
	at io.camunda.client.impl.http.ApiCallback.failed(ApiCallback.java:88)
	at org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)
	at org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)
	at org.apache.hc.client5.http.impl.async.InternalAbstractHttpAsyncClient$2.failed(InternalAbstractHttpAsyncClient.java:367)
	at org.apache.hc.client5.http.impl.async.AsyncRedirectExec$1.failed(AsyncRedirectExec.java:261)
	at org.apache.hc.client5.http.impl.async.ContentCompressionAsyncExec$1.failed(ContentCompressionAsyncExec.java:186)
	at org.apache.hc.client5.http.impl.async.AsyncHttpRequestRetryExec$1.failed(AsyncHttpRequestRetryExec.java:203)
	at org.apache.hc.client5.http.impl.async.AsyncProtocolExec$1.failed(AsyncProtocolExec.java:294)
	at org.apache.hc.client5.http.impl.async.AsyncConnectExec$1.failed(AsyncConnectExec.java:170)
	at org.apache.hc.client5.http.impl.async.InternalHttpAsyncExecRuntime$1.failed(InternalHttpAsyncExecRuntime.java:137)
	at org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)
	at org.apache.hc.client5.http.impl.nio.PoolingAsyncClientConnectionManager$3$1.failed(PoolingAsyncClientConnectionManager.java:380)
	at org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)
	at org.apache.hc.core5.pool.LaxConnPool$LeaseRequest.failed(LaxConnPool.java:338)
	at org.apache.hc.core5.pool.LaxConnPool$PerRoutePool.servicePendingRequests(LaxConnPool.java:536)
	at org.apache.hc.core5.pool.LaxConnPool$PerRoutePool.enumAvailable(LaxConnPool.java:621)
	at org.apache.hc.core5.pool.LaxConnPool.enumAvailable(LaxConnPool.java:257)
	at org.apache.hc.client5.http.impl.nio.PoolingAsyncClientConnectionManager$2.closeExpired(PoolingAsyncClientConnectionManager.java:207)
	at org.apache.hc.client5.http.impl.nio.PoolingAsyncClientConnectionManager.closeExpired(PoolingAsyncClientConnectionManager.java:639)
	at org.apache.hc.client5.http.impl.IdleConnectionEvictor.lambda$new$0(IdleConnectionEvictor.java:65)
	at java.base/java.lang.Thread.run(Thread.java:1583)
Caused by: org.apache.hc.core5.util.DeadlineTimeoutException: Deadline: 2026-09-30T03:34:45.481+0000, -116 MILLISECONDS overdue
	at org.apache.hc.core5.util.DeadlineTimeoutException.from(DeadlineTimeoutException.java:49)
	... 7 more
```

### Hypothesis 1: REST Query calls consume significant CPU resources

As we saw that we ran several REST query calls during our load tests, it is plausible that these calls are consuming a significant amount of CPU resources, leading to throttling and decreased performance under higher loads. Especially because we might use the same connections and thread pool as the REST client for starting instances (unlike gRPC - which runs on a separate thread pool and connection mechanism).

We started a new test with disabling this REST query calls to observe the impact on CPU usage and overall performance.

The **hypothesis didn't hold** as disabling the REST query calls did not prevent CPU throttling and decreased performance under higher loads.


### Reproduction throughput degradation

First we started an experiment with a 300 PI load to observe the system's behavior under moderate load conditions.

![](reproduce-300.png)

This performed quite stable and smooth.


We increased the load to 500 PI to observe the system's behavior under higher load conditions.

![](reproduce-500.png)

It took a while until it actually break down, first the system seemed to handle the load well, but eventually, we observed significant CPU throttling and decreased performance.

![](reproduce-500-cpu.png)

The latency was increasing directly to 5s under the 500 PI load.

![](reproduce-500-latency.png)

As soon as the started ran into this CPU throttling, the throughput started to degrade significantly, and we can see how the pressure n Camunda vanishes from the system.

![](reproduce-500-camunda-cpu.png)

### Investigation of our Code

We investigated the starter/load test application and found several things to improve:


Issue/PRs

FIX: disabling data reading queries when none storage is used
FIX: the todos for Starter (e.g metric count only after response, record response failures as well, remove try/catch, etc)
issue: add more client metrics - for HTTP
FIX: DataReaderMeter scheduling is blocking the threads
FIX: No synchronization for setting the PI context for the DataReaderMeter

<!-- TODO: Link issues -->

### Adding more observability

As it was not clear yet, what is actually going on we thought of adding more observability to the system to better understand the behavior under different load conditions. We investigated the existing metrics, and whether it is possible to get more observability from the Camunda Client and Starter.

During our research we found https://hc.apache.org/httpcomponents-client-5.6.x/observation.html

We decided to integrate this observation mechanism into our HTTP client to gain better insights into the request and response behavior under different load conditions. This would allow us to track metrics such as request duration, response status, and potential bottlenecks more effectively.

<!-- TODO: add issue -->

After several back and forths (and fights with maven, docker, etc.) we were able to finnaly get the right metrics

```
# HELP http_client_inflight
# TYPE http_client_inflight gauge
http_client_inflight{kind="async"} 315.0
# HELP http_client_pool_available
# TYPE http_client_pool_available gauge
http_client_pool_available 0.0
# HELP http_client_pool_leased
# TYPE http_client_pool_leased gauge
http_client_pool_leased 100.0
# HELP http_client_pool_pending
# TYPE http_client_pool_pending gauge
http_client_pool_pending 232.0
# HELP http_client_request_seconds
# TYPE http_client_request_seconds histogram
http_client_request_seconds_bucket{method="GET",status="200",le="0.5"} 0
http_client_request_seconds_bucket{method="GET",status="200",le="+Inf"} 1
http_client_request_seconds_count{method="GET",status="200"} 1
http_client_request_seconds_sum{method="GET",status="200"} 0.835351196
http_client_request_seconds_bucket{method="POST",status="200",le="0.5"} 10
http_client_request_seconds_bucket{method="POST",status="200",le="+Inf"} 99
http_client_request_seconds_count{method="POST",status="200"} 99
http_client_request_seconds_sum{method="POST",status="200"} 199.841334404
# HELP http_client_request_seconds_max
# TYPE http_client_request_seconds_max gauge
http_client_request_seconds_max{method="GET",status="200"} 0.835351196
http_client_request_seconds_max{method="POST",status="200"} 4.536155268
```

https://github.com/camunda/camunda/pull/64477

![](client_metrics.png)

We were able to observe that the client was actually having a lot of requests in flight, while using its maximum connection pool capacity. Yet it was not clear why this caused us to perform worse then with 300 PIs


![](client-metrics-comparing.png)

### CPU increasing (and the impact on memory)

As we were heavily CPU throttled we thought about simply increase the CPU resource allocation for the client, to validate if this would improve the performance. We doubled the CPU from 250ms to 500ms (and alter increased it to 2 cores)

While doing this we also profiled the Starter 

![](starter-profile.png)


It allows to realize that the Starter was actually doing most of the time GC (~80%). As we were using before under 1 core, we are using the serialized GC pausing all threads and impacting the overall performance.

![](starter-gc.png)


Explanation on this:

- We were runnig with a rate of 300 PI/s and able to handle all requests efficiently in time, not accumulating a big backlog
- When increasing the rate to 500 PI/s, we were accumulating more and more requests (as the system was not able to process them fast enough, as we have a limit of maximumg connection pool of 100 and responses are also slower with REST). As we accumulating more requests up to 80k, GC activity increased and became more pronounced, leading to CPU throttling and decreased performance.

We had *another hypothesis* that the increased CPU allocation would actually make it worse now, because since we increased to 2 cores we are using GC 1 an concurrent garbarge collector, so no longer directly pausing our threads. This means we were able to continue with sending more and more requests accumulating more backlog. As the GC can't keep up with the increased memory pressure, it would eventually go out of memory.

```
[01-10-2026 11:43:33 +02:00]: load-tests/docs/scripts ck-fix-starter $ k8s:camunda-benchmark-prod:c8-jb-none-500-cpu-2
$ k logs starter-55464ff455-b74nn | grep Mem
NOTE: Picked up JDK_JAVA_OPTIONS: -XX:+HeapDumpOnOutOfMemoryError
java.lang.OutOfMemoryError: Java heap space
```

This was at the end prooven by our experiment as well, in addition we ran into https://github.com/camunda/camunda/issues/34597


```
[01-10-2026 11:41:42 +02:00]: load-tests/docs/scripts ck-fix-starter $ k8s:camunda-benchmark-prod:c8-jb-none-500-cpu-2
$ k logs starter-55464ff455-b74nn | grep StackOverflow
Exception in thread "httpclient-dispatch-1" java.lang.StackOverflowError
Exception in thread "httpclient-dispatch-2" java.lang.StackOverflowError
```

As the starter is using a scheduled executor for handling tasks, once it received OOM and StackoverflowError it silently died. The rate stopped - the schedule executor stopped executing more tasks - the starter is like a zombie

Potential fix: 

https://kubernetes.io/docs/concepts/storage/volumes/#emptydir

Using empheral storage for heap dumps and use EXIT ON OOM flag for JVM

### Open / Close Model

After we found the actual problem we were thinkin how we could fix this, we remember reading about Open/Close Models in load testing.

https://grafana.com/docs/k6/latest/using-k6/scenarios/
https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/


Right now our starter follows the open model, where new tasks are continuously scheduled without waiting for the previous ones to complete. If we would wait for the response we would go into a closed model. 

Both have their use cases, and for sizing the close model is as well interesting, but to put stress on the system we prefer the open model.

Looking at the code, we realized we can limit the number of concurrently scheduled tasks to prevent overwhelming the system and hitting resource limits too quickly. If we would limit it to the actual rate, configured this would be equal to the close model, if we give it a bit more head room, we can still maintain the benefits of the open model while avoiding resource exhaustion.


https://github.com/camunda/camunda/pull/64477/commits/745c4d33079339a2f85d758b8e34620b3e2b1462


This fix brought us back to a stable throughput, much higher than we normally reach with an elasticsearch secondary storage.


![](fix-starter-throughput.png)

The inflight requests are in bound `rate * 10` as we configured the semaphore to limit the concurrency.

![](fix-starter-pool.png)

In addition we can see that the CPU throttling is reduced compared to before, indicating that the system is no longer overwhelmed by too many concurrent tasks.

![](fix-starter-cpu.png)

## Conclusion


The investigation revealed that we have not enough observability right now to investigate Camunda Client issues, easily. As a result, it was difficult to pinpoint the exact cause of the performance degradation. Ultimately, we were able to pin-point that the starter was overwhelmed by too many concurrent tasks, leading to high GC pressure and memory contention, leading to CPU throttling and reduced throughput. 

Increasing the CPU resources made it even worse, as it allowed more concurrent tasks to be scheduled, exacerbating the memory contention and GC pressure until the system crashed.

The key takeaway is that controlling concurrency is crucial for maintaining system stability and performance. By limiting the number of concurrently scheduled tasks, we can prevent resource exhaustion and ensure that the system operates within its capacity.


## Found Bugs

<!-- TODO: List all issues we found  -->
