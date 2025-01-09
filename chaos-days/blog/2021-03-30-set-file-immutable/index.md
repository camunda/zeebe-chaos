---
layout: posts
title:  "Set file immutable"
date:   2021-03-30
categories: 
  - chaos_experiment 
  - filesystem 
  - immutable
authors: zell
---

# Chaos Day Summary

This chaos day was a bit different. Actually I wanted to experiment again with camunda cloud and verify that our high load chaos experiments are now working with the newest cluster plans, see [zeebe-cluster-testbench#135](https://github.com/zeebe-io/zeebe-cluster-testbench/issues/135). 
Unfortunately I found out that our test chaos cluster was in a way broken, that we were not able to create new clusters. Luckily this was fixed at the end of the day, thanks to @immi :) 

Because of these circumstances I thought about different things to experiment with, and I remembered that in the [last chaos day](/2021-03-23-camunda-cloud-network-partition/index.md) we worked with patching running deployments, in order to add more capabilities.
This allowed us to create ip routes and experiment with the zeebe deployment distribution. During this I have read the [capabilities list of linux](https://man7.org/linux/man-pages/man7/capabilities.7.html), and found out that we can mark files as immutable, which might be interesting for a chaos experiment.

In this chaos day I planned to find out how marking a file immutable affects our brokers and I made the hypothesis that: *If a leader has a write error, which is not recoverable, it will step down and another leader should take over.* I put this in our hypothesis backlog ([zeebe-chaos#52](https://github.com/camunda/zeebe-chaos/issues/52)). 

In order to really run this kind of experiment I need to find out whether marking a file immutable will cause any problems and if not how I can cause write errors such that affects the broker.
Unfortunately it turned out that immutable files will not cause issues on already opened file channels, but I found some other bugs/issues, which you can read below.

In the next chaos days I will search for a way to cause write errors proactively, so we can verify that our system can handle such issues.

<!--truncate-->

### Immutable File

In order to [mark a file as immutable](https://delightlylinux.wordpress.com/2012/12/11/file-immutable-attribute/) we can use the command `chattr +i`. For that we need a specific capability called `LINUX_IMMUTABLE`. Since we were at this time not able to create new clusters I tested it with the helm charts, where it is anyway easier to give us these capabilities.

We just need to add in our *values* files the following:

```shell
podSecurityContext:
  capabilities:
        add: [ "LINUX_IMMUTABLE" ]
```

Since we want to experiment with leaders I need to get the current topology from the gateway, here I found an issue on our latest zbctl build and the human output ([camunda-cloud/zeebe#6692](https://github.com/camunda-cloud/zeebe/issues/6692)). This is already fixed, thanks to [Miguel](https://github.com/MiguelPires) :rocket:!

After finding the correct leader we can execute on the corresponding broker/pod following commands to mark a file as immutable.

```shell
[zell ns:zell-chaos]$ k exec -it zell-chaos-zeebe-1 -- bash # opens an interactive shell on zell-chaos-zeebe-1
root@zell-chaos-zeebe-1# cd data/raft-partition/partitions/1/
root@zell-chaos-zeebe-1# chattr +i raft-partition-partition-1-1.log # marks the log file immutable
root@zell-chaos-zeebe-1# lsattr raft-partition-partition-1-1.log
----i---------e---- raft-partition-partition-1-1.log
```

With the last command `lsattr` we print out the file attributes, where we see that the immutable flag is set (the `i` in the output).
After setting the log file to immutable nothing really happens. I checked again the manual page and read that this not affects already created file channels.

```shell
CHATTR(1)                           General Commands Manual                          CHATTR(1)

NAME
       chattr - change file attributes on a Linux file system

      ...
      i      A  file with the 'i' attribute cannot be modified: it cannot be deleted or renamed, no link can be created to this file, most of the file's metadata can not be modified, and the
              file can not be opened in write mode.  Only the superuser or a process possessing the CAP_LINUX_IMMUTABLE capability can set or clear this attribute.
      ...
      
      BUGS AND LIMITATIONS
       The 'c', 's',  and 'u' attributes are not honored by the ext2, ext3, and ext4 filesystems as implemented in the current mainline Linux kernels.  Setting 'a' and 'i' attributes will not
       affect the ability to write to already existing file descriptors.
```

So we actually are not able to use it to cause any write errors on a running leader, but interesting was what happened later when the pod was restarted. The pod was not able to restart, since the log was still immutable. We can see the following bootstrap sequence and the related errors:

```shell
D 2021-03-30T09:11:02.433467Z Found segment: 1 (raft-partition-partition-2-1.log) 
I 2021-03-30T09:11:02.499079Z RaftServer{raft-partition-partition-2} - Transitioning to FOLLOWER 
I 2021-03-30T09:11:02.500847Z RaftPartitionServer{raft-partition-partition-1} - Starting server for partition PartitionId{id=1, group=raft-partition} 
I 2021-03-30T09:11:02.506876Z RaftServer{raft-partition-partition-2} - Server join completed. Waiting for the server to be READY 
E 2021-03-30T09:11:02.508400Z Bootstrap Broker-1 [6/13]: cluster services failed with unexpected exception. 
I 2021-03-30T09:11:02.523239Z Closing Broker-1 [1/5]: subscription api 
D 2021-03-30T09:11:02.525497Z Closing Broker-1 [1/5]: subscription api closed in 2 ms 
I 2021-03-30T09:11:02.526484Z Closing Broker-1 [2/5]: command api handler 
D 2021-03-30T09:11:02.528108Z Closing Broker-1 [2/5]: command api handler closed in 1 ms 
I 2021-03-30T09:11:02.528740Z Closing Broker-1 [3/5]: command api transport 
I 2021-03-30T09:11:03.519309Z RaftServer{raft-partition-partition-2} - Found leader 2 
I 2021-03-30T09:11:03.521376Z RaftServer{raft-partition-partition-2} - Setting firstCommitIndex to 2. RaftServer is ready only after it has committed events upto this index 
I 2021-03-30T09:11:03.522206Z RaftPartitionServer{raft-partition-partition-2} - Successfully started server for partition PartitionId{id=2, group=raft-partition} in 1171ms 
I 2021-03-30T09:11:04.553825Z Stopped 
D 2021-03-30T09:11:04.555166Z Closing Broker-1 [3/5]: command api transport closed in 2026 ms 
I 2021-03-30T09:11:04.556177Z Closing Broker-1 [4/5]: membership and replication protocol 
I 2021-03-30T09:11:04.558282Z RaftServer{raft-partition-partition-2} - Transitioning to INACTIVE 
E 2021-03-30T09:11:04.558408Z Closing Broker-1 [4/5]: membership and replication protocol failed to close. 
I 2021-03-30T09:11:04.560776Z Closing Broker-1 [5/5]: actor scheduler 
D 2021-03-30T09:11:04.561558Z Closing actor thread ground 'Broker-1-zb-fs-workers' 
D 2021-03-30T09:11:04.563600Z Closing segment: JournalSegment{id=1, version=1, index=1} 
D 2021-03-30T09:11:04.563881Z Closing actor thread ground 'Broker-1-zb-fs-workers': closed successfully 
D 2021-03-30T09:11:04.564448Z Closing actor thread ground 'Broker-1-zb-actors' 
D 2021-03-30T09:11:04.566157Z Closing actor thread ground 'Broker-1-zb-actors': closed successfully 
D 2021-03-30T09:11:04.567716Z Closing Broker-1 [5/5]: actor scheduler closed in 6 ms 
I 2021-03-30T09:11:04.568366Z Closing Broker-1 succeeded. Closed 5 steps in 2045 ms. 
E 2021-03-30T09:11:04.568908Z Failed to start broker 1! 
I 2021-03-30T09:11:04.574482Z 

Error starting ApplicationContext. To display the conditions report re-run your application with 'debug' enabled. 
E 2021-03-30T09:11:04.595919Z Application run failed 
I 2021-03-30T09:11:04.627321Z Shutting down ExecutorService 'applicationTaskExecutor' 
```

The following exception occurred on opening the log:

```java
java.util.concurrent.CompletionException: io.zeebe.journal.JournalException: java.nio.file.FileSystemException: /usr/local/zeebe/data/raft-partition/partitions/1/raft-partition-partition-1-1.log: Operation not permitted
	at java.util.concurrent.CompletableFuture.encodeThrowable(Unknown Source) ~[?:?]
	at java.util.concurrent.CompletableFuture.completeThrowable(Unknown Source) ~[?:?]
	at java.lang.Thread.run(Unknown Source) ~[?:?]
Caused by: io.zeebe.journal.JournalException: java.nio.file.FileSystemException: /usr/local/zeebe/data/raft-partition/partitions/1/raft-partition-partition-1-1.log: Operation not permitted
	at io.zeebe.journal.file.SegmentedJournal.openChannel(SegmentedJournal.java:468) ~[zeebe-journal-1.0.0-SNAPSHOT.jar:1.0.0-SNAPSHOT]
	at io.zeebe.journal.file.SegmentedJournal.loadSegments(SegmentedJournal.java:490) ~[zeebe-journal-1.0.0-SNAPSHOT.jar:1.0.0-SNAPSHOT]
```

After this exception on bootstrap the broker tries to close itself, and we see an error on closing a step `2021-03-30 11:11:04.558 CEST Closing Broker-1 [4/5]: membership and replication protocol failed to close.` This seems to be caused by a NPE.
```shell

java.lang.NullPointerException: null
	at io.atomix.raft.partition.impl.RaftPartitionServer.stop(RaftPartitionServer.java:141) ~[atomix-cluster-1.0.0-SNAPSHOT.jar:1.0.0-SNAPSHOT]
	at io.atomix.raft.partition.RaftPartition.closeServer(RaftPartition.java:165) ~[atomix-cluster-1.0.0-SNAPSHOT.jar:1.0.0-SNAPSHOT]
	at io.atomix.raft.partition.RaftPartition.close(RaftPartition.java:155) ~[atomix-cluster-1.0.0-SNAPSHOT.jar:1.0.0-SNAPSHOT]
```

The problem now here is that the broker never comes back. It is not restarted, which is a bit confusing. Furthermore it hasn't retried on opening, which is also unexpected, since it might be a temporary exception. 

We can see in stackdriver that the new leader is not able to connect, which is expected. But we also see that the other Broker never comes back which is unexpected!

```shell

W 2021-03-30T09:11:05.006825Z RaftServer{raft-partition-partition-2} - AppendRequest{term=2, leader=2, prevLogIndex=2, prevLogTerm=2, entries=0, commitIndex=2} to 1 failed: java.util.concurrent.CompletionException: io.atomix.cluster.messaging.MessagingException$NoRemoteHandler: No remote message handler registered for this message 
W 2021-03-30T09:11:05.256830Z RaftServer{raft-partition-partition-2} - AppendRequest{term=2, leader=2, prevLogIndex=2, prevLogTerm=2, entries=0, commitIndex=2} to 1 failed: java.util.concurrent.CompletionException: io.atomix.cluster.messaging.MessagingException$NoRemoteHandler: No remote message handler registered for this message 
W 2021-03-30T09:11:08.450683Z RaftServer{raft-partition-partition-3} - AppendRequest{term=2, leader=2, prevLogIndex=1, prevLogTerm=1, entries=0, commitIndex=2} to 1 failed: java.util.concurrent.CompletionException: io.atomix.cluster.messaging.MessagingException$NoRemoteHandler: No remote message handler registered for this message 
W 2021-03-30T09:11:12.897492Z RaftServer{raft-partition-partition-1} - AppendRequest{term=2, leader=2, prevLogIndex=1, prevLogTerm=1, entries=0, commitIndex=2} to 1 failed: java.util.concurrent.CompletionException: io.atomix.cluster.messaging.MessagingException$NoRemoteHandler: No remote message handler registered for this message 
W 2021-03-30T09:11:25.950363Z RaftServer{raft-partition-partition-3} - AppendRequest{term=2, leader=2, prevLogIndex=1, prevLogTerm=1, entries=0, commitIndex=2} to 1 failed: java.util.concurrent.CompletionException: io.atomix.cluster.messaging.MessagingException$NoRemoteHandler: No remote message handler registered for this message
```

We need to investigate this issue [camunda-cloud/zeebe#6702](https://github.com/camunda-cloud/zeebe/issues/6702) further, and I need to find out how we can cause write errors proactively.

#### Found Bugs

 * Int Console was not able to create new clusters in ultrachaos 
 * Zbctl human output looks broken [#6692](https://github.com/camunda-cloud/zeebe/issues/6692)
 * Broker is not correctly shutdown [#6702](https://github.com/camunda-cloud/zeebe/issues/6702)


