---
layout: posts
title:  "Experiment without Exporters"
date:   2020-07-30
categories: 
  - chaos_experiment 
  - exporters
authors: zell
---

# Chaos Day Summary

 * Run a chaos experiment without exporters

<!--truncate-->

## Chaos Experiment

 We wanted to run a chaos experiment, which covers [#20](https://github.com/camunda/zeebe-chaos/issues/20).
 Furthermore, it was recently asked in the forum whether it makes a difference performance wise to run a broker without exporters, see [here](https://forum.zeebe.io/t/zeebe-low-performance/1356/17) 

### Expected

 Running Broker without exporter should work without any problems. We should be able to delete data on every snapshot interval.
 Performance wise there should be no difference. 

### Actual

 We have setup three different benchmarks:

 * default with elastic and metrics exporter enabled
 * only with metrics exporter
 * without any exporter

 These benchmarks run overnight without bigger issues. This means all of three where able to take snapshots and compact the log. This satisfy our hypothesis of https://github.com/camunda/zeebe-chaos/issues/20 .

| Default | Without exporters |
|---|---|
| ![default-pvc](default-pvc.png) | ![without-exporter-pvc](without-exporter-pvc.png) |

The resource consumption seem to be kind of similar, but we still see that the memory usage increases overtime. This seems to be related to [#4812](https://github.com/zeebe-io/zeebe/issues/4812)

| Default | Metric Exporter | Without exporters |
|---|---|---|
| ![default](default-resources.png) | ![metric](metric-exporter-resources.png) | ![without](without-exporter-resources.png) |

 Unexpected was that we see a difference in throughput. The benchmark without exporters seem to have a better throughput overall. It is able to complete ~ 30 workflow instances more per second, then the other benchmarks.

| Default | Metric Exporter | Without exporters |
|---|---|---|
| ![default](default-general.png) | ![metric](metric-exporter-general.png) | ![without](without-exporter-general.png) |

  We compared also other benchmarks which we have currently running, e.g. a snapshot from 24-07-2020 or from the 0.24.1 release. 

| Snapshot 24-07 | Release 0.24.1 |
|---|---|
| ![snapshot](snapshot-24-7-general.png) | ![release-241](release-0241-general.png) |

  All benchmarks with exporters seem to have a throughput around ~140 workflow instance completions per second, but the benchmarks without exporters reaches ~170 workflow instance completions per second.
 When we check the metrics we can see that sometimes brokers are leader for all partition and sometimes it is good distributed, but even this makes not that huge difference as the fact to having no exporter.
 This is unexpected and we need to investigate further, created new issue for this [#5085](https://github.com/zeebe-io/zeebe/issues/5085)
 
 The latency seems to be not affected by this.

#### General Observations

##### RocksDB

After taking a look at the metrics of the different benchmarks we can see that at one benchmark we have a higher live data size, which is unexepected.


  ![without-exporter-rocksdb](without-exporter-rocksdb.png)
  ![default-rocksdb](default-rocksdb.png)
  ![metric-exporter-rocksdb](default-rocksdb.png)

Created an issue for it [#5081](https://github.com/zeebe-io/zeebe/issues/5081)

##### Atomix Bootstrap

On taking a look at the logs during starting the benchmarks we can see that the logging of atomix is not really useful.

It prints several statements which seem to be just noisy.
```
I 2020-07-30T10:17:54.198527Z TCP server listening for connections on 0.0.0.0:26502 
I 2020-07-30T10:17:54.199468Z Started 
I 2020-07-30T10:17:54.223547Z UDP server listening for connections on 0.0.0.0:26502 
I 2020-07-30T10:17:54.224941Z Joined 
I 2020-07-30T10:17:54.228644Z Started 
I 2020-07-30T10:17:54.229384Z Started 
I 2020-07-30T10:17:54.229856Z Started 
I 2020-07-30T10:17:54.231121Z Started 
```

Created an issue for it [#5080](https://github.com/zeebe-io/zeebe/issues/5080)

##### Merged log statement

Another issue we can see during startup is that from time the log statements are merged together in stackdriver.

Looks like this:

```
I 2020-07-30T09:16:47.053339Z Bootstrap Broker-1 partitions succeeded. Started 3 steps in 166 ms. 
I 2020-07-30T09:16:47.053352763Z {"severity":"DEBUG","logging.googleapis.com/sourceLocation":{"function":"startStepByStep","file":"StartProcess.java","line":63},"message":"Bootstrap Broker-1 partitions [3/3]: partition 1 started in 7 ms","serviceContext":{"service":"zeebe-broker","version":"zeebe-chaos-metric-exporter"},"context":{"threadId":1,"broker-id":"Broker-1","threadPriority":5,"loggerName":"io.zeebe.broker.system","threadName":"main"},"timestampSeconds":1596100607,"timestampNanos":52869000,"logger":"io.zeebe.broker.system","thread":"main"}{"severity":"DEBUG","logging.googleapis.com/sourceLocation":{"function":"calculateHealth","file":"CriticalComponentsHealthMonitor.java","line":91},"message":"The components are healthy. The current health status of components: {ZeebePartition-1=HEALTHY}","serviceContext":{"service":"zeebe-broker","version":"zeebe-chaos-metric-exporter"},"context":{"threadId":263,"threadPriority":5,"loggerName":"io.zeebe.broker.system","threadName":"Broker-1-zb-actors-3","actor-name":"Broker-1-ZeebePartition-1"},"timestampSeconds":1596100607,"timestampNanos":52860000,"logger":"io.zeebe.broker.system","thread":"Broker-1-zb-actors-3"}
 
I 2020-07-30T09:16:47.053379451Z 

```

Created an issue for it [#5079](https://github.com/zeebe-io/zeebe/issues/5079)

##### Wrong Exporter Configuration
When Exporter is configured falsely it breaks the start up, which means the exporter can't be loaded and we see an exception.

```
{
 insertId: "1mp0o7821rhxwebj4"  
 jsonPayload: {
  context: {…}   
  exception: "java.lang.IllegalStateException: Failed to load exporter with configuration: ExporterCfg{, jarPath='null', className='', args={index={ignoreVariablesAbove=32767}, url=http://elasticsearch-master:9200}}
	at io.zeebe.broker.system.partitions.ZeebePartition.<init>(ZeebePartition.java:145) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.Broker.lambda$partitionsStep$22(Broker.java:344) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.lambda$startStepByStep$2(StartProcess.java:60) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.takeDuration(StartProcess.java:88) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.startStepByStep(StartProcess.java:58) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.takeDuration(StartProcess.java:88) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.start(StartProcess.java:43) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.Broker.partitionsStep(Broker.java:352) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.Broker.lambda$initStart$10(Broker.java:184) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.lambda$startStepByStep$2(StartProcess.java:60) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.takeDuration(StartProcess.java:88) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.startStepByStep(StartProcess.java:58) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.takeDuration(StartProcess.java:88) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.bootstrap.StartProcess.start(StartProcess.java:43) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.Broker.internalStart(Broker.java:135) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.util.LogUtil.doWithMDC(LogUtil.java:21) [zeebe-util-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.Broker.start(Broker.java:115) [zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.StandaloneBroker.run(StandaloneBroker.java:65) [zeebe-distribution-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at org.springframework.boot.SpringApplication.callRunner(SpringApplication.java:795) [spring-boot-2.3.1.RELEASE.jar:2.3.1.RELEASE]
	at org.springframework.boot.SpringApplication.callRunners(SpringApplication.java:779) [spring-boot-2.3.1.RELEASE.jar:2.3.1.RELEASE]
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:322) [spring-boot-2.3.1.RELEASE.jar:2.3.1.RELEASE]
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:1237) [spring-boot-2.3.1.RELEASE.jar:2.3.1.RELEASE]
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:1226) [spring-boot-2.3.1.RELEASE.jar:2.3.1.RELEASE]
	at io.zeebe.broker.StandaloneBroker.main(StandaloneBroker.java:52) [zeebe-distribution-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
Caused by: io.zeebe.broker.exporter.repo.ExporterLoadException: Cannot load exporter [elasticsearch]: cannot load specified class
	at io.zeebe.broker.exporter.repo.ExporterRepository.load(ExporterRepository.java:81) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.system.partitions.ZeebePartition.<init>(ZeebePartition.java:143) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	... 23 more
Caused by: java.lang.ClassNotFoundException: 
	at jdk.internal.loader.BuiltinClassLoader.loadClass(Unknown Source) ~[?:?]
	at jdk.internal.loader.ClassLoaders$AppClassLoader.loadClass(Unknown Source) ~[?:?]
	at java.lang.ClassLoader.loadClass(Unknown Source) ~[?:?]
	at io.zeebe.broker.exporter.repo.ExporterRepository.load(ExporterRepository.java:78) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	at io.zeebe.broker.system.partitions.ZeebePartition.<init>(ZeebePartition.java:143) ~[zeebe-broker-0.25.0-SNAPSHOT.jar:0.25.0-SNAPSHOT]
	... 23 more
"   
  logger: "io.zeebe.broker.system"   
  message: "Bootstrap Broker-1 [11/12]: zeebe partitions failed with unexpected exception."   
  serviceContext: {…}   
  thread: "main"   
 }
 labels: {…}  
 logName: "projects/zeebe-io/logs/stdout"  
 receiveTimestamp: "2020-07-30T04:58:07.561999605Z"  
 resource: {…}  
 severity: "INFO"  
 sourceLocation: {…}  
 timestamp: "2020-07-30T04:58:04.221614Z"  
}
```


The Broker is shutdown afterwards.

```
I 2020-07-30T05:02:58.275180Z Bootstrap Broker-1 partitions [1/3]: partition 3 
I 2020-07-30T05:02:58.288302Z Bootstrap Broker-1 partitions [1/3]: partition 3 failed with unexpected exception. 
I 2020-07-30T05:02:58.289941Z Closing Broker-1 partitions succeeded. Closed 0 steps in 0 ms. 
I 2020-07-30T05:02:58.290305Z Bootstrap Broker-1 [11/12]: zeebe partitions failed with unexpected exception. 
I 2020-07-30T05:02:58.291341Z Closing Broker-1 [1/10]: leader management request handler 
D 2020-07-30T05:02:58.293136Z Closing Broker-1 [1/10]: leader management request handler closed in 2 ms 
I 2020-07-30T05:02:58.293512Z Closing Broker-1 [2/10]: disk space monitor 
D 2020-07-30T05:02:58.294140Z Closing Broker-1 [2/10]: disk space monitor closed in 1 ms 
I 2020-07-30T05:02:58.294509Z Closing Broker-1 [3/10]: monitoring services 
D 2020-07-30T05:02:58.295059Z Closing Broker-1 [3/10]: monitoring services closed in 1 ms 
I 2020-07-30T05:02:58.295371Z Closing Broker-1 [4/10]: topology manager 
D 2020-07-30T05:02:58.295964Z Closing Broker-1 [4/10]: topology manager closed in 0 ms 
I 2020-07-30T05:02:58.296264Z Closing Broker-1 [5/10]: cluster services 
D 2020-07-30T05:02:58.296612Z Closing Broker-1 [5/10]: cluster services closed in 0 ms 
I 2020-07-30T05:02:58.296928Z Closing Broker-1 [6/10]: subscription api 
D 2020-07-30T05:02:58.297450Z Closing Broker-1 [6/10]: subscription api closed in 0 ms 
I 2020-07-30T05:02:58.297763Z Closing Broker-1 [7/10]: command api handler 
D 2020-07-30T05:02:58.298951Z Closing Broker-1 [7/10]: command api handler closed in 0 ms 
I 2020-07-30T05:02:58.299366Z Closing Broker-1 [8/10]: command api transport 
I 2020-07-30T05:03:00.320886Z Stopped 
D 2020-07-30T05:03:00.321691Z Closing Broker-1 [8/10]: command api transport closed in 2022 ms 
I 2020-07-30T05:03:00.322220Z Closing Broker-1 [9/10]: membership and replication protocol 
I 2020-07-30T05:03:00.323776Z RaftServer{raft-partition-partition-3} - Transitioning to INACTIVE 
I 2020-07-30T05:03:00.324179Z RaftServer{raft-partition-partition-2} - Transitioning to INACTIVE 
I 2020-07-30T05:03:00.324205Z RaftServer{raft-partition-partition-1} - Transitioning to INACTIVE 
I 2020-07-30T05:03:00.343039Z Stopped 
I 2020-07-30T05:03:00.344506Z Stopped 
I 2020-07-30T05:03:00.345422Z Stopped 
I 2020-07-30T05:03:00.346547Z Stopped 
I 2020-07-30T05:03:00.347267Z 1 - Member deactivated: Member{id=1, address=zeebe-chaos-zeebe-1.zeebe-chaos-zeebe.zeebe-chaos.svc.cluster.local:26502, properties={brokerInfo=EADJAAAAAQABAAAAAwAAAAMAAAADAAAAAAABCgAAAGNvbW1hbmRBcGlJAAAAemVlYmUtY2hhb3MtemVlYmUtMS56ZWViZS1jaGFvcy16ZWViZS56ZWViZS1jaGFvcy5zdmMuY2x1c3Rlci5sb2NhbDoyNjUwMQUAAAwAAA8AAAAwLjI1LjAtU05BUFNIT1Q=}} 
I 2020-07-30T05:03:00.347747Z Stopped 
I 2020-07-30T05:03:00.348326Z Left 
I 2020-07-30T05:03:00.349083Z Stopped 
I 2020-07-30T05:03:04.364059Z Stopped 
I 2020-07-30T05:03:04.364994Z Stopped 
D 2020-07-30T05:03:04.365955Z Closing Broker-1 [9/10]: membership and replication protocol closed in 4043 ms 
I 2020-07-30T05:03:04.366433Z Closing Broker-1 [10/10]: actor scheduler 
D 2020-07-30T05:03:04.366879Z Closing blocking task runner 
D 2020-07-30T05:03:04.367217Z Closing actor thread ground 'Broker-1-zb-fs-workers' 
D 2020-07-30T05:03:04.368641Z Closing actor thread ground 'Broker-1-zb-fs-workers': closed successfully 
D 2020-07-30T05:03:04.369018Z Closing actor thread ground 'Broker-1-zb-actors' 
D 2020-07-30T05:03:04.369955Z Closing blocking task runner: closed successfully 
D 2020-07-30T05:03:04.370112Z Closing actor thread ground 'Broker-1-zb-actors': closed successfully 
D 2020-07-30T05:03:04.371034Z Closing Broker-1 [10/10]: actor scheduler closed in 4 ms 
I 2020-07-30T05:03:04.371414Z Closing Broker-1 succeeded. Closed 10 steps in 6080 ms. 
E 2020-07-30T05:03:04.371769Z Failed to start broker 1! 
```
This is works without problems. I think this is good to know.


### Code

To deploy a benchmark without exporters we had to do a `helm template` and remove all exporter related env variables.

## Participants

 * @zelldon
