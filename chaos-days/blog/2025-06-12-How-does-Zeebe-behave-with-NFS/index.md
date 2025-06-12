---
layout: posts
title:  "How does Zeebe behave with NFS"
date:   2025-06-12
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: zell
---

# Chaos Day Summary

This week we ([Lena](https://github.com/lenaschoenburg), [Nicolas](https://github.com/npepinpe), [Roman](https://github.com/romansmirnov) and [I](https://github.com/ChrisKujawa)) hold a workshop where we looked into how Zeebe behaves with network file storage (NFS).

We ran several experiments with NFS and Zeebe, and messing around with connectivity.

**TL;DR;** We were able to show that NFS can handle certain connectivity issues, just causing Zeebe to process slower. IF we completely lose the connection to the NFS server, several issues can arise, like IOExceptions on flush (where RAFT goes into inactive mode) or SIGBUS errors on reading (like replay), causing the JVM to crash.

<!--truncate-->

## Setup

> Note:
> 
> You can skip this section if you're not interested in how we set up the NFS server


For our experiments we want to have a quick feedback loop, and small blast radius, meaning avoiding using K8, or any other cloud services. The idea was to set up a NFS server via docker, and mess with the network, to cause NFS errors.

### Run NFS Docker Container

After a smaller research we were able [to find a project](https://github.com/normal-computing/docker-nfs-server), that provides us a NFS server docker image.

This can be run via: 

```shell
sudo podman run \
   # Needs privileged access for setting up the exports rule, etc\
  --privileged
  # Mounting a local directory as volume into the container
  -v /home/cqjawa/nfs-workshop/nfs:/mnt/data:rw  \
   # expose the NFS por
  -p 2049:2049t \
   # Allowing the local host IP to access the NFS server 
  -e NFS_SERVER_ALLOWED_CLIENTS=10.88.0.0/12 \
   # Enable DEBUG LOGS
  -e NFS_SERVER_DEBUG=1 \ 
   ghcr.io/normal-computing/nfs-server:latest
```

### Mount the NFS to local file storage

To use the NFS server and make it available to our Zeebe container we first have to mount it via the NFS client.

This can be done via:

```shell
sudo mount -v -t nfs4 \
  -o proto=tcp,port=2049,soft,timeo=10 \
  localhost:/ \
  ~/nfs-workshop/nfs-client-mount/
```

* `-v` verbose
* `-t` file system type: tells the client to use NFS4 
* `-o` Options for the mount: `proto=tcp,port=2049,soft,timeo=10` 
    * Protocol options, like transport via `tcp`, port to be used, [soft mount](https://kb.netapp.com/on-prem/ontap/da/NAS/NAS-KBs/What_are_the_differences_between_hard_mount_and_soft_mount) to make sure to retry on unavailability and not block, timeout after 10s

### Run the Zeebe Container

After we mounted the NFS to our local filesystem we can start our Zeebe container. 

```shell
 podman run -d \
   -v /home/cqjawa/nfs-workshop/nfs-client-mount/:/usr/local/zeebe/data \
   -p 26500:26500 \
   -p 9600:9600 \
   gcr.io/zeebe-io/zeebe:8.7.5-root
```

This is mounting our NFS mounted directory into the container as data directory for the Zeebe container.

### Running load

For simplicity we used `zbctl` to start some load. As a first step we had to deploy some process model.

```shell
 zbctl --insecure deploy one_task.bpmn 
```

This was using the [one_task.bpmn](https://github.com/camunda/zeebe-chaos/blob/main/go-chaos/internal/bpmn/one_task.bpmn) from `go-chaos/`.

Creating instances in a loop:

```shell
while [[ true ]];
do 
    zbctl --insecure \
    create instance 2251799813685250;
    sleep 5;
done
```

Running worker:

```shell
 zbctl --insecure \
   create worker "benchmark-task"  \
   --handler "echo {\"result\":\"Pong\"}"
```

## Chaos Experiment - Use ipTables with containerized NFS Server

We wanted to disrupt the NFS connections with `iptables` and cause some errors. 

### Expected

We can drop packages with `iptables`, and we can observe errors in the Zeebe container logs.

### Actual

Setting up the following iptables rule should allow us to disrupt the NFS connection, but it didn't worked.
```shell
sudo iptables -A OUTPUT -p tcp --dport 2049 --sport 2049 -d localhost -j DROP
```


At the end we were setting up a lots of different rules, but nothing seem to work. 

```shell
Every 1.0s: sudo iptables -L -v                                                                                                             cq-p14s: Thu Jun 12 16:01:28 2025

Chain INPUT (policy ACCEPT 6090K packets, 11G bytes)
 pkts bytes target     prot opt in     out     source               destination
    0     0 DROP       tcp  --  any    any     anywhere             cq-p14s              tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.0.88.5            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.0.88.1            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.88.0.5            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             cq-p14s              tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             anywhere             tcp spt:nfs dpt:nfs

Chain FORWARD (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain OUTPUT (policy ACCEPT 6182K packets, 22G bytes)
 pkts bytes target     prot opt in     out     source               destination
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             0.0.0.0              tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             cq-p14s              tcp dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             localhost            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             11.0.88.5            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.0.88.5            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             0.0.0.0              tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.0.88.1            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             10.88.0.5            tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             cq-p14s              tcp spt:nfs dpt:nfs
    0     0 DROP       tcp  --  any    any     anywhere             anywhere             tcp spt:nfs dpt:nfs

```

We even suspended the NFS server, via `docker pause`. We were able to observe that data was still synced between directories.

This was some indication for us, that the kernel might do some magic behind the scenes, and the NFS server didn't worked as we expected it to.


## Chaos Experiment - Use ipTables with external NFS Server

As we were not able to disrupt the network, we thought it might make sense to externalize the NFS server (to a different host).

### Setup external NFS 

We followed [this guide](https://idroot.us/install-nfs-server-fedora-41), to set up a NFS server running on a different machine.

### Mount external NFS

The mounting was quite similar as before (except) using a different host

```shell
sudo  mount -v -t nfs4 -o proto=tcp,port=2049,soft,timeo=10 192.168.24.110:/ ~/nfs-workshop/nfs-client-mount/
```

### Run Zeebe Container

The same for running the Zeebe container.

```shell
podman run -d -v /home/cqjawa/nfs-workshop/nfs-client-mount/srv/nfs/:/usr/local/zeebe/data -p 26500:26500 -p 9600:9600 gcr.io/zeebe-io/zeebe:8.7.5-root
```


### Expected

We were expecting some errors during processing and writing when the connection was completely dropped.

### Actual


Similar to previous `iptables` we dropped all outgoing packages for the port `2049` with the new destination.

```shell
sudo iptables -A OUTPUT -p tcp --dport 2049 -d 192.168.24.110 -j DROP
```

```shell
Every 1.0s: sudo iptables -L -v                                                                                                            cq-p14s: Thu Jun 12 16:13:44 2025

Chain INPUT (policy ACCEPT 6211K packets, 11G bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain FORWARD (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination

Chain OUTPUT (policy ACCEPT 6297K packets, 23G bytes)
 pkts bytes target     prot opt in     out     source               destination
   35 2064K DROP       tcp  --  any    any     anywhere             192.168.24.110	 tcp dpt:nfs

```

Now we were actually able to observe some errors. The clients were receiving `DEADLINE EXCEEDED` exceptions (starter and worker).

```shell
2025/06/12 16:14:03 Failed to activate jobs for worker 'zbctl': rpc error: code = DeadlineExceeded desc = context deadline exceeded
Error: rpc error: code = DeadlineExceeded desc = context deadline exceeded
Error: rpc error: code = DeadlineExceeded desc = context deadline exceeded
Error: rpc error: code = DeadlineExceeded desc = stream terminated by RST_STREAM with error code: CANCEL
Error: rpc error: code = DeadlineExceeded desc = context deadline exceeded
```


After sometime running with the disconnected NFS server Zeebe actually failed to flush

```shell
[2025-06-12 09:02:00.819] [raft-server-0-1] [{actor-name=raft-server-1, actor-scheduler=Broker-0, partitionId=1, raft-role=LEADER}] ERROR
        io.atomix.raft.impl.RaftContext - An uncaught exception occurred, transition to inactive role
java.io.UncheckedIOException: java.io.IOException: Input/output error (msync with parameter MS_SYNC failed)
        at java.base/java.nio.MappedMemoryUtils.force(Unknown Source) ~[?:?]
        at java.base/java.nio.Buffer$2.force(Unknown Source) ~[?:?]
        at java.base/jdk.internal.misc.ScopedMemoryAccess.forceInternal(Unknown Source) ~[?:?]
        at java.base/jdk.internal.misc.ScopedMemoryAccess.force(Unknown Source) ~[?:?]
        at java.base/java.nio.MappedByteBuffer.force(Unknown Source) ~[?:?]
        at java.base/java.nio.MappedByteBuffer.force(Unknown Source) ~[?:?]
        at io.camunda.zeebe.journal.file.Segment.flush(Segment.java:125) ~[zeebe-journal-8.7.5.jar:8.7.5]
        at io.camunda.zeebe.journal.file.SegmentsFlusher.flush(SegmentsFlusher.java:58) ~[zeebe-journal-8.7.5.jar:8.7.5]
        at io.camunda.zeebe.journal.file.SegmentedJournalWriter.flush(SegmentedJournalWriter.java:125) ~[zeebe-journal-8.7.5.jar:8.7.5]
        at io.camunda.zeebe.journal.file.SegmentedJournal.flush(SegmentedJournal.java:173) ~[zeebe-journal-8.7.5.jar:8.7.5]
        at io.atomix.raft.storage.log.RaftLogFlusher$DirectFlusher.flush(RaftLogFlusher.java:73) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.storage.log.RaftLog.flush(RaftLog.java:196) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.impl.RaftContext.setCommitIndex(RaftContext.java:538) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.roles.LeaderAppender.appendEntries(LeaderAppender.java:560) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.roles.LeaderRole.replicate(LeaderRole.java:740) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.roles.LeaderRole.safeAppendEntry(LeaderRole.java:735) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.raft.roles.LeaderRole.lambda$appendEntry$15(LeaderRole.java:701) ~[zeebe-atomix-cluster-8.7.5.jar:8.7.5]
        at io.atomix.utils.concurrent.SingleThreadContext$WrappedRunnable.run(SingleThreadContext.java:178) ~[zeebe-atomix-utils-8.7.5.jar:8.7.5]
        at java.base/java.util.concurrent.Executors$RunnableAdapter.call(Unknown Source) ~[?:?]
        at java.base/java.util.concurrent.FutureTask.run(Unknown Source) ~[?:?]
        at java.base/java.util.concurrent.ScheduledThreadPoolExecutor$ScheduledFutureTask.run(Unknown Source) ~[?:?]
        at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(Unknown Source) ~[?:?]
        at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(Unknown Source) ~[?:?]
        at java.base/java.lang.Thread.run(Unknown Source) [?:?]
Caused by: java.io.IOException: Input/output error (msync with parameter MS_SYNC failed)
        at java.base/java.nio.MappedMemoryUtils.force0(Native Method) ~[?:?]
        ... 24 more
```

This caused the RAFT Leader role to become inactive, and uninstalling all related services.

```shell
INFO io.atomix.raft.impl.RaftContext - Transitioning to INACTIVE
```

Furthermore, interesting is that the `DiskSpaceMonitor` was detecting OOD and pausing the stream processor.
```shell
[2025-06-12 09:02:00.795] [zb-actors-0] [{actor-name=DiskSpaceUsageMonitorActor, actor-scheduler=Broker-0}] WARN 
        io.camunda.zeebe.broker.system - Out of disk space. Current available 0 bytes. Minimum needed 2147483648 bytes.
[2025-06-12 09:02:00.796] [zb-actors-0] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] WARN 
        io.camunda.zeebe.broker.system - Disk space usage is above threshold. Pausing stream processor.
```

At the end, the system was not running anymore. This means availability was impacted, but not durability, as we do not write anything wrong (or do not continue with dirty data)

## Chaos Experiment 3 - Random dropping packages

It is possible with `iptables` to randomly drop packages, allow to validate how the system behaves on certain package loss.

### Expected

We expected that here the system might also fail, potentially, with some exceptions.

### Actual

Running the following command, sets up an `iptables` rule that drops random with `80%` probability packages for destination port `2049`

```shell
sudo iptables -A OUTPUT -p tcp --dport 2049 -d 192.168.24.110 -m statistic --mode random --probability 0.80 -j DROP
```

As NFS is TCP based it seem to be that NFS can handle certain data/package loss, and is repeating the packages. 

The general processing was much slower, this was observed by the rate of how many instances were created and jobs completed.

Other than that the system continued to run healthy.

## Chaos Experiment 4 - Drop connection on reading

We wanted to cause some SIGBUS errors, as we knew this can happen with mmapped files, like it is used in Zeebe. This might be reproduced on reading of memory mapped data. 

For this we planned to create a lot of data on our Zeebe system and restarting it, causing Zeebe to fail on replay when the connection is blocked.

### Expected 

We expected that during read we cause a SIGBUS, causing the system to crash

### Actual

To make sure we are creating continuous segments, and not compacting (causing longer replay) we increased the snapshot period and reduced the log segment size.

```shell
podman run -d \
  -v /home/cqjawa/nfs-workshop/nfs-client-mount/srv/nfs/:/usr/local/zeebe/data \
  -p 26500:26500 -p 9600:9600 \
  -e ZEEBE_BROKER_THREADS_CPUTHREADCOUNT=2 \
  -e ZEEBE_BROKER_THREADS_IOTHREADCOUNT=2 \
  -e ZEEBE_BROKER_DATA_LOGSEGMENTSIZE=16MB \
  -e ZEEBE_BROKER_DATA_SNAPSHOTPERIOD=8h \
  gcr.io/zeebe-io/zeebe:8.7.5-root
```

First we set up an `iptable` rule to make sure that the reading was slower from NFS (by random dropping ~80% of packages).

```shell
sudo iptables -A OUTPUT -p tcp --dport 2049 -d 192.168.24.110 -m statistic --mode random --probability 0.80 -j DROP
```

```shell
[2025-06-12 09:25:00.543] [zb-actors-1] [{actor-name=StreamProcessor-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.processor - Processor starts replay of events. [snapshot-position: 611, replay-mode: PROCESSING]
```

When we saw that the StreamProcessor was starting with replay we started to drop packages again completely.

```shell
sudo iptables -A OUTPUT -p tcp --dport 2049 -d 192.168.24.110 -j DROP
```

After a certain period of time, we ran into a SIGBUS Error

```shell
[2025-06-12 09:25:00.543] [zb-actors-1] [{actor-name=StreamProcessor-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.processor - Processor starts replay of events. [snapshot-position: 611, replay-mode: PROCESSING]
[2025-06-12 09:25:00.545] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 - transitioning CommandApiService
[2025-06-12 09:25:00.547] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 - transitioning SnapshotDirector
[2025-06-12 09:25:00.549] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 - transitioning ExporterDirector
[2025-06-12 09:25:00.555] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 - transitioning BackupApiRequestHandler
[2025-06-12 09:25:00.557] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 - transitioning Admin API
[2025-06-12 09:25:00.558] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - Transition to LEADER on term 4 completed
[2025-06-12 09:25:00.561] [zb-actors-1] [{actor-name=ZeebePartition-1, actor-scheduler=Broker-0, partitionId=1}] INFO 
	io.camunda.zeebe.broker.system - ZeebePartition-1 recovered, marking it as healthy
[2025-06-12 09:25:00.562] [zb-actors-1] [{actor-name=HealthCheckService, actor-scheduler=Broker-0}] INFO 
	io.camunda.zeebe.broker.system - Partition-1 recovered, marking it as healthy
#
# A fatal error has been detected by the Java Runtime Environment:
#
#  SIGBUS (0x7) at pc=0x00007f89ec4601a5, pid=2, tid=49
#
# JRE version: OpenJDK Runtime Environment Temurin-21.0.7+6 (21.0.7+6) (build 21.0.7+6-LTS)
# Java VM: OpenJDK 64-Bit Server VM Temurin-21.0.7+6 (21.0.7+6-LTS, mixed mode, sharing, tiered, compressed oops, compressed class ptrs, g1 gc, linux-amd64)
# Problematic frame:
# v  ~StubRoutines::updateBytesCRC32C 0x00007f89ec4601a5
#
# Core dump will be written. Default location: Core dumps may be processed with "/usr/lib/systemd/systemd-coredump %P %u %g %s %t %c %h %d" (or dumping to /usr/local/zeebe/core.2)
#
# An error report file with more information is saved as:
# /usr/local/zeebe/hs_err_pid2.log
[275.689s][warning][os] Loading hsdis library failed
#
# If you would like to submit a bug report, please visit:
#   https://github.com/adoptium/adoptium-support/issues
```

This caused to crash the JVM and stop the docker container, as expected.

## Results

With the workshop of experimenting with NFS we got several learnings how Zeebe and NFS behaves on connectivity issues, summarized as follows:

  * We could confirm that network errors lead to unrecoverable SIGBUS errors, which cause the broker to crash.
     * This is due primarily to our usage of mmap both in RocksDB and Zeebe.
     * There is an easy workaround with RocksDB where you can simply turn off mmap, but no such workaround exists in Zeebe at the moment.
     * This only impacts availability as the application crashes, but since Zeebe is designed to be crash resilient, so no inconsistencies or data corruption.
     * We don’t have a clear idea of the frequency of these errors - it’s essentially environment based (i.e. how bad the network connectivity is).
 * With only partial connectivity (simulated by dropping packets, e.g. 70% of packets) we mostly observed performance issues, as things got slower - however messages were retried, so no errors occurred.
 * Network errors when using normal file I/O resulted in IOException as expected. 
     * This caused the Raft partition to go inactive, for example, when the leader fails to flush on commit (a known issue which is already planned to be fixed for graceful error handling).
 * When the NFS server was unavailable, the disk space monitor detected there was no more disk available, and writes stopped.
 * Did not test that it recovers when the server is back, but we expect it would.
 * **Minor**, but we should open an issue for it:
     * when the leader goes inactive, we report an internal error that there is no message handler for command-api-1, but really we should be returning an UNAVAILABLE as a proper error, and not logging this as error level (we have other means to detect this).
