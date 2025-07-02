---
layout: posts
title:  "Follow up REST API performance"
date:   2025-07-02
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Investigation REST API Performance

## REST API Metrics

One remark from last experiments was that we not have good insights for the REST API. Actually we have the necessary metrics already exposed, but not yet available in our Dashboard.

This is currently prepared with [#33907](https://github.com/camunda/camunda/pull/33907). Based on this I was able to further investigate the REST API performance.

![rest-api](rest-api.png)

What we can see is that our requests take on average more than 50ms to complete. This is causing our throughput to go down, we are not able to create 150 PI/s even.

Looking at a different Benchmark using gRPC we can see that requests take 5-10ms to complete, and having a stable throughput 

![grpc-latency](grpc-latency.png)
![grpc](grpc.png)

Due to the slower workers (on completion), we can see error reports of the workers not being able to accept further job pushes. This has been mentioned in the previous blog post as well.  This, in consequence, means the worker send FAIL commands for such jobs, to give them back. It has a cascading effect, as jobs are sent back and forth and impacting the general process instance execution latency (which grows up to 60s compared to 0.2 s).


## Investigating Worker errors

In our previous experiments we have seen the following exceptions

```
13:25:14.684 [pool-4-thread-3] WARN  io.camunda.client.job.worker - Worker benchmark failed to handle job with key 4503599628992806 of type benchmark-task, sending fail command to broker
java.lang.IllegalStateException: Queue full
	at java.base/java.util.AbstractQueue.add(AbstractQueue.java:98) ~[?:?]
	at java.base/java.util.concurrent.ArrayBlockingQueue.add(ArrayBlockingQueue.java:329) ~[?:?]
	at io.camunda.zeebe.Worker.lambda$handleJob$1(Worker.java:122) ~[classes/:?]
	at io.camunda.client.impl.worker.JobRunnableFactoryImpl.executeJob(JobRunnableFactoryImpl.java:45) ~[camunda-client-java-8.8.0-SNAPSHOT.jar:8.8.0-SNAPSHOT]
	at io.camunda.client.impl.worker.JobRunnableFactoryImpl.lambda$create$0(JobRunnableFactoryImpl.java:40) ~[camunda-client-java-8.8.0-SNAPSHOT.jar:8.8.0-SNAPSHOT]
	at io.camunda.client.impl.worker.BlockingExecutor.lambda$execute$0(BlockingExecutor.java:50) ~[camunda-client-java-8.8.0-SNAPSHOT.jar:8.8.0-SNAPSHOT]
	at java.base/java.util.concurrent.Executors$RunnableAdapter.call(Executors.java:572) ~[?:?]
	at java.base/java.util.concurrent.FutureTask.run(FutureTask.java:317) ~[?:?]
	at java.base/java.util.concurrent.ScheduledThreadPoolExecutor$ScheduledFutureTask.run(ScheduledThreadPoolExecutor.java:304) ~[?:?]
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1144) ~[?:?
```

This is actually coming from the Worker (benchmark) application, as it is collecting all [the request futures in a blocking queue](https://github.com/camunda/camunda/blob/main/zeebe/benchmarks/project/src/main/java/io/camunda/zeebe/Worker.java#L54).

As the performance is lower of handling requests, we collect more futures in the worker, causing to fill the queue. This in the end causes also to fail more jobs - causing even more work.

This allows explains why our workers have a higher memory consumption - we had to increase the worker memory to have a stable worker.

## Profiling the System

With the previous results we were encouraged to do some profiling. For the start we used [JFR](https://docs.oracle.com/javacomponents/jmc-5-4/jfr-runtime-guide/about.htm#JFRUH170) for some basic profiling.

You can do this by:

```shell
  kubectl exec -it "$1" -- jcmd 1 JFR.start duration=100s filename=/usr/local/camunda/data/flight-$(date +%d%m%y-%H%M).jfr
```

If the flight recording is done, you can copy the recording (via `kubectl cp`) and open it with Intellij (JMC didn't worked for me) 

![first-profile](first-profile.png)

We see that the Spring filter chaining is dominating the profile, which is not unexpected as every request has go through this chain. As this is a CPU based sampling profile it is likely to be part of the profile. Still, it was something interesting to note and investigate.

### Path pattern matching

Some research showed that it might be interesting to look into other path pattern matchers, as we use the (legacy) [ant path matcher](https://github.com/camunda/camunda/blob/main/dist/src/main/resources/application.properties#L17) with [regex](https://github.com/camunda/camunda/blob/main/authentication/src/main/java/io/camunda/authentication/config/WebSecurityConfig.java#L86).  

**Resources:**

 * PathPattern - https://spring.io/blog/2020/06/30/url-matching-with-pathpattern-in-spring-mvc#pathpattern
 * [Results of using PathPattern and related discussion on GH](https://github.com/spring-projects/spring-framework/issues/31098#issuecomment-1891737375)

### Gateway - Broker request latency

As we have such a high request-response latency, we have to find out where the time is spent. Ideally we would have some sort of tracing (which we didn't have yet), or we look at metrics that cover sub-parts of the system and request-response cycle.

The REST API request-response latency metric, we can take it as the complete round trip, accepting the request on the gateway edge, converting it to a Broker request, sending it to the Broker, Broker processes, sends response back, etc.

Luckily we have a metric, that is covering the part of sending the Broker request (from the other side of the Gateway) to the Broker and wait for the response. See related [code here](https://github.com/camunda/camunda/blob/main/zeebe/broker-client/src/main/java/io/camunda/zeebe/broker/client/impl/BrokerRequestManager.java#L153).

The difference shows us that there is not a small overhead, meaning that actually the Gateway to Broker request-response is slower with REST as well, which is unexpected.

This can either be because of different data is sent (?), or a different API is used, or some other execution mechanics, etc.

Using the same cluster and enabling the REST API later, we can see the immediate effect on performance.

![rest-enabled](rest-enabled.png)

#### Request handling execution logic

A difference we have spotted with REST API and gRPC is the usage of the BrokerClient.

While we use on the gRPC side the [BrokerClient with retries](https://github.com/camunda/camunda/blob/main/zeebe/gateway-grpc/src/main/java/io/camunda/zeebe/gateway/EndpointManager.java#L457) and direct response handling, on the REST API we use no retries and [handle the response async with the ForkJoinPool](https://github.com/camunda/camunda/blob/main/service/src/main/java/io/camunda/service/ApiServices.java#L55).

As our benchmark clusters have two CPUs, [meaning 1 Thread for the common ForkJoin thread pool](https://docs.oracle.com/javase/8/docs/api/java/util/concurrent/ForkJoinPool.html) we expected some contention on the thread.

For testing purposes we increased the thread count by: `-Djava.util.concurrent.ForkJoinPool.common.parallelism=8`

In a profile we can see that more threads are used, but it doesn't change anything in the performance.

![profile-inc-fork-join](profile-inc-fork-join.png)

![rest-gw-metrics-after-increaese-thread-pool](rest-gw-metrics-after-increaese-thread-pool.png)

The assumption was that we might not be able to handle the response in time with one thread, and this causes some contention also on the Gateway-Broker request-response cycle, but this is not the case.

We seem to spend time somewhere else or have a general resource contention issue. What we can see is that we have to work with more CPU throttling, then without REST API usage.

![rest-api-cpu-throttling.png](rest-api-cpu-throttling.png)

Increasing the CPU resolves the general performance problem - hinting even more that we might have some issues with threads competing with resources, etc.

In the following screenshot you see the test with 6 CPUs per Camunda application.

![six-cpus](six-cpus.png)

Compared to the previous run with 2 CPUs per Camunda application, where it had to fight with a lot of CPU throttling. The request-response latency was five times higher on average.

![two-cpus](two-cpus.png)
