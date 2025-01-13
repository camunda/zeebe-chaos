---
layout: posts
title:  "Investigate failing Chaos Tests"
date:   2020-11-03
categories: 
  - investigation
tags:
  - tests
authors: zell
---

# Chaos Day Summary

Today as part of the Chaos Day I wanted to investigate why our current Chaos Tests are failing and why our targeting cluster has been broken by them,
see the related issue [#5688](https://github.com/zeebe-io/zeebe/issues/5688).

**TL;DR**

We found three new bugs regarding the reprocessing detection and deployment distribution, but still were not able to reproduce the real issue.

<!--truncate-->

## Investigation

I started already yesterday with the investigation and found out that two brokers (`Broker-0` and `Broker-2`) are failing to restart on partition three, but `Broker-0` is able to start the processing on partition three. See this [comment](https://github.com/zeebe-io/zeebe/issues/5688#issuecomment-720401612) for more information. Note that partition three is receiving deployment from partition one via deployment distribution.

Together with @saig0 I looked at the code and we found out that reprocessing can be a bit problematic in some situations regarding the `WorkflowPersistenceCache`, see related [comment](https://github.com/zeebe-io/zeebe/issues/5688#issuecomment-721021464).

The cache is used in the `DeploymentCreateProcessor` to verify that the deployment is new and to add it to the state. If this deployment with the same key already exists (in-memory) then the processor rejects the `CREATE` command.
Now we can have situations which might be problematic. Say we have two of the `CREATE` commands on normal processing, this can happen when the first partition re-distributes the deployment. When the first one is processed we create a follow up event (`CREATED`), on the second `CREATE` we will write a rejection, since it is already in-memory. If a leader change happens, then it is crucial where the snapshot was taken. If the snapshot position is **AFTER** the first `CREATE` this means that we will handle the second `CREATE` as the first one, which means we will generate on reprocessing a `CREATE` but on the log is a rejection written. This is because the in-memory state doesn't reflect the real state. Opened an issue for this [#5753](https://github.com/zeebe-io/zeebe/issues/5753).

The bug which we found was not the real issue we currently have with the broken cluster, since we have two nodes which have the **same** snapshot, but on one Broker it fails and another it doesn't. To really understand what the issue is we need the related log, so we need to reproduce this issue.

### Reproducing Chaos

I created a benchmark with the Zeebe version `0.24.4`. Check [this](https://github.com/zeebe-io/zeebe/tree/develop/benchmarks/setup) for how to setup a benchmark. I realized that creating a benchmark for an earlier version is currently not working because we set the `useMMap` flag in the `zeebe-values.yaml` file. After removing that it works without problems.

To run all experiments in a loop I used in the `chaos-experiments/kubernetes` folder
```
 while [ $? -eq 0 ]; do for ex in */experiment.json; do chaos run $ex; done; done

```
During running the experiments I found a bug in our chaos experiments, where it seems that some experiments are not executed correctly, see [#43](https://github.com/camunda/zeebe-chaos/issues/43).


It took a while, but at some point the experiments start to fail. Interesting is that if you look at the pods all seem to be ready, but in the metrics we can see that one partition is unhealthy (Partition one this time).
Checking the logs I found this:

```
io.zeebe.engine.processor.InconsistentReprocessingException: Reprocessing issue detected! Restore the data from a backup and follow the recommended upgrade procedure. [cause: "The key of the record on the log stream doesn't match to the record from reprocessing.", log-stream-record: {"partitionId":1,"value":{"errorMessage":"","type":"benchmark-task","errorCode":"","variables":{},"worker":"benchmark-worker","deadline":1604403149010,"bpmnProcessId":"benchmark","workflowKey":2251799813685250,"customHeaders":{},"retries":3,"elementId":"task","elementInstanceKey":2251799813688892,"workflowDefinitionVersion":1,"workflowInstanceKey":2251799813688864},"sourceRecordPosition":8054,"timestamp":1604403162815,"position":9274,"valueType":"JOB","intent":"TIME_OUT","recordType":"COMMAND","rejectionReason":"","rejectionType":"NULL_VAL","key":2251799813688902}, reprocessing-record: {key=2251799813688825, sourceRecordPosition=8054, intent=DeploymentIntent:DISTRIBUTED, recordType=EVENT}]
	at io.zeebe.engine.processor.ReProcessingStateMachine.lambda$verifyRecordMatchesToReprocessing$12(ReProcessingStateMachine.java:400) ~[zeebe-workflow-engine-0.24.4.jar:0.24.4]
	at java.util.Optional.ifPresent(Unknown Source) ~[?:?]
	at io.zeebe.engine.processor.ReProcessingStateMachine.verifyRecordMatchesToReprocessing(ReProcessingStateMachine.java:394) ~[zeebe-workflow-engine-0.24.4.jar:0.24.4]
	at io.zeebe.engine.processor.ReProcessingStateMachine.reprocessEvent(ReProcessingStateMachine.java:258) ~[zeebe-workflow-engine-0.24.4.jar:0.24.4]
	at io.zeebe.engine.processor.ReProcessingStateMachine.reprocessNextEvent(ReProcessingStateMachine.java:226) ~[zeebe-workflow-engine-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorJob.invoke(ActorJob.java:73) [zeebe-util-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorJob.execute(ActorJob.java:39) [zeebe-util-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorTask.execute(ActorTask.java:118) [zeebe-util-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorThread.executeCurrentTask(ActorThread.java:107) [zeebe-util-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorThread.doWork(ActorThread.java:91) [zeebe-util-0.24.4.jar:0.24.4]
	at io.zeebe.util.sched.ActorThread.run(ActorThread.java:204) [zeebe-util-0.24.4.jar:0.24.4]
```

I think this is caused by [#3124](https://github.com/zeebe-io/zeebe/issues/3124), because if the distribution succeeds we want to write a `DISTRUBITED` event on the log, with the stream writer from the context. This can happen concurrently with our reprocessing.
My assumption was that this is caused by a race condition, which might get fixed when we restart the pod. I restarted the `Broker-2`, which was leader for partition 3 and `Broker-1` took over and started the partition successfully.

```
2020-11-03 14:54:32.916 CET Broker-1-StreamProcessor-1 Processor finished reprocessing at event position 18711
```

I started the chaos experiments again. They are running now for a while.

Together with @npepinpe I discussed the open issues. We found another problematic bug with the in-memory state of the workflow cache. If on reprocessing an error/exception happens, then we retry the processing of this record, endless.
Before we retry we normally rollback the current transaction to discard all changes in the state. This doesn't apply for the in-memory state of the workflow cache, so this can lead to our specific scenario. We checked the log but found no
indication that on reprocessing another exception happen, which caused an retry. In anyway we need to fix the cache to avoid the bugs we have described. We concluded that we need the log of the failing cluster to really understand what was happening. We will try to fix the bugs above soon as possible and in parallel run the experiments endless until they fail again.

### Notes
To investigate the disks I prepared the follwing commands, which I can use to download the state of the brokers to my local machine.

```sh
kubectl exec zell-chaos-0244-zeebe-2 -- tar -cf data.tar.gz data/ # compress the data dir
kubectl cp zell-chaos-0244-zeebe-2:/usr/local/zeebe/data.tar.gz broker-2/data.tar.gz # download the tarball
cd broker-2/
tar -xvf broker-2-data.tar.gz

```


## New Issues

 * Gateway experiments are not executed [#43](https://github.com/camunda/zeebe-chaos/issues/43)
 * Deployment Reprocessing inconsistencies [#5753](https://github.com/zeebe-io/zeebe/issues/5753)
 
## Participants

  * @saig0
  * @npepinpe
  * @zelldon
