---
layout: posts
title:  "Experiment with Timers and Huge Variables"
date:   2020-07-09
categories: 
  - chaos_experiment 
  - bpmn
authors: zell
---

# Chaos Day Summary

 * Failure documentation about RAFT
 * Added chaos day summaries to repo
 * Run Chaos experiment with a lot of timers
 * Run Chaos experiment with huge variables

<!--truncate-->

## Chaos Experiment

### A Lot of Timers 

Based on the Hypothesis written here:  [#31](https://github.com/camunda/zeebe-chaos/issues/31) we run an experiment with a stable load of 10 simple workflow instances per second (only start and end event) and 10 workflow instances with 
multiple timers. We wanted to explore what happens when we have a lot of timers running and especially what happens when the are triggered at once. We created the following workflow model, where timers are exponentially created.

![timerProcess](timerProcess.png)

The experiments is based on the hypotheses we wrote here [#31](https://github.com/camunda/zeebe-chaos/issues/31).

#### Expectations 

 * at some point we can't create new instances from out side, since backpressure will block that
 * the metrics for processing records (events/commands) should be stable since there will be always new events/records
 * the cluster itself should be stable


#### Observations

During running the experiments we saw that indeed we were not able to create new instances at some point.
The cluster kept stable and there was no leader change at all. Unexpected was the behavior of the processing record metrics, since it fluctuates a lot.

Furthermore we reached really high processing records throughput, which we normally not see.

![overall](overall.png)

The log appender backpressure seem to work, at some point it deferred around 1.3 million records.

On resource consumption side it seems that memory is growing, but we create also more timer might be the issue.

![mem](mem.png)

Interesting was that we saw a huge difference between processing and exporting.

![exportvsprocess](exportvsprocess.png) 

The issue we currently have is that we stop processing to trigger/evaluate due timers and write them to the log.
After we did that we will process a bunch of events again and then trigger/evaluate again. I think this should be decoupled to streamline the processing throughput. I created a new issue for that [#4931](https://github.com/zeebe-io/zeebe/issues/4931)

### Huge Variables

In order to understand better what is the impact of huge variables I did a small experiment with a payload which was ~ 5 MB.

I expected that this will not work, since this is larger then the maximum message size. I would expect an appropriate error message, but unfortunately I just got a cancel on the client side.


Starter Exception:
```
2020-07-09T12:27:20.945889183Z 12:27:20.945 [Thread-2] WARN  io.zeebe.ResponseChecker - Request failed
 I 
2020-07-09T12:27:20.945909759Z java.util.concurrent.ExecutionException: io.grpc.StatusRuntimeException: CANCELLED: HTTP/2 error code: CANCEL
 I 
2020-07-09T12:27:20.945924206Z Received Rst Stream
 I 
2020-07-09T12:27:20.945929123Z 	at java.util.concurrent.CompletableFuture.reportGet(Unknown Source) ~[?:?]
 I 
2020-07-09T12:27:20.945934351Z 	at java.util.concurrent.CompletableFuture.get(Unknown Source) ~[?:?]
 I 
2020-07-09T12:27:20.945939157Z 	at io.zeebe.ResponseChecker.run(ResponseChecker.java:41) [app.jar:0.23.3]
 I 
2020-07-09T12:27:20.945972595Z Caused by: io.grpc.StatusRuntimeException: CANCELLED: HTTP/2 error code: CANCEL
 I 
2020-07-09T12:27:20.945978663Z Received Rst Stream
 I 
2020-07-09T12:27:20.945983657Z 	at io.grpc.Status.asRuntimeException(Status.java:533) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.945988515Z 	at io.grpc.stub.ClientCalls$StreamObserverToCallListenerAdapter.onClose(ClientCalls.java:449) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.945993803Z 	at io.grpc.PartialForwardingClientCallListener.onClose(PartialForwardingClientCallListener.java:39) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.945998591Z 	at io.grpc.ForwardingClientCallListener.onClose(ForwardingClientCallListener.java:23) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946003749Z 	at me.dinowernli.grpc.prometheus.MonitoringClientCallListener.onClose(MonitoringClientCallListener.java:50) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946007316Z 	at io.grpc.internal.ClientCallImpl.closeObserver(ClientCallImpl.java:426) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946010626Z 	at io.grpc.internal.ClientCallImpl.access$500(ClientCallImpl.java:66) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946013822Z 	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl.close(ClientCallImpl.java:689) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946017026Z 	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl.access$900(ClientCallImpl.java:577) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946020414Z 	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInternal(ClientCallImpl.java:751) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946025498Z 	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInContext(ClientCallImpl.java:740) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946030508Z 	at io.grpc.internal.ContextRunnable.run(ContextRunnable.java:37) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946035265Z 	at io.grpc.internal.SerializingExecutor.run(SerializingExecutor.java:123) ~[app.jar:0.23.3]
 I 
2020-07-09T12:27:20.946040Z 	at java.util.concurrent.ThreadPoolExecutor.runWorker(Unknown Source) ~[?:?]
 I 
2020-07-09T12:27:20.946045833Z 	at java.util.concurrent.ThreadPoolExecutor$Worker.run(Unknown Source) ~[?:?]
 I 
2020-07-09T12:27:20.946050880Z 	at java.lang.Thread.run(Unknown Source) ~[?:?]
 I 
```


Interesting is what happens on the Gateway:

```
2020-07-09 14:27:33.929 CEST
Stream Error
 
Expand all | Collapse all{
 insertId: "yoxfczvg9usifqd7d"  
 jsonPayload: {
  context: {…}   
  exception: "io.netty.handler.codec.http2.Http2Exception$StreamException: Received DATA frame for an unknown stream 10277
	at io.netty.handler.codec.http2.Http2Exception.streamError(Http2Exception.java:147) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2ConnectionDecoder$FrameReadListener.shouldIgnoreHeadersOrDataFrame(DefaultHttp2ConnectionDecoder.java:596) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2ConnectionDecoder$FrameReadListener.onDataRead(DefaultHttp2ConnectionDecoder.java:239) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.Http2InboundFrameLogger$1.onDataRead(Http2InboundFrameLogger.java:48) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2FrameReader.readDataFrame(DefaultHttp2FrameReader.java:422) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2FrameReader.processPayloadState(DefaultHttp2FrameReader.java:251) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2FrameReader.readFrame(DefaultHttp2FrameReader.java:160) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.Http2InboundFrameLogger.readFrame(Http2InboundFrameLogger.java:41) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.DefaultHttp2ConnectionDecoder.decodeFrame(DefaultHttp2ConnectionDecoder.java:174) ~[netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.Http2ConnectionHandler$FrameDecoder.decode(Http2ConnectionHandler.java:378) [netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.http2.Http2ConnectionHandler.decode(Http2ConnectionHandler.java:438) [netty-codec-http2-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.ByteToMessageDecoder.decodeRemovalReentryProtection(ByteToMessageDecoder.java:501) [netty-codec-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.ByteToMessageDecoder.callDecode(ByteToMessageDecoder.java:440) [netty-codec-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.handler.codec.ByteToMessageDecoder.channelRead(ByteToMessageDecoder.java:276) [netty-codec-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.AbstractChannelHandlerContext.invokeChannelRead(AbstractChannelHandlerContext.java:379) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.AbstractChannelHandlerContext.invokeChannelRead(AbstractChannelHandlerContext.java:365) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.AbstractChannelHandlerContext.fireChannelRead(AbstractChannelHandlerContext.java:357) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.DefaultChannelPipeline$HeadContext.channelRead(DefaultChannelPipeline.java:1410) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.AbstractChannelHandlerContext.invokeChannelRead(AbstractChannelHandlerContext.java:379) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.AbstractChannelHandlerContext.invokeChannelRead(AbstractChannelHandlerContext.java:365) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.DefaultChannelPipeline.fireChannelRead(DefaultChannelPipeline.java:919) [netty-transport-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.epoll.AbstractEpollStreamChannel$EpollStreamUnsafe.epollInReady(AbstractEpollStreamChannel.java:792) [netty-transport-native-epoll-4.1.50.Final-linux-x86_64.jar:4.1.50.Final]
	at io.netty.channel.epoll.AbstractEpollChannel$AbstractEpollUnsafe$1.run(AbstractEpollChannel.java:387) [netty-transport-native-epoll-4.1.50.Final-linux-x86_64.jar:4.1.50.Final]
	at io.netty.util.concurrent.AbstractEventExecutor.safeExecute(AbstractEventExecutor.java:164) [netty-common-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.util.concurrent.SingleThreadEventExecutor.runAllTasks(SingleThreadEventExecutor.java:472) [netty-common-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.channel.epoll.EpollEventLoop.run(EpollEventLoop.java:384) [netty-transport-native-epoll-4.1.50.Final-linux-x86_64.jar:4.1.50.Final]
	at io.netty.util.concurrent.SingleThreadEventExecutor$4.run(SingleThreadEventExecutor.java:989) [netty-common-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.util.internal.ThreadExecutorMap$2.run(ThreadExecutorMap.java:74) [netty-common-4.1.50.Final.jar:4.1.50.Final]
	at io.netty.util.concurrent.FastThreadLocalRunnable.run(FastThreadLocalRunnable.java:30) [netty-common-4.1.50.Final.jar:4.1.50.Final]
	at java.lang.Thread.run(Unknown Source) [?:?]
"   
  logger: "io.grpc.netty.NettyServerHandler"   
  message: "Stream Error"   
  serviceContext: {…}   
  thread: "grpc-default-worker-ELG-1-1"   
 }

```

Created an issue for this: [#4928](https://github.com/zeebe-io/zeebe/issues/4928)

I will probably continue with this next week.

## Participants

 * @zelldon
