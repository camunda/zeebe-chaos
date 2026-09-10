---
layout: posts
title:  "Client resilience against OIDC outages"
date:   2026-09-10
categories: 
  - chaos_experiment 
  - bpmn
tags:
  - availability
authors: 
  - jon
  - zell
---

# Chaos Day Summary



**TL;DR;** 

<!--truncate-->

## Chaos Experiment

We conducted an investigation to test and understand connectivity on client bootstrap related to https://github.com/camunda/camunda/issues/58983

We want to investigate the startup or the connection bootstrapping with OIDC, observing the application deployment not authentication and being stuck. 


We should investigate certain parts like, how identity starts, the applications starts, how they connect and what else might fail in between.

Related issue https://github.com/camunda/camunda/issues/58983 load-test: clients are unable to authenticate for a long time after creation
 

### Expected


- Mostly investigating - current assumption clients are keeping connection and not renewing them after failure


Correlate 
- Creation of keycloak 
- Bootstrapping of identity
- Client connection


### Actual

#### First try - failed

```
[10-09-2026 11:18:29 +02:00]: load-tests/setup/c8-chaos-client-investigation main $ k8s:camunda-benchmark-prod:c8-ls-pt-s5-20260909 took 16s 
$ kgpo
NAME                                         READY   STATUS      RESTARTS   AGE
camunda-0                                    1/1     Running     0          63m
camunda-1                                    1/1     Running     0          65m
camunda-2                                    1/1     Running     0          66m
connectors-6945dccc6c-lkh4c                  1/1     Running     0          21h
coordinated-leader-balancer-29817157-js5dw   0/1     Completed   0          41m
coordinated-leader-balancer-29817172-9f2zd   0/1     Completed   0          26m
coordinated-leader-balancer-29817187-p2xv6   0/1     Completed   0          11m
identity-6ff4c8749b-pqpxv                    1/1     Running     0          17h
postgresql-1                                 1/1     Running     0          23h
postgresql-keycloak-1                        1/1     Running     0          17h
postgresql-pooler-5686fbf44c-54qvd           1/1     Running     0          37m
starter-6589c79c7b-l5q2q                     1/1     Running     0          62m
starter-pt1-568d4d4c88-hqv58                 1/1     Running     0          3m10s
starter-pt2-78cbffdffc-hf7j2                 1/1     Running     0          62m
starter-pt3-6f5c54754b-m9cfm                 1/1     Running     0          62m
starter-pt4-6876559d58-7r7kn                 1/1     Running     0          62m
starter-pt5-74bd4fb477-jw49x                 1/1     Running     0          34m
starter-pt6-5cb4486896-kf7c6                 1/1     Running     0          62m
starter-pt7-7d4d4c9ccc-lz6s8                 1/1     Running     0          62m
worker-f7547ff89-5td2c                       1/1     Running     0          62m
worker-f7547ff89-9fptj                       1/1     Running     0          62m
worker-f7547ff89-9qbvw                       1/1     Running     0          62m
worker-pt1-67c7f7d577-55ph4                  1/1     Running     0          61m
worker-pt1-67c7f7d577-67bgm                  1/1     Running     0          3m20s
worker-pt1-67c7f7d577-92ctd                  1/1     Running     0          62m
worker-pt1-67c7f7d577-msdgt                  1/1     Running     0          62m
worker-pt2-66b4cf8b7d-2rdxp                  1/1     Running     0          62m
worker-pt2-66b4cf8b7d-69q87                  1/1     Running     0          61m
worker-pt2-66b4cf8b7d-vhrjg                  1/1     Running     0          62m
worker-pt3-7b99955f5b-66wj5                  1/1     Running     0          61m
worker-pt3-7b99955f5b-9djwl                  1/1     Running     0          61m
worker-pt3-7b99955f5b-z29z6                  1/1     Running     0          62m
worker-pt4-54656d745f-7cc7p                  1/1     Running     0          50m
worker-pt4-54656d745f-v948j                  1/1     Running     0          61m
worker-pt4-54656d745f-vcs9r                  1/1     Running     0          62m
worker-pt5-7c94568f46-479s9                  1/1     Running     0          62m
worker-pt5-7c94568f46-b8787                  1/1     Running     0          61m
worker-pt5-7c94568f46-vcr2f                  1/1     Running     0          61m
worker-pt6-c7cf5b6d9-d8xzc                   1/1     Running     0          61m
worker-pt6-c7cf5b6d9-fv5kh                   1/1     Running     0          62m
worker-pt6-c7cf5b6d9-vnhrj                   1/1     Running     0          37m
worker-pt7-5df9777d45-9txxh                  1/1     Running     0          61m
worker-pt7-5df9777d45-cttvm                  1/1     Running     0          62m
worker-pt7-5df9777d45-j6c4j                  1/1     Running     0          61m
```

- Checking for metrics


```
[10-09-2026 11:19:11 +02:00]: load-tests/setup/c8-chaos-client-investigation main $ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ kgpo
NAME                                                         READY   STATUS      RESTARTS   AGE
camunda-0                                                    1/1     Running     0          4m38s
camunda-1                                                    1/1     Running     0          4m38s
camunda-2                                                    1/1     Running     0          4m38s
connectors-7dfdb78784-x5kf8                                  1/1     Running     0          4m38s
customer-notification-66946877d4-bzhmm                       1/1     Running     0          4m44s
dispute-process-request-get-vendor-info-b6b7c5549-mxrgc      1/1     Running     0          4m44s
dispute-process-request-proof-from-vendor-767dbf8f6b-7mxtd   1/1     Running     0          4m44s
elasticsearch-es-masters-0                                   1/1     Running     0          4m42s
elasticsearch-es-masters-1                                   1/1     Running     0          4m41s
elasticsearch-es-masters-2                                   1/1     Running     0          4m41s
extract-data-from-document-85dccbd6f9-k2bc7                  1/1     Running     0          4m43s
identity-888476b98-tb4ft                                     1/1     Running     0          4m38s
inform-about-successful-claim-8467688bf-rmm67                1/1     Running     0          4m43s
leader-balancer-29817200-mjwlr                               0/1     Completed   4          3m6s
metrics-exporter-599745b466-shdx6                            1/1     Running     0          4m43s
optimize-6b4bc8d797-474hj                                    1/1     Running     0          4m38s
postgresql-keycloak-1                                        1/1     Running     0          4m4s
prom-els-exporter-d66964655-8xcd2                            1/1     Running     0          4m43s
refunding-ff69f578d-wl9jm                                    1/1     Running     0          2m2s
starter-57bf558c4b-6n89x                                     1/1     Running     0          4m44s

```


##### Starter

```
{"timestampSeconds":1789032034,"timestampNanos":938595826,"severity":"WARNING","message":"Failed to retrieve topology: ","logging.googleapis.com/sourceLocation":{"file":"ThrottledLogger.java","line":251,"function":"io.camunda.zeebe.util.logging.ThrottledLogger.lambda$warn$34"},"threadContext":{"id":1,"name":"main","priority":5},"loggerName":"io.camunda.zeebe.metrics.ConnectionMonitor","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"},"exception":"io.camunda.client.api.command.ClientException: org.apache.hc.client5.http.HttpHostConnectException: Connect to http://camunda:8080 [camunda/10.152.76.195] failed: Connection refused\n\tat io.camunda.client.impl.http.ApiCallback.failed(ApiCallback.java:88)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)\n\tat org.apache.hc.client5.http.impl.async.InternalAbstractHttpAsyncClient$2.failed(InternalAbstractHttpAsyncClient.java:367)\n\tat org.apache.hc.client5.http.impl.async.AsyncRedirectExec$1.failed(AsyncRedirectExec.java:261)\n\tat org.apache.hc.client5.http.impl.async.ContentCompressionAsyncExec$1.failed(ContentCompressionAsyncExec.java:186)\n\tat org.apache.hc.client5.http.impl.async.AsyncHttpRequestRetryExec$1.failed(AsyncHttpRequestRetryExec.java:203)\n\tat org.apache.hc.client5.http.impl.async.AsyncProtocolExec$1.failed(AsyncProtocolExec.java:294)\n\tat org.apache.hc.client5.http.impl.async.AsyncConnectExec$2.failed(AsyncConnectExec.java:235)\n\tat org.apache.hc.core5.concurrent.CallbackContribution.failed(CallbackContribution.java:52)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)\n\tat org.apache.hc.client5.http.impl.nio.PoolingAsyncClientConnectionManager$4.failed(PoolingAsyncClientConnectionManager.java:529)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)\n\tat org.apache.hc.client5.http.impl.nio.DefaultAsyncClientConnectionOperator$1.failed(DefaultAsyncClientConnectionOperator.java:202)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)\n\tat org.apache.hc.client5.http.impl.nio.MultihomeIOSessionRequester$2$1.failed(MultihomeIOSessionRequester.java:168)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.reactor.IOSessionRequest.failed(IOSessionRequest.java:85)\n\tat org.apache.hc.core5.reactor.InternalConnectChannel.onException(InternalConnectChannel.java:109)\n\tat org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:55)\n\tat org.apache.hc.core5.reactor.SingleCoreIOReactor.processEvents(SingleCoreIOReactor.java:193)\n\tat org.apache.hc.core5.reactor.SingleCoreIOReactor.doExecute(SingleCoreIOReactor.java:140)\n\tat org.apache.hc.core5.reactor.AbstractSingleCoreIOReactor.execute(AbstractSingleCoreIOReactor.java:92)\n\tat org.apache.hc.core5.reactor.IOReactorWorker.run(IOReactorWorker.java:44)\n\tat java.base/java.lang.Thread.run(Thread.java:1583)\nCaused by: org.apache.hc.client5.http.HttpHostConnectException: Connect to http://camunda:8080 [camunda/10.152.76.195] failed: Connection refused\n\tat java.base/sun.nio.ch.Net.pollConnect(Native Method)\n\tat java.base/sun.nio.ch.Net.pollConnectNow(Net.java:694)\n\tat java.base/sun.nio.ch.SocketChannelImpl.finishConnect(SocketChannelImpl.java:973)\n\tat org.apache.hc.core5.reactor.InternalConnectChannel.onIOEvent(InternalConnectChannel.java:70)\n\tat org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)\n\t... 5 more\n"}
```

Was retrying and then connecting at some point


##### Workers

```
{"timestampSeconds":1789032077,"timestampNanos":45605265,"severity":"WARNING","message":"Failed to retrieve topology: ","logging.googleapis.com/sourceLocation":{"file":"ThrottledLogger.java","line":251,"function":"io.camunda.zeebe.util.logging.ThrottledLogger.lambda$warn$34"},"threadContext":{"id":1,"name":"main","priority":5},"loggerName":"io.camunda.zeebe.metrics.ConnectionMonitor","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"},"exception":"io.camunda.client.api.command.ClientException: java.net.ConnectException: Connection refused\n\tat io.camunda.client.impl.http.ApiCallback.failed(ApiCallback.java:88)\n\tat org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)\n\tat org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)\n\tat org.apache.hc.client5.http.impl.async.InternalAbstractHttpAsyncClient$2.failed(InternalAbstractHttpAsyncClient.java:367)\n\tat org.apache.hc.client5.http.impl.async.AsyncRedirectExec$1.failed(AsyncRedirectExec.java:261)\n\tat org.apache.hc.client5.http.impl.async.ContentCompressionAsyncExec$1.failed(ContentCompressionAsyncExec.java:186)\n\tat org.apache.hc.client5.http.impl.async.AsyncHttpRequestRetryExec$1.failed(AsyncHttpRequestRetryExec.java:203)\n\tat org.apache.hc.client5.http.impl.async.AsyncProtocolExec$1.failed(AsyncProtocolExec.java:294)\n\tat org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.failed(HttpAsyncMainClientExec.java:135)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1StreamHandler.failed(ClientHttp1StreamHandler.java:300)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1StreamDuplexer.terminate(ClientHttp1StreamDuplexer.java:191)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.shutdownSession(AbstractHttp1StreamDuplexer.java:167)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.onException(AbstractHttp1StreamDuplexer.java:412)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1IOEventHandler.exception(AbstractHttp1IOEventHandler.java:90)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1IOEventHandler.exception(ClientHttp1IOEventHandler.java:41)\n\tat org.apache.hc.core5.reactor.InternalDataChannel.onException(InternalDataChannel.java:177)\n\tat org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:55)\n\tat org.apache.hc.core5.reactor.InternalConnectChannel.onIOEvent(InternalConnectChannel.java:79)\n\tat org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)\n\tat org.apache.hc.core5.reactor.SingleCoreIOReactor.processEvents(SingleCoreIOReactor.java:193)\n\tat org.apache.hc.core5.reactor.SingleCoreIOReactor.doExecute(SingleCoreIOReactor.java:140)\n\tat org.apache.hc.core5.reactor.AbstractSingleCoreIOReactor.execute(AbstractSingleCoreIOReactor.java:92)\n\tat org.apache.hc.core5.reactor.IOReactorWorker.run(IOReactorWorker.java:44)\n\tat java.base/java.lang.Thread.run(Thread.java:1583)\nCaused by: java.net.ConnectException: Connection refused\n\tat java.base/sun.nio.ch.Net.pollConnect(Native Method)\n\tat java.base/sun.nio.ch.Net.pollConnectNow(Net.java:694)\n\tat java.base/sun.nio.ch.NioSocketImpl.timedFinishConnect(NioSocketImpl.java:542)\n\tat java.base/sun.nio.ch.NioSocketImpl.connect(NioSocketImpl.java:592)\n\tat java.base/java.net.Socket.connect(Socket.java:751)\n\tat java.base/sun.net.NetworkClient.doConnect(NetworkClient.java:178)\n\tat java.base/sun.net.www.http.HttpClient.openServer(HttpClient.java:531)\n\tat java.base/sun.net.www.http.HttpClient.openServer(HttpClient.java:636)\n\tat java.base/sun.net.www.http.HttpClient.<init>(HttpClient.java:282)\n\tat java.base/sun.net.www.http.HttpClient.New(HttpClient.java:386)\n\tat java.base/sun.net.www.http.HttpClient.New(HttpClient.java:408)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.getNewHttpClient(HttpURLConnection.java:1324)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.plainConnect0(HttpURLConnection.java:1257)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.plainConnect(HttpURLConnection.java:1143)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.connect(HttpURLConnection.java:1072)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.getOutputStream0(HttpURLConnection.java:1474)\n\tat java.base/sun.net.www.protocol.http.HttpURLConnection.getOutputStream(HttpURLConnection.java:1437)\n\tat io.camunda.client.impl.oauth.OAuthCredentialsProvider.doFetchCredentials(OAuthCredentialsProvider.java:369)\n\tat io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:317)\n\tat io.camunda.client.impl.oauth.OAuthCredentialsCache.computeIfMissingOrInvalid(OAuthCredentialsCache.java:205)\n\tat io.camunda.client.impl.oauth.OAuthCredentialsProvider.applyCredentials(OAuthCredentialsProvider.java:165)\n\tat io.camunda.client.impl.http.HttpClientFactory.lambda$defaultClientBuilder$0(HttpClientFactory.java:227)\n\tat org.apache.hc.core5.http.protocol.DefaultHttpProcessor.process(DefaultHttpProcessor.java:107)\n\tat org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.produceRequest(HttpAsyncMainClientExec.java:152)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1StreamHandler.produceOutput(ClientHttp1StreamHandler.java:199)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1StreamDuplexer.execute(ClientHttp1StreamDuplexer.java:336)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.processCommands(AbstractHttp1StreamDuplexer.java:246)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.onConnect(AbstractHttp1StreamDuplexer.java:260)\n\tat org.apache.hc.core5.http.impl.nio.AbstractHttp1IOEventHandler.connected(AbstractHttp1IOEventHandler.java:55)\n\tat org.apache.hc.core5.http.impl.nio.ClientHttp1IOEventHandler.connected(ClientHttp1IOEventHandler.java:41)\n\tat org.apache.hc.core5.reactor.InternalDataChannel.onIOEvent(InternalDataChannel.java:129)\n\tat org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)\n\t... 7 more\n"}
{"timestampSeconds":1789032078,"timestampNanos":48357472,"severity":"WARNING","message":"Token fetch failed for clientId=orchestration (attempt 1/5), retrying in 577ms: Connection refused","logging.googleapis.com/sourceLocation":{"file":"OAuthCredentialsProvider.java","line":331,"function":"io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials"},"threadContext":{"id":17,"name":"httpclient-dispatch-1","priority":5},"loggerName":"io.camunda.client.impl.oauth.OAuthCredentialsProvider","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"}}
{"timestampSeconds":1789032078,"timestampNanos":627510090,"severity":"WARNING","message":"Token fetch failed for clientId=orchestration (attempt 2/5), retrying in 1697ms: Connection refused","logging.googleapis.com/sourceLocation":{"file":"OAuthCredentialsProvider.java","line":331,"function":"io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials"},"threadContext":{"id":17,"name":"httpclient-dispatch-1","priority":5},"loggerName":"io.camunda.client.impl.oauth.OAuthCredentialsProvider","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"}}
{"timestampSeconds":1789032080,"timestampNanos":325431982,"severity":"WARNING","message":"Token fetch failed for clientId=orchestration (attempt 3/5), retrying in 3780ms: Connection refused","logging.googleapis.com/sourceLocation":{"file":"OAuthCredentialsProvider.java","line":331,"function":"io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials"},"threadContext":{"id":17,"name":"httpclient-dispatch-1","priority":5},"loggerName":"io.camunda.client.impl.oauth.OAuthCredentialsProvider","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"}}
{"timestampSeconds":1789032084,"timestampNanos":106314037,"severity":"WARNING","message":"Token fetch failed for clientId=orchestration (attempt 4/5), retrying in 6394ms: Connection refused","logging.googleapis.com/sourceLocation":{"file":"OAuthCredentialsProvider.java","line":331,"function":"io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials"},"threadContext":{"id":17,"name":"httpclient-dispatch-1","priority":5},"loggerName":"io.camunda.client.impl.oauth.OAuthCredentialsProvider","serviceContext":{"service":"load-tester","version":"c8-chaos-client-investigation"}}
```

- not clear which endpoint got refused

##### Camunda



```
{"timestampSeconds":1789032549,"timestampNanos":132345151,"severity":"ERROR","message":"Processor 'io.camunda.zeebe.engine.processing.bpmn.BpmnStreamProcessor' implements SuspensionAware but returned a null suspension behavior for command 'PROCESS_INSTANCE'; processing it normally. Please report this as a bug.","logging.googleapis.com/sourceLocation":{"file":"SuspensionBehavior.java","line":76,"function":"io.camunda.zeebe.engine.processing.streamprocessor.SuspensionBehavior.process"},"logging.googleapis.com/labels":{"actor-name":"StreamProcessor-1","actor-scheduler":"Broker-0","partitionId":"1","physicalTenant":"default"},"threadContext":{"id":32,"name":"zb-actors-0","priority":5},"loggerName":"io.camunda.zeebe.broker.process","serviceContext":{"service":"zeebe","version":"56fa5f6"},"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","reportLocation":{"filePath":"SuspensionBehavior.java","functionName":"process","lineNumber":76}}
{"timestampSeconds":1789032549,"timestampNanos":136564804,"severity":"ERROR","message":"Processor 'io.camunda.zeebe.engine.processing.job.JobCompleteProcessor' implements SuspensionAware but returned a null suspension behavior for command 'JOB'; processing it normally. Please report this as a bug.","logging.googleapis.com/sourceLocation":{"file":"SuspensionBehavior.java","line":76,"function":"io.camunda.zeebe.engine.processing.streamprocessor.SuspensionBehavior.process"},"logging.googleapis.com/labels":{"actor-name":"StreamProcessor-1","actor-scheduler":"Broker-0","partitionId":"1","physicalTenant":"default"},"threadContext":{"id":32,"name":"zb-actors-0","priority":5},"loggerName":"io.camunda.zeebe.broker.process","serviceContext":{"service":"zeebe","version":"56fa5f6"},"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","reportLocation":{"filePath":"SuspensionBehavior.java","functionName":"process","lineNumber":76}}
{"timestampSeconds":1789032549,"timestampNanos":137659045,"severity":"ERROR","message":"Processor 'io.camunda.zeebe.engine.processing.bpmn.BpmnStreamProcessor' implements SuspensionAware but returned a null suspension behavior for command 'PROCESS_INSTANCE'; processing it normally. Please report this as a bug.","logging.googleapis.com/sourceLocation":{"file":"SuspensionBehavior.java","line":76,"function":"io.camunda.zeebe.engine.processing.streamprocessor.SuspensionBehavior.process"},"logging.googleapis.com/labels":{"actor-name":"StreamProcessor-1","actor-scheduler":"Broker-0","partitionId":"1","physicalTenant":"default"},"threadContext":{"id":32,"name":"zb-actors-0","priority":5},"loggerName":"io.camunda.zeebe.broker.process","serviceContext":{"service":"zeebe","version":"56fa5f6"},"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","reportLocation":{"filePath":"SuspensionBehavior.java","functionName":"process","lineNumber":76}}
{"timestampSeconds":1789032549,"timestampNanos":138729156,"severity":"ERROR","message":"Processor 'io.camunda.zeebe.engine.processing.bpmn.BpmnStreamProcessor' implements SuspensionAware but returned a null suspension behavior for command 'PROCESS_INSTANCE'; processing it normally. Please report this as a bug.","logging.googleapis.com/sourceLocation":{"file":"SuspensionBehavior.java","line":76,"function":"io.camunda.zeebe.engine.processing.streamprocessor.SuspensionBehavior.process"},"logging.googleapis.com/labels":{"actor-name":"StreamProcessor-1","actor-scheduler":"Broker-0","partitionId":"1","physicalTenant":"default"},"threadContext":{"id":32,"name":"zb-actors-0","priority":5},"loggerName":"io.camunda.zeebe.broker.process","serviceContext":{"service":"zeebe","version":"56fa5f6"},"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","reportLocation":{"filePath":"SuspensionBehavior.java","functionName":"process","lineNumber":76}}
```

Taking down our test for now 12:06

As we had a bug discovered https://github.com/camunda/camunda/issues/62686



#### Second try


```
[10-09-2026 12:13:30 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 16s 
$ kgpo
NAME                                                         READY   STATUS      RESTARTS   AGE
camunda-0                                                    1/1     Running     0          117m
camunda-1                                                    1/1     Running     0          117m
camunda-2                                                    1/1     Running     0          117m
connectors-7dfdb78784-44x69                                  1/1     Running     0          101m
customer-notification-66946877d4-856xh                       1/1     Running     0          117m
dispute-process-request-get-vendor-info-b6b7c5549-h7g8t      1/1     Running     0          22m
dispute-process-request-proof-from-vendor-767dbf8f6b-rjnqh   1/1     Running     0          117m
elasticsearch-es-masters-0                                   1/1     Running     0          117m
elasticsearch-es-masters-1                                   1/1     Running     0          117m
elasticsearch-es-masters-2                                   1/1     Running     0          117m
extract-data-from-document-85dccbd6f9-tcfsk                  1/1     Running     0          53m
identity-888476b98-8wpf4                                     1/1     Running     0          117m
inform-about-successful-claim-8467688bf-xhvxt                1/1     Running     0          117m
leader-balancer-29817350-4cjk2                               0/1     Completed   0          20m
leader-balancer-29817360-h6vrg                               0/1     Completed   0          10m
leader-balancer-29817370-9fg7n                               0/1     Completed   0          35s
metrics-exporter-599745b466-gm8t2                            1/1     Running     0          117m
optimize-6b4bc8d797-8w5kf                                    1/1     Running     0          101m
postgresql-keycloak-1                                        1/1     Running     0          116m
prom-els-exporter-d66964655-8vv8k                            1/1     Running     0          117m
refunding-ff69f578d-fw8nd                                    1/1     Running     0          53m
starter-57bf558c4b-xt7c9                                     1/1     Running     0          53m
```

Pods were running at 12:13:30 +02:00 but traffic started only after 12:20


Connectors 

```
    readinessProbe:
      failureThreshold: 5
      httpGet:
        path: /actuator/health/readiness
        port: http
        scheme: HTTP
      initialDelaySeconds: 30
      periodSeconds: 30
      successThreshold: 1
      timeoutSeconds: 1

```


Last warning before these logs stop `2026-09-10 12:20:20.395`
```
Failed to stream jobs of type 'io.camunda.agenticai:a2aclient:0' to worker 'A2A Client'
io.grpc.StatusRuntimeException: CANCELLED
	at io.grpc.Status.asRuntimeException(Status.java:532)
	at io.grpc.stub.ClientCalls$StreamObserverToCallListenerAdapter.onClose(ClientCalls.java:581)
	at io.grpc.internal.ClientCallImpl.closeObserver(ClientCallImpl.java:566)
	at io.grpc.internal.ClientCallImpl.access$100(ClientCallImpl.java:72)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInternal(ClientCallImpl.java:734)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInContext(ClientCallImpl.java:715)
	at io.grpc.internal.ContextRunnable.run(ContextRunnable.java:37)
	at io.grpc.internal.SerializingExecutor.run(SerializingExecutor.java:133)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(Unknown Source)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(Unknown Source)
	at java.base/java.lang.Thread.run(Unknown Source)
Caused by: java.io.IOException: OAuth credentials provider is in non-retryable failure cooldown until 2026-09-10T10:20:20.499231873Z due to earlier non-retryable token endpoint response.
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:305)
	at io.camunda.client.impl.oauth.OAuthCredentialsCache.computeIfMissingOrInvalid(OAuthCredentialsCache.java:205)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.applyCredentials(OAuthCredentialsProvider.java:165)
	at io.camunda.client.impl.CamundaCallCredentials.lambda$applyRequestMetadata$0(CamundaCallCredentials.java:53)
	... 3 common frames omitted
Caused by: java.io.IOException: Failed while requesting access token with status code 401 and message Unauthorized.
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.doFetchCredentials(OAuthCredentialsProvider.java:378)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:317)
	at io.camunda.client.impl.oauth.OAuthCredentialsCache.computeIfMissingOrInvalid(OAuthCredentialsCache.java:205)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.applyCredentials(OAuthCredentialsProvider.java:165)
	at io.camunda.client.impl.http.HttpClientFactory.lambda$defaultClientBuilder$0(HttpClientFactory.java:227)
	at org.apache.hc.core5.http.protocol.DefaultHttpProcessor.process(DefaultHttpProcessor.java:107)
	at org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.…[message truncated due to size]
  ```

  Next log is 4 minutes later refreshing token

##### Starter

```
io.camunda.client.api.command.ClientException: java.io.IOException: OAuth credentials provider is in non-retryable failure cooldown until 2026-09-10T10:20:20.724006897Z due to earlier non-retryable token endpoint response.
	at io.camunda.client.impl.http.ApiCallback.failed(ApiCallback.java:88)
	at org.apache.hc.core5.concurrent.BasicFuture.failed(BasicFuture.java:166)
	at org.apache.hc.core5.concurrent.ComplexFuture.failed(ComplexFuture.java:79)
	at org.apache.hc.client5.http.impl.async.InternalAbstractHttpAsyncClient$2.failed(InternalAbstractHttpAsyncClient.java:367)
	at org.apache.hc.client5.http.impl.async.AsyncRedirectExec$1.failed(AsyncRedirectExec.java:261)
	at org.apache.hc.client5.http.impl.async.ContentCompressionAsyncExec$1.failed(ContentCompressionAsyncExec.java:186)
	at org.apache.hc.client5.http.impl.async.AsyncHttpRequestRetryExec$1.failed(AsyncHttpRequestRetryExec.java:203)
	at org.apache.hc.client5.http.impl.async.AsyncProtocolExec$1.failed(AsyncProtocolExec.java:294)
	at org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.failed(HttpAsyncMainClientExec.java:135)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamHandler.failed(ClientHttp1StreamHandler.java:300)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamDuplexer.terminate(ClientHttp1StreamDuplexer.java:191)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.shutdownSession(AbstractHttp1StreamDuplexer.java:167)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.onException(AbstractHttp1StreamDuplexer.java:412)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1IOEventHandler.exception(AbstractHttp1IOEventHandler.java:90)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1IOEventHandler.exception(ClientHttp1IOEventHandler.java:41)
	at org.apache.hc.core5.reactor.InternalDataChannel.onException(InternalDataChannel.java:177)
	at org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:55)
	at org.apache.hc.core5.reactor.InternalConnectChannel.onIOEvent(InternalConnectChannel.java:79)
	at org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)
	at org.apache.hc.core5.reactor.SingleCoreIOReactor.processEvents(SingleCoreIOReactor.java:193)
	at org.apache.hc.core5.reactor.SingleCoreIOReactor.doExecute(SingleCoreIOReactor.java:140)
	at org.apache.hc.core5.reactor.AbstractSingleCoreIOReactor.execute(AbstractSingleCoreIOReactor.java:92)
	at org.apache.hc.core5.reactor.IOReactorWorker.run(IOReactorWorker.java:44)
	at java.base/java.lang.Thread.run(Thread.java:1583)
Caused by: java.io.IOException: OAuth credentials provider is in non-retryable failure cooldown until 2026-09-10T10:20:20.724006897Z due to earlier non-retryable token endpoint response.
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:305)
	at io.camunda.client.impl.oauth.OAuthCredentialsCache.computeIfMissingOrInvalid(OAuthCredentialsCache.java:205)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.applyCredentials(OAuthCredentialsProvider.java:165)
	at io.camunda.client.impl.http.HttpClientFactory.lambda$defaultClientBuilder$0(HttpClientFactory.java:227)
	at org.apache.hc.core5.http.protocol.DefaultHttpProcessor.process(DefaultHttpProcessor.java:107)
	at org.apache.hc.client5.http.impl.async.HttpAsyncMainClientExec$1.produceRequest(HttpAsyncMainClientExec.java:152)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamHandler.produceOutput(ClientHttp1StreamHandler.java:199)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1StreamDuplexer.execute(ClientHttp1StreamDuplexer.java:336)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.processCommands(AbstractHttp1StreamDuplexer.java:246)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1StreamDuplexer.onConnect(AbstractHttp1StreamDuplexer.java:260)
	at org.apache.hc.core5.http.impl.nio.AbstractHttp1IOEventHandler.connected(AbstractHttp1IOEventHandler.java:55)
	at org.apache.hc.core5.http.impl.nio.ClientHttp1IOEventHandler.connected(ClientHttp1IOEventHandler.java:41)
	at org.apache.hc.core5.reactor.InternalDataChannel.onIOEvent(InternalDataChannel.java:129)
	at org.apache.hc.core5.reactor.InternalChannel.handleIOEvent(InternalChannel.java:51)
	... 7 more
Caused by: java.io.IOException: Failed while requesting access token with status code 401 and message Unauthorized.
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.doFetchCredentials(OAuthCredentialsProvider.java:378)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:317)
	... 20 more
```

#### 

 Deploying 

  - Elastic
  - Keycloak
  - MGTM identity
  - Camunda
  - other applications


But the bootstrap is ordered

- Elastic needs to come up and be ready
- ONLY THEN Camunda comes up and can create ES schema
- Camunda distributed system starts with partitions - all cluster nodes need to be joining this before marking partition as ready
- We need a leader for partition one - only then we can start processing on partition one
- THEN Camunda will start to create necessary init permissions AT PARTITION one - via processing and then exporting

Parallel
- Postgress starts 
- Keycloak starts
- Clients start 
- Keycloak writes stuff into Postgres
- MGMT Identity also writes stuff into keycloak - like realm, etc.,
- Only here clients are able to retrieve token successfully
-> the token is not valid yet - because it needs the permissions from Camunda itself to be authorized - CLIENTS wait also on the sequence above


#### Code review

https://github.com/camunda/camunda/blob/main/clients/java/src/main/java/io/camunda/client/impl/oauth/OAuthCredentialsProvider.java#L388C14-L388C74

https://github.com/camunda/camunda/blob/main/clients/java/src/main/java/io/camunda/client/impl/oauth/OAuthCredentialsProvider.java#L296 

https://console.cloud.google.com/logs/query;query=resource.labels.cluster_name%3D%22camunda-benchmark%22%0Aresource.labels.namespace_name%3D%22c8-chaos-client-investigation%22%0A-resource.labels.container_name%3D%22orchestration%22%0A-resource.labels.container_name%3D%22optimize%22%0Aresource.labels.container_name%3D%22starter%22%0ASEARCH%2528%22OAuth%20credentials%20provider%20latched%20%60non-retryable%60%20failure%20for%22%2529;pinnedLogId=2026-09-10T10:15:20.724532568Z%2Fikl7leplntuz6uzm;summaryFields=resource%252Flabels%252Fpod_name,resource%252Flabels%252Fcontainer_name:false:32:beginning;cursorTimestamp=2026-09-10T10:15:20.724532568Z;startTime=2026-09-10T10:08:46.011Z;endTime=2026-09-10T10:40:13.943Z?project=camunda-benchmark
```
OAuth credentials provider latched non-retryable failure for clientId=orchestration after HTTP 401 from token endpoint http://c8-chaos-client-investigation.keycloak-operator.svc.cluster.local:18080/auth/realms/camunda-platform/protocol/openid-connect/token. Token fetches will fail fast until 2026-09-10T10:20:20.724006897Z (PT5M), then a fresh attempt will be made. Verify clientId, clientSecret, audience, and token URL configuration.
```

client doesnt retry because it gets a 401 - then after 5 min cooldown it retries again


https://github.com/camunda/camunda/blob/ebfeeb73a291b23cc069b36f65704e39c2c6c4af/clients/camunda-spring-boot-starter/src/main/java/io/camunda/client/spring/properties/CamundaClientAuthProperties.java#L157

https://github.com/camunda/camunda/blob/main/clients/java/src/main/java/io/camunda/client/impl/oauth/OAuthCredentialsProviderBuilder.java#L99

https://docs.camunda.io/docs/apis-tools/camunda-spring-boot-starter/properties-reference/#camundaclientauthtokenfetchnonretryablecooldown


#### Third session


- recreate load test
- configure in load test apps https://docs.camunda.io/docs/apis-tools/camunda-spring-boot-starter/properties-reference/#camundaclientauthtokenfetchnonretryablecooldown 

`camunda.client.auth.token-fetch-non-retryable-cooldown` to be 30s - "PT30S"



```
[10-09-2026 15:19:11 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ kgpo -w
NAME                                                         READY   STATUS       RESTARTS      AGE
camunda-0                                                    0/1     Running      0             23s
camunda-1                                                    0/1     Running      0             23s
camunda-2                                                    0/1     Running      0             23s
connectors-7dfdb78784-llx75                                  0/1     Running      0             23s
customer-notification-66946877d4-7qs2c                       1/1     Running      0             30s
dispute-process-request-get-vendor-info-b6b7c5549-56ctj      1/1     Running      0             30s
dispute-process-request-proof-from-vendor-767dbf8f6b-tpvws   1/1     Running      0             29s
elasticsearch-es-masters-0                                   0/1     Running      0             28s
elasticsearch-es-masters-1                                   0/1     Running      0             28s
elasticsearch-es-masters-2                                   0/1     Running      0             28s
extract-data-from-document-85dccbd6f9-27zt6                  1/1     Running      0             29s
identity-888476b98-fgkm5                                     0/1     Running      0             23s
inform-about-successful-claim-8467688bf-skfd4                1/1     Running      0             29s
metrics-exporter-599745b466-5vtkn                            1/1     Running      0             29s
optimize-6b4bc8d797-2llpj                                    0/1     Init:Error   2 (17s ago)   23s
postgresql-keycloak-1-initdb-lnt6t                           1/1     Running      0             28s
prom-els-exporter-d66964655-ppd9j                            1/1     Running      0             29s
refunding-ff69f578d-bdsqk                                    1/1     Running      0             29s
starter-57bf558c4b-t4c6b                                     1/1     Running      0             30s
elasticsearch-es-masters-2                                   1/1     Running      0             28s

```

Config fix worked 

```
OAuth credentials provider latched non-retryable failure for clientId=orchestration after HTTP 401 from token endpoint http://c8-chaos-client-investigation.keycloak-operator.svc.cluster.local:18080/auth/realms/camunda-platform/protocol/openid-connect/token. Token fetches will fail fast until 2026-09-10T13:29:19.407492112Z (PT30S), then a fresh attempt will be made. Verify clientId, clientSecret, audience, and token URL configuration.
```

#### 4 experiment


- we want to get understanding of how the system behaves under OIDC outages - or better identity restarts together with workers https://github.com/camunda/camunda/issues/62647


Identity is sharing the node with a owkrer pod (might be common based on the sizes ) - might happen in our issue as well
```
$ k describe pod identity-888476b98-db4hd
Name:             identity-888476b98-db4hd
Namespace:        c8-chaos-client-investigation
Priority:         0
Service Account:  identity
Node:             gke-camunda-benchmar-n2-standard-4-v2-f87032ef-t7zj/10.5.0.91
```


```
Non-terminated Pods:             (15 in total)
  Namespace                      Name                                                              CPU Requests  CPU Limits  Memory Requests  Memory Limits  Age
  ---------                      ----                                                              ------------  ----------  ---------------  -------------  ---
  c8-chaos-client-investigation  identity-888476b98-db4hd                                          600m (15%)    2 (51%)     400Mi (3%)       2Gi (15%)      13m
  c8-chaos-client-investigation  worker-6d55dd58d9-kcwhs                                           500m (12%)    500m (12%)  512Mi (3%)       512Mi (3%)     13m
  c8-jb-test                     identity-77f598cdfd-4htrj                                         600m (15%)    2 (51%)     400Mi (3%)       2Gi (15%)      37m
  c8-jb-test                     optimize-b4fdc54dc-rvj4f                                          600m (15%)    2 (51%)     1Gi (7%)         2Gi (15%)      37m
```

Cant delete the node L/

```
[10-09-2026 15:40:15 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 3s 
$ k delete node gke-camunda-benchmar-n2-standard-4-v2-f87032ef-t7zj
Error from server (Forbidden): nodes "gke-camunda-benchmar-n2-standard-4-v2-f87032ef-t7zj" is forbidden: User "christopher.zell" cannot delete resource "nodes" in API group "" at the cluster scope
Ask your Teleport admin to ensure that your Teleport role includes access to the nodes in "kubernetes_resources" field.
Check by running: kubectl auth can-i delete nodes/gke-camunda-benchmar-n2-standard-4-v2-f87032ef-t7zj
```


Deleting pods 


```

k [10-09-2026 15:43:18 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ k delete pod identity-888476b98-db4hd worker-6d55dd58d9-kcwhs
pod "identity-888476b98-db4hd" deleted from c8-chaos-client-investigation namespace
pod "worker-6d55dd58d9-kcwhs" deleted from c8-chaos-client-investigation namespace
```


```

[10-09-2026 15:43:50 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 3s 
$ kgpo
NAME                                READY   STATUS      RESTARTS   AGE
camunda-0                           1/1     Running     0          16m
camunda-1                           1/1     Running     0          16m
camunda-2                           1/1     Running     0          16m
connectors-7dfdb78784-6hspn         1/1     Running     0          16m
elasticsearch-es-masters-0          1/1     Running     0          17m
elasticsearch-es-masters-1          1/1     Running     0          17m
elasticsearch-es-masters-2          1/1     Running     0          17m
identity-888476b98-2bh9s            0/1     Running     0          12s
leader-balancer-29817450-bnvkm      0/1     Completed   0          13m
leader-balancer-29817460-qthn5      0/1     Completed   0          3m59s
metrics-exporter-599745b466-gdskh   1/1     Running     0          17m
optimize-6b4bc8d797-lkp5h           1/1     Running     0          16m
postgresql-keycloak-1               1/1     Running     0          16m
prom-els-exporter-d66964655-68fr9   1/1     Running     0          17m
starter-765bcf9474-qnnbs            1/1     Running     0          17m
worker-6d55dd58d9-6zfdx             1/1     Running     0          12s
worker-6d55dd58d9-bksbh             1/1     Running     0          17m
worker-6d55dd58d9-rpxg4             1/1     Running     0          17m
```


https://console.cloud.google.com/logs/query;duration=PT10M;query=resource.labels.cluster_name%3D%22camunda-benchmark%22%0A--resource.labels.namespace_name%3D%22c8-chaos-client-investigation%22%0Aresource.labels.namespace_name%3D%22keycloak-operator%22%0A-resource.labels.container_name%3D%22keycloak-operator%22%0Aresource.labels.pod_name%3D%22c8-chaos-client-investigation-0%22%0AjsonPayload.loggerName%3D%22org.keycloak.http.access-log%22;summaryFields=:false:32:beginning?project=camunda-benchmark

Key cloak logs


```
[10-09-2026 15:48:26 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ kgpo
NAME                                                         READY   STATUS      RESTARTS   AGE
camunda-0                                                    1/1     Running     0          21m
camunda-1                                                    1/1     Running     0          21m
camunda-2                                                    1/1     Running     0          21m
connectors-7dfdb78784-6hspn                                  1/1     Running     0          21m
customer-notification-67bb8bbf4f-sdh9c                       1/1     Running     0          43s
dispute-process-request-get-vendor-info-5bd65ff5b6-mfphv     1/1     Running     0          43s
dispute-process-request-proof-from-vendor-7599799469-nw4sz   1/1     Running     0          43s
elasticsearch-es-masters-0                                   1/1     Running     0          21m
elasticsearch-es-masters-1                                   1/1     Running     0          21m
elasticsearch-es-masters-2                                   1/1     Running     0          21m
extract-data-from-document-5f7f8cc8d4-6rll8                  1/1     Running     0          43s
identity-888476b98-6mx7g                                     1/1     Running     0          2m37s
inform-about-successful-claim-7fd8579fd4-rstql               1/1     Running     0          43s
leader-balancer-29817450-bnvkm                               0/1     Completed   0          18m
leader-balancer-29817460-qthn5                               0/1     Completed   0          8m28s
metrics-exporter-599745b466-gdskh                            1/1     Running     0          21m
optimize-6b4bc8d797-lkp5h                                    1/1     Running     0          21m
postgresql-keycloak-1                                        1/1     Running     0          20m
prom-els-exporter-d66964655-68fr9                            1/1     Running     0          21m
refunding-777dbf8447-5wkgr                                   1/1     Running     0          42s
starter-7645bdc7-h62k6                                       1/1     Running     0          44s
```


Installed realistic work loiad to better identify the workers 



```
[10-09-2026 15:49:11 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ k delete pod -l app.kubernetes.io/name=load-tester
pod "customer-notification-67bb8bbf4f-sdh9c" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-get-vendor-info-5bd65ff5b6-mfphv" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-proof-from-vendor-7599799469-nw4sz" deleted from c8-chaos-client-investigation namespace
pod "extract-data-from-document-5f7f8cc8d4-6rll8" deleted from c8-chaos-client-investigation namespace
pod "inform-about-successful-claim-7fd8579fd4-rstql" deleted from c8-chaos-client-investigation namespace
pod "refunding-777dbf8447-5wkgr" deleted from c8-chaos-client-investigation namespace
pod "starter-7645bdc7-h62k6" deleted from c8-chaos-client-investigation namespace
[10-09-2026 15:49:49 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 4s 
$ ^C
[10-09-2026 15:49:50 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ k delete pod identity-888476b98-6mx7g 
pod "identity-888476b98-6mx7g" deleted from c8-chaos-client-investigation namespace
[10-09-2026 15:49:57 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation 
$ k delete pod -l app.kubernetes.io/name=load-tester
pod "customer-notification-67bb8bbf4f-jt8ll" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-get-vendor-info-5bd65ff5b6-vrpgv" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-proof-from-vendor-7599799469-rdprj" deleted from c8-chaos-client-investigation namespace
pod "extract-data-from-document-5f7f8cc8d4-lpkvv" deleted from c8-chaos-client-investigation namespace
pod "inform-about-successful-claim-7fd8579fd4-5l5jl" deleted from c8-chaos-client-investigation namespace
pod "refunding-777dbf8447-lmzrn" deleted from c8-chaos-client-investigation namespace
pod "starter-7645bdc7-4sd6x" deleted from c8-chaos-client-investigation namespace
```


Were not able to reproduce

We were checking for occurrencing from the day before

```
{
  "insertId": "z5fjpyq3k34j0xiv",
  "jsonPayload": {
    "message": "Application run failed",
    "exception": "org.springframework.beans.factory.BeanCreationException: Error creating bean with name 'worker': Invocation of init method failed\n\tat org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor.postProcessBeforeInitialization(InitDestroyAnnotationBeanPostProcessor.java:220)\n\tat org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.applyBeanPostProcessorsBeforeInitialization(AbstractAutowireCapableBeanFactory.java:426)\n\tat org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.initializeBean(AbstractAutowireCapableBeanFactory.java:1807)\n\tat org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.doCreateBean(AbstractAutowireCapableBeanFactory.java:603)\n\tat org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.createBean(AbstractAutowireCapableBeanFactory.java:525)\n\tat org.springframework.beans.factory.support.AbstractBeanFactory.lambda$doGetBean$0(AbstractBeanFactory.java:333)\n\tat org.springframework.beans.factory.support.DefaultSingletonBeanRegistry.getSingleton(DefaultSingletonBeanRegistry.java:371)\n\tat org.springframework.beans.factory.support.AbstractBeanFactory.doGetBean(AbstractBeanFactory.java:331)\n\tat org.springframework.beans.factory.support.AbstractBeanFactory.getBean(AbstractBeanFactory.java:196)\n\tat org.springframework.beans.factory.support.DefaultListableBeanFactory.instantiateSingleton(DefaultListableBeanFactory.java:1225)\n\tat org.springframework.beans.factory.support.DefaultListableBeanFactory.preInstantiateSingleton(DefaultListableBeanFactory.java:1191)\n\tat org.springframework.beans.factory.support.DefaultListableBeanFactory.preInstantiateSingletons(DefaultListableBeanFactory.java:1121)\n\tat org.springframework.context.support.AbstractApplicationContext.finishBeanFactoryInitialization(AbstractApplicationContext.java:994)\n\tat org.springframework.context.support.AbstractApplicationContext.refresh(AbstractApplicationContext.java:621)\n\tat org.springframework.boot.web.server.reactive.context.ReactiveWebServerApplicationContext.refresh(ReactiveWebServerApplicationContext.java:69)\n\tat org.springframework.boot.SpringApplication.refresh(SpringApplication.java:756)\n\tat org.springframework.boot.SpringApplication.refreshContext(SpringApplication.java:445)\n\tat org.springframework.boot.SpringApplication.run(SpringApplication.java:321)\n\tat org.springframework.boot.SpringApplication.run(SpringApplication.java:1365)\n\tat org.springframework.boot.SpringApplication.run(SpringApplication.java:1354)\n\tat io.camunda.zeebe.LoadTesterApplication.main(LoadTesterApplication.java:20)\nCaused by: java.lang.IllegalStateException: Failed to retrieve topology due to authentication error; check your config\n\tat io.camunda.zeebe.metrics.ConnectionMonitor.awaitAndPrintTopology(ConnectionMonitor.java:67)\n\tat io.camunda.zeebe.worker.Worker.awaitTopologyAndLogConfig(Worker.java:62)\n\tat java.base/jdk.internal.reflect.DirectMethodHandleAccessor.invoke(DirectMethodHandleAccessor.java:103)\n\tat java.base/java.lang.reflect.Method.invoke(Method.java:580)\n\tat org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor$LifecycleMethod.invoke(InitDestroyAnnotationBeanPostProcessor.java:453)\n\tat org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor$LifecycleMetadata.invokeInitMethods(InitDestroyAnnotationBeanPostProcessor.java:397)\n\tat org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor.postProcessBeforeInitialization(InitDestroyAnnotationBeanPostProcessor.java:217)\n\t... 20 more\nCaused by: io.camunda.client.api.command.ClientStatusException: Invalid bearer token\n\tat io.camunda.client.impl.CamundaClientFutureImpl.transformExecutionException(CamundaClientFutureImpl.java:121)\n\tat io.camunda.client.impl.CamundaClientFutureImpl.join(CamundaClientFutureImpl.java:53)\n\tat io.camunda.zeebe.metrics.ConnectionMonitor.awaitAndPrintTopology(ConnectionMonitor.java:50)\n\t... 26 more\nCaused by: java.util.concurrent.ExecutionException: io.grpc.StatusRuntimeException: UNAUTHENTICATED: Invalid bearer token\n\tat java.base/java.util.concurrent.CompletableFuture.reportGet(CompletableFuture.java:396)\n\tat java.base/java.util.concurrent.CompletableFuture.get(CompletableFuture.java:2073)\n\tat io.camunda.client.impl.CamundaClientFutureImpl.join(CamundaClientFutureImpl.java:51)\n\t... 27 more\nCaused by: io.grpc.StatusRuntimeException: UNAUTHENTICATED: Invalid bearer token\n\tat io.grpc.Status.asRuntimeException(Status.java:532)\n\tat io.grpc.stub.ClientCalls$StreamObserverToCallListenerAdapter.onClose(ClientCalls.java:581)\n\tat io.grpc.PartialForwardingClientCallListener.onClose(PartialForwardingClientCallListener.java:39)\n\tat io.grpc.ForwardingClientCallListener.onClose(ForwardingClientCallListener.java:23)\n\tat io.grpc.ForwardingClientCallListener$SimpleForwardingClientCallListener.onClose(ForwardingClientCallListener.java:40)\n\tat io.micrometer.core.instrument.binder.grpc.MetricCollectingClientCallListener.onClose(MetricCollectingClientCallListener.java:57)\n\tat io.grpc.internal.ClientCallImpl.closeObserver(ClientCallImpl.java:566)\n\tat io.grpc.internal.ClientCallImpl.access$100(ClientCallImpl.java:72)\n\tat io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInternal(ClientCallImpl.java:734)\n\tat io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInContext(ClientCallImpl.java:715)\n\tat io.grpc.internal.ContextRunnable.run(ContextRunnable.java:37)\n\tat io.grpc.internal.SerializingExecutor.run(SerializingExecutor.java:133)\n\tat java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1144)\n\tat java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:642)\n\tat java.base/java.lang.Thread.run(Thread.java:1583)\n",
    "threadContext": {
      "id": 1,
      "priority": 5,
      "name": "main"
    },
    "loggerName": "org.springframework.boot.SpringApplication",
    "serviceContext": {
      "service": "load-tester",
      "version": "c8-medic-2026-37-62bd53b8-stable-89"
    }
  },
  "resource": {
    "type": "k8s_container",
    "labels": {
      "container_name": "refunding",
      "cluster_name": "camunda-benchmark",
      "location": "europe-west1",
      "project_id": "camunda-benchmark",
      "pod_name": "refunding-6f46659ddf-n25zw",
      "namespace_name": "c8-medic-2026-37-62bd53b8-stable-89"
    }
  },
  "timestamp": "2026-09-09T15:42:24.744378150Z",
  "severity": "ERROR",
  "labels": {
    "compute.googleapis.com/resource_name": "gke-camunda-benchmar-n2-standard-4-v2-d2beeabd-79lr",
    "k8s-pod/app_kubernetes_io/component": "zeebe-client",
    "logging.gke.io/top_level_controller_name": "refunding",
    "k8s-pod/pod-template-hash": "6f46659ddf",
    "k8s-pod/topology_kubernetes_io/zone": "europe-west1-d",
    "logging.gke.io/top_level_controller_type": "Deployment",
    "k8s-pod/app": "refunding",
    "k8s-pod/topology_kubernetes_io/region": "europe-west1"
  },
  "logName": "projects/camunda-benchmark/logs/stdout",
  "sourceLocation": {
    "file": "SpringApplication.java",
    "line": "863",
    "function": "org.springframework.boot.SpringApplication.reportFailure"
  },
  "receiveTimestamp": "2026-09-09T15:42:28.591003294Z",
  "errorGroups": [
    {
      "id": "CNXbwKGr3ezipwE"
    }
  ]
}```


https://console.cloud.google.com/logs/query;query=resource.labels.cluster_name%3D%22camunda-benchmark%22%0Aresource.labels.namespace_name%3D%22c8-medic-2026-37-62bd53b8-stable-89%22%0A-resource.labels.container_name%3D%22orchestration%22%0A-resource.labels.container_name%3D%22optimize%22%0A-resource.labels.pod_name%3D%22postgresql-keycloak-1%22;pinnedLogId=2026-09-09T15:42:24.744378150Z%2Fz5fjpyq3k34j0xiv;summaryFields=resource%252Flabels%252Fpod_name,resource%252Flabels%252Fcontainer_name:false:32:beginning;cursorTimestamp=2026-09-09T15:42:24.744378150Z;startTime=2026-09-09T15:36:08.312Z;endTime=2026-09-09T15:56:08.312Z?project=camunda-benchmark


```
org.springframework.beans.factory.BeanCreationException: Error creating bean with name 'worker': Invocation of init method failed
	at org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor.postProcessBeforeInitialization(InitDestroyAnnotationBeanPostProcessor.java:220)
	at org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.applyBeanPostProcessorsBeforeInitialization(AbstractAutowireCapableBeanFactory.java:426)
	at org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.initializeBean(AbstractAutowireCapableBeanFactory.java:1807)
	at org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.doCreateBean(AbstractAutowireCapableBeanFactory.java:603)
	at org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.createBean(AbstractAutowireCapableBeanFactory.java:525)
	at org.springframework.beans.factory.support.AbstractBeanFactory.lambda$doGetBean$0(AbstractBeanFactory.java:333)
	at org.springframework.beans.factory.support.DefaultSingletonBeanRegistry.getSingleton(DefaultSingletonBeanRegistry.java:371)
	at org.springframework.beans.factory.support.AbstractBeanFactory.doGetBean(AbstractBeanFactory.java:331)
	at org.springframework.beans.factory.support.AbstractBeanFactory.getBean(AbstractBeanFactory.java:196)
	at org.springframework.beans.factory.support.DefaultListableBeanFactory.instantiateSingleton(DefaultListableBeanFactory.java:1225)
	at org.springframework.beans.factory.support.DefaultListableBeanFactory.preInstantiateSingleton(DefaultListableBeanFactory.java:1191)
	at org.springframework.beans.factory.support.DefaultListableBeanFactory.preInstantiateSingletons(DefaultListableBeanFactory.java:1121)
	at org.springframework.context.support.AbstractApplicationContext.finishBeanFactoryInitialization(AbstractApplicationContext.java:994)
	at org.springframework.context.support.AbstractApplicationContext.refresh(AbstractApplicationContext.java:621)
	at org.springframework.boot.web.server.reactive.context.ReactiveWebServerApplicationContext.refresh(ReactiveWebServerApplicationContext.java:69)
	at org.springframework.boot.SpringApplication.refresh(SpringApplication.java:756)
	at org.springframework.boot.SpringApplication.refreshContext(SpringApplication.java:445)
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:321)
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:1365)
	at org.springframework.boot.SpringApplication.run(SpringApplication.java:1354)
	at io.camunda.zeebe.LoadTesterApplication.main(LoadTesterApplication.java:20)
Caused by: java.lang.IllegalStateException: Failed to retrieve topology due to authentication error; check your config
	at io.camunda.zeebe.metrics.ConnectionMonitor.awaitAndPrintTopology(ConnectionMonitor.java:67)
	at io.camunda.zeebe.worker.Worker.awaitTopologyAndLogConfig(Worker.java:62)
	at java.base/jdk.internal.reflect.DirectMethodHandleAccessor.invoke(DirectMethodHandleAccessor.java:103)
	at java.base/java.lang.reflect.Method.invoke(Method.java:580)
	at org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor$LifecycleMethod.invoke(InitDestroyAnnotationBeanPostProcessor.java:453)
	at org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor$LifecycleMetadata.invokeInitMethods(InitDestroyAnnotationBeanPostProcessor.java:397)
	at org.springframework.beans.factory.annotation.InitDestroyAnnotationBeanPostProcessor.postProcessBeforeInitialization(InitDestroyAnnotationBeanPostProcessor.java:217)
	... 20 more
Caused by: io.camunda.client.api.command.ClientStatusException: Invalid bearer token
	at io.camunda.client.impl.CamundaClientFutureImpl.transformExecutionException(CamundaClientFutureImpl.java:121)
	at io.camunda.client.impl.CamundaClientFutureImpl.join(CamundaClientFutureImpl.java:53)
	at io.camunda.zeebe.metrics.ConnectionMonitor.awaitAndPrintTopology(ConnectionMonitor.java:50)
	... 26 more
Caused by: java.util.concurrent.ExecutionException: io.grpc.StatusRuntimeException: UNAUTHENTICATED: Invalid bearer token
	at java.base/java.util.concurrent.CompletableFuture.reportGet(CompletableFuture.java:396)
	at java.base/java.util.concurrent.CompletableFuture.get(CompletableFuture.java:2073)
	at io.camunda.client.impl.CamundaClientFutureImpl.join(CamundaClientFutureImpl.java:51)
	... 27 more
Caused by: io.grpc.StatusRuntimeException: UNAUTHENTICATED: Invalid bearer token
	at io.grpc.Status.asRuntimeException(Status.java:532)
	at io.grpc.stub.ClientCalls$StreamObserverToCallListenerAdapter.onClose(ClientCalls.java:581)
	at io.grpc.PartialForwardingClientCallListener.onClose(PartialForwardingClientCallListener.java:39)
	at io.grpc.ForwardingClientCallListener.onClose(ForwardingClientCallListener.java:23)
	at io.grpc.ForwardingClientCallListener$SimpleForwardingClientCallListener.onClose(ForwardingClientCallListener.java:40)
	at io.micrometer.core.instrument.binder.grpc.MetricCollectingClientCallListener.onClose(MetricCollectingClientCallListener.java:57)
	at io.grpc.internal.ClientCallImpl.closeObserver(ClientCallImpl.java:566)
	at io.grpc.internal.ClientCallImpl.access$100(ClientCallImpl.java:72)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInternal(ClientCallImpl.java:734)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInContext(ClientCallImpl.java:715)
	at io.grpc.internal.ContextRunnable.run(ContextRunnable.java:37)
	at io.grpc.internal.SerializingExecutor.run(SerializingExecutor.java:133)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1144)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:642)
	at java.base/java.lang.Thread.run(Thread.java:1583)
```

```
$ k delete pod -n keycloak-operator c8-chaos-client-investigation-0 
Error from server (Forbidden): pods "c8-chaos-client-investigation-0" is forbidden: User "christopher.kujawa@camunda.com" cannot delete resource "pods" in API group "" in the namespace "keycloak-operator"
```


```
^C[10-09-2026 16:15:51 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 3s 
$ k edit keycloak -n keycloak-operator c8-chaos-client-investigation
keycloak.k8s.keycloak.org/c8-chaos-client-investigation edited
[10-09-2026 16:16:14 +02:00]: load-tests/setup/c8-chaos-client-investigation main $⇣ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 20s 
$ k get pod -n keycloak-operator c8-chaos-client-investigation-0 -w
NAME                              READY   STATUS              RESTARTS   AGE
c8-chaos-client-investigation-0   0/1     ContainerCreating   0          1s
c8-chaos-client-investigation-0   0/1     ContainerCreating   0          1s
c8-chaos-client-investigation-0   0/1     Running             0          2s
```

```
[10-09-2026 16:14:17 +02:00]: ~ k8s:camunda-benchmark-prod:c8-chaos-client-investigation took 4s 
$ k delete pod -l app.kubernetes.io/name=load-tester
pod "customer-notification-67bb8bbf4f-vgnr2" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-get-vendor-info-5bd65ff5b6-m4s6d" deleted from c8-chaos-client-investigation namespace
pod "dispute-process-request-proof-from-vendor-7599799469-6ssvm" deleted from c8-chaos-client-investigation namespace
pod "extract-data-from-document-5f7f8cc8d4-8zfx7" deleted from c8-chaos-client-investigation namespace
pod "inform-about-successful-claim-7fd8579fd4-l74jt" deleted from c8-chaos-client-investigation namespace
pod "refunding-777dbf8447-n77v5" deleted from c8-chaos-client-investigation namespace
pod "starter-7645bdc7-mqjpw" deleted from c8-chaos-client-investigation namespace

```



After serveral retries and restarts

Failed while fetching credentials for clientId=orchestration: 



```
java.net.ConnectException: Connection refused
	at java.base/sun.nio.ch.Net.pollConnect(Native Method)
	at java.base/sun.nio.ch.Net.pollConnectNow(Net.java:694)
	at java.base/sun.nio.ch.NioSocketImpl.timedFinishConnect(NioSocketImpl.java:542)
	at java.base/sun.nio.ch.NioSocketImpl.connect(NioSocketImpl.java:592)
	at java.base/java.net.Socket.connect(Socket.java:751)
	at java.base/sun.net.NetworkClient.doConnect(NetworkClient.java:178)
	at java.base/sun.net.www.http.HttpClient.openServer(HttpClient.java:531)
	at java.base/sun.net.www.http.HttpClient.openServer(HttpClient.java:636)
	at java.base/sun.net.www.http.HttpClient.<init>(HttpClient.java:282)
	at java.base/sun.net.www.http.HttpClient.New(HttpClient.java:386)
	at java.base/sun.net.www.http.HttpClient.New(HttpClient.java:408)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.getNewHttpClient(HttpURLConnection.java:1324)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.plainConnect0(HttpURLConnection.java:1257)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.plainConnect(HttpURLConnection.java:1143)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.connect(HttpURLConnection.java:1072)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.getOutputStream0(HttpURLConnection.java:1474)
	at java.base/sun.net.www.protocol.http.HttpURLConnection.getOutputStream(HttpURLConnection.java:1437)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.doFetchCredentials(OAuthCredentialsProvider.java:369)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.fetchCredentials(OAuthCredentialsProvider.java:317)
	at io.camunda.client.impl.oauth.OAuthCredentialsCache.doForceRefreshIfChanged(OAuthCredentialsCache.java:250)
	at io.camunda.client.impl.oauth.OAuthCredentialsCache.forceRefreshIfChanged(OAuthCredentialsCache.java:233)
	at io.camunda.client.impl.oauth.OAuthCredentialsProvider.shouldRetryRequest(OAuthCredentialsProvider.java:205)
	at io.camunda.client.impl.RetriableStreamingFutureImpl.onError(RetriableStreamingFutureImpl.java:52)
	at io.grpc.stub.ClientCalls$StreamObserverToCallListenerAdapter.onClose(ClientCalls.java:581)
	at io.grpc.PartialForwardingClientCallListener.onClose(PartialForwardingClientCallListener.java:39)
	at io.grpc.ForwardingClientCallListener.onClose(ForwardingClientCallListener.java:23)
	at io.grpc.ForwardingClientCallListener$SimpleForwardingClientCallListener.onClose(ForwardingClientCallListener.java:40)
	at io.micrometer.core.instrument.binder.grpc.MetricCollectingClientCallListener.onClose(MetricCollectingClientCallListener.java:57)
	at io.grpc.internal.DelayedClientCall$DelayedListener$3.run(DelayedClientCall.java:561)
	at io.grpc.internal.DelayedClientCall$DelayedListener.delayOrExecute(DelayedClientCall.java:490)
	at io.grpc.internal.DelayedClientCall$DelayedListener.onClose(DelayedClientCall.java:545)
	at io.grpc.internal.ClientCallImpl.closeObserver(ClientCallImpl.java:566)
	at io.grpc.internal.ClientCallImpl.access$100(ClientCallImpl.java:72)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInternal(ClientCallImpl.java:734)
	at io.grpc.internal.ClientCallImpl$ClientStreamListenerImpl$1StreamClosed.runInContext(ClientCallImpl.java:715)
	at io.grpc.internal.ContextRunnable.run(ContextRunnable.java:37)
	at io.grpc.internal.SerializingExecutor.run(SerializingExecutor.java:133)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1144)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:642)
	at java.base/java.lang.Thread.run(Thread.java:1583)

```


https://console.cloud.google.com/logs/query;query=resource.labels.cluster_name%3D%22camunda-benchmark%22%0Aresource.labels.namespace_name%3D%22c8-chaos-client-investigation%22%0A-resource.labels.container_name%3D%22orchestration%22%0A-resource.labels.container_name%3D%22optimize%22%0A-resource.labels.pod_name%3D%22postgresql-keycloak-1%22%0Aseverity%3D%22ERROR%22;pinnedLogId=2026-09-10T14:26:58.738122893Z%2Fhqlwsnwicasv0zcx;summaryFields=resource%252Flabels%252Fpod_name,resource%252Flabels%252Fcontainer_name:false:32:beginning;cursorTimestamp=2026-09-10T14:26:58.738122893Z;duration=PT10M?project=camunda-benchmark


## Found Bugs

- Identity has NO metrics at all
- Clients have no metrics for: ouath refresh, token request, failure rate, etc.
