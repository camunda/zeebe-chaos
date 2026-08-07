---
layout: posts
title: Impact of worker downtime on a realistic load test
date: 2026-08-06
categories:
  - chaos_experiment
tags:
  - availability
  - performance
authors:
- zell
- jon
---

# Chaos Day Summary

We picked this scenario because it had already happened to us once, by accident: a worker in one of our other load tests had gone down for a while, and when it came back, throughput looked fine while something underneath clearly was not. We never got to the bottom of it at the time. Today we reproduced the same shape of failure deliberately, with the time to actually dig in.

In today's Chaos Day, we simulated an extended outage of a job worker in our realistic "bank customer complaint/dispute handling" load test, to see how well the system recovers once the worker comes back. Running the experiment was the quick part. Explaining what we had measured took considerably longer, and ended in four bug reports.

**TL;DR:** We took a job worker down for 80 minutes. Every job type's own throughput recovered within minutes, which looked like a clean recovery, but the backlog of process instances stuck behind it kept growing for two more hours, because the affected step fans out 1 instance into 50 downstream jobs. Adding worker capacity and replicas on the downstream job type did not meaningfully fix it on its own. Tracing the code afterwards gave us a mechanism: job push hands newly created jobs straight to a worker and they never enter the queue the backlog sits in, so the backlog can only be drained by the polling path, which is competing for the very capacity push keeps consuming. On top of that architectural inversion, we found three separate defects in the Java client's polling path. Two of them explain what we measured; the third is a worse bug that we could only reproduce in a test, and the metrics say it never fired on the day. The analysis produced four issues: [camunda/camunda#59631](https://github.com/camunda/camunda/issues/59631) for the engine-side backlog blindness, and [camunda/camunda#59635](https://github.com/camunda/camunda/issues/59635) as the umbrella for the three client defects.

<!--truncate-->

## Chaos Experiment

![](realistic.png)

This is the process our load test drives end to end; we will come back to its two multi-instance activities further down.

We run our realistic load test in the `c8-chaos-w32` namespace. The setup: a 3-partition Zeebe cluster with a set of dedicated job worker Deployments (one per job type) driving the "Bank: Customer complaint/dispute handling" process, plus a `starter` Deployment continuously creating new process instances.

Each worker is configured independently in the [`camunda-load-tests-helm`](https://github.com/camunda/camunda-load-tests-helm) chart, via a values file that this scenario mirrors in-repo ([`load-tester-values-realistic-benchmark.yaml`](https://github.com/camunda/camunda/blob/53c704ef5e146d9c4867d1827a8597409ffd5bcc/load-tests/setup/scenarios/load-tester-values-realistic-benchmark.yaml)), for example:

```yaml
workers:
  customer-notification:
    replicas: 1
    capacity: 30
    jobType: "customer_notification"
    payloadPath: "bpmn/emptyPayload.json"
    completionDelay: 300ms
    message:
      name: "dispute_process_receive_documents"
      correlationVariable: "correlationKey"
```

Three of these knobs matter a lot later, so it is worth being precise about what they map to in the Java client ([`application.yaml`](https://github.com/camunda/camunda/blob/c0cbe225642e0002a3bce445aaaa9cafd394f269/load-tests/load-tester/src/main/resources/application.yaml#L57-L67)):

* `capacity` becomes `max-jobs-active`, the number of jobs a worker will accept concurrently.
* `completionDelay` is how long our handler sleeps before completing, simulating real work.
* the job `timeout` is a static `1800ms`, and worker pods run with `execution-threads: 10` by default ([worker profile](https://github.com/camunda/camunda/blob/c0cbe225642e0002a3bce445aaaa9cafd394f269/load-tests/load-tester/src/main/resources/application.yaml#L122-L128)).

### Expected

Before starting our experiment the following expectations were set when a worker goes down and comes back up:

1. Throughput goes back to the previous steady state, or higher.
2. Recovery takes at most as long as the outage did, ideally less.
3. Increasing worker thread count increases throughput.
4. Increasing worker capacity (batch size) increases throughput, up to a point where it starts to slow the system down instead.

### Actual

![](general.png)

#### The outage

At 09:43 CEST we scaled the `extract-data-from-document` worker Deployment down to 0 replicas, simulating a client-side outage. We kept it down for 80 minutes, scaling it back up at 11:04 CEST.

Here is the whole day at a glance, with every action we took marked directly on the chart:

![01-day-overview](01-day-overview.png)

The top panel is active process instances for the namespace; the bottom panel is jobs handled per second, broken down by job type. Active instances climb in a straight line for the whole 80-minute outage, then keep climbing for another two hours past the point the worker was already back up, peak around 13:00. Once we stop the starter just after 16:15 we are able to clean up the backlog quickly.

#### Recovery

Zooming into the outage window itself makes the mechanism obvious:

![02-outage-window](02-outage-window.png)

The moment `extract-data-from-document` goes to 0 replicas, every job type's handled rate drops to zero, not just the one whose worker we killed: `customer_notification`, `dispute_process_request_proof_from_vendor`, `dispute_process_request_get_vendor_info`, `refunding`, all of them. Every one of those job types sits downstream of the step we took out, so once nothing can get past it, nothing downstream has any work to do either. Active instances, meanwhile, starts climbing the instant the worker goes down, since the starter keeps creating new instances at 1/s and each one now has nowhere to go once it reaches that step.

Once the worker came back, every job type's handled rate jumped back up within seconds, which is exactly what would make this look like a clean recovery if all we were watching was a single aggregate number:

![03-fanout-cascade](03-fanout-cascade.png)

Look closer, though, and the job types split into two groups that behave nothing alike. The one
whose worker we actually killed is the well-behaved one.

![](extract-task.png)

`extract_data_from_document` maps 1:1 to root process instances, so its backlog was roughly 4,800 jobs, one for each instance that piled up (80 minutes of outage at one new instance per second). It drains that within about 10 minutes and settles straight back to its steady 1/s.

![extract-data-handled](extract-data-handled.png)


In contrast: `dispute_process_request_proof_from_vendor`, `dispute_process_request_get_vendor_info`, and `refunding` do the opposite.

![](realistic.png)

If we look at our process model again, we can see two multi-instance activities, which means each process instance creates many jobs for the tasks inside them. Both iterate over `disputeDetails.disputePositions`, and our load test's payload always sets that collection to 50 entries. One is the "Vendor fraud claim validation" subprocess, which contains `dispute_process_request_proof_from_vendor` and `dispute_process_request_get_vendor_info`. The other is the "Initiate credit and clawback action" call activity, which starts a `refundingProcess` instance per entry.

![](dispute-job.png)

The moment the earlier worker recovers its backlog, the following job workers jump into execution and then stay pinned near their ceiling for hours. That is the fan-out arriving: every process instance that clears `extract-data-from-document` produces exactly 1 `extract_data_from_document` job, but 50 each of `dispute_process_request_proof_from_vendor`, `dispute_process_request_get_vendor_info` and `refunding`. So the roughly 4,800 process instances that piled up during the outage were never a 4,800-job backlog. They were on the order of 720,000 jobs, about 150 per instance across three job types, all of which still have to complete before the root instances can finish.

This is also why we first had to accumulate more process instances before we saw any drain at all: the backlog only starts shrinking once creation of new work is outpaced by completion of the fanned-out work already in flight. Around 13:15 we can see that turn happen:

![04-active-process-instances](04-active-process-instances.png)

Continuing the experiment, we tried different approaches to speed up the draining of the backlog. We checked the Operate web app to see where most instances were currently stuck (we didn't capture a screenshot at the time) and noticed `dispute-process-request-proof-from-vendor` was the most affected, and focused there. As a first step, at 14:52 CEST, we increased its capacity from 60 to 100, then at 15:26 CEST scaled it out to 3 replicas instead of 1. That did change the number on the dashboard: the handled rate for that job type steps from roughly 90/s to roughly 230/s the moment the extra replicas come up, and later peaks near 290/s once the other workers are out of the way.

![scaling-explanation](scaling-explanation.png)

A 3x jump in handled jobs per second, and the backlog still did not drain. Two separate things were going on.

So we tried something blunter. If that one job type was the bottleneck, maybe every other worker was simply competing with it for cluster capacity, and taking them out of the picture would hand that capacity over. From 15:59 CEST we scaled almost every other worker to zero, `customer-notification`, `dispute-process-request-get-vendor-info`, `extract-data-from-document`, `refunding` and `inform-about-successful-claim`, and at 16:05 CEST pushed `dispute-process-request-proof-from-vendor` up to 5 replicas.

The first is that we should be careful about reading that 290/s as 290 jobs of forward progress per second. `jobHandled` is incremented when the handler method returns, not when the broker accepts the resulting command: [`JobRunnableFactoryImpl`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobRunnableFactoryImpl.java#L52-L66) runs the done-callback in a `finally` block, so a job whose deadline has already passed still runs, still returns, and still counts. Broker-side command rejections show that pressure building through exactly the window we were tuning in:

![job-timeouts](job-timeouts.png)

`JOB.TIME_OUT` rejections climb through the afternoon, from a baseline in the teens to around 50/s after 15:00 and a spike near 90/s at 16:05. A rejected `JOB.TIME_OUT` means the broker went to time a job out and found it had already moved on, so a rising rate of them means a rising number of jobs finishing right at their deadline boundary. The largest burst of the whole day, around 255/s, sits at 11:00, which is the moment the recovered worker first met its backlog.

From about 16:15 a second intent joins it: rejected `JOB.COMPLETE` commands, at 25 to 40/s. A rejected completion means a worker finished a job it no longer owned, because the broker had already timed it out and made it available again. That is the deadline overrun described in defect 3 below, caught in the act, and every one of those jobs was counted as handled on the client while contributing nothing. It is the difference between a throughput number and a progress number.

The second is that the backlog did not drain, it moved, and looking at the subprocess again explains why. Inside "Vendor fraud claim validation", `dispute_process_request_proof_from_vendor` is immediately followed by `dispute_process_request_get_vendor_info`. So every proof-from-vendor job we managed to complete created a get-vendor-info job for a worker we had just scaled to zero, and the queue shifted one task to the right rather than shrinking. That is the fan-out chain working exactly as designed, against us: with a multi-instance subprocess, giving one job type more capacity just relocates the pile to whatever follows it. We reverted the whole thing within ten minutes, back to a single replica everywhere by 16:08 CEST.

```
$ timedatectl; k scale deployment starter --replicas=0
               Local time: Thu 2026-08-06 16:15:39 CEST
                Time zone: Europe/Berlin (CEST, +0200)
System clock synchronized: yes
deployment.apps/starter scaled
```

Only scaling down the starter really freed up capacity for the backlog to drain. After scaling it down at 16:15 CEST, the backlog drained over the next ~40 minutes, hitting its floor once everything was processed at 16:55 CEST.

## Where this left us

The system is resilient to a worker outage in the sense that throughput recovers quickly once the worker comes back. However, the backlog of process instances that accumulated during the outage can continue to grow for hours after the worker has returned. Depending on the process, that can make things worse, as it did in our case, because of the fan-out in this particular process and the way job push interacts with the backlog. Simply increasing worker capacity or adding replicas does not effectively address this issue on its own. The root cause lies in both architectural decisions and client-side bugs, which we explore in detail below.

## Root cause

We picked this back up after the fact and traced the engine, gateway and client code, since "the backlog isn't shrinking while throughput looks fine" needed a mechanism, not just a description. We started from the metrics and a hypothesis that job push was being prioritised, then went looking for where that prioritisation actually lives.

It turned out to be two independent things stacked on top of each other. The first is architectural and is true of every Camunda cluster. The second is a set of client bugs that turn "the backlog drains slowly" into "the backlog never drains".

### Layer 1: push hands out jobs that never enter the backlog

[`BpmnJobActivationBehavior#publishWork`](https://github.com/camunda/camunda/blob/a79395b4d0af9a578aceb7f2f34e69371c302bea/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/bpmn/behavior/BpmnJobActivationBehavior.java#L78-L116) checks exactly one thing when a job becomes activatable: is there a live worker stream for this job type right now? If there is, the job is activated and pushed immediately. If there is not, the engine only records the job as `ACTIVATABLE` and notifies workers that work exists.

That check happens on job creation, on backoff-retry recurrence ([`JobRecurAfterBackoffProcessor`](https://github.com/camunda/camunda/blob/e931e33fcb21d14917f577defc1b00d03577a75d/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobRecurAfterBackoffProcessor.java#L59)), on incident resolution ([`IncidentResolveProcessor`](https://github.com/camunda/camunda/blob/fb6cfe1a4da708b955e880fcd5f3840082ac2e56/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/incident/IncidentResolveProcessor.java#L292)), and on a job failure that still has retries left ([`JobFailProcessor`](https://github.com/camunda/camunda/blob/41f13654c376d48a6e0bb3fce2db270e2103d058/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobFailProcessor.java#L150)). It never checks whether an older backlog of the same job type is already waiting.

The consequence is the whole story in one sentence: **a pushed job never enters the `ACTIVATABLE` pool at all**, so it never queues behind the backlog, because it is never in the same queue.

That lines up exactly with what we saw. Jobs created while `extract-data-from-document` was down had no stream to go to, so they landed in `ACTIVATABLE`. The moment the worker came back and registered its stream, every subsequently created job, from the ongoing new instance creation and from completing whatever backlog items did get through, was pushed straight out. The pre-existing backlog could only be served by the polling path, `ActivateJobs`, and polling was now competing for the exact capacity that push was continuously consuming.

![07-job-push-end-to-end](07-job-push-end-to-end.svg)

We filed this as [camunda/camunda#59631](https://github.com/camunda/camunda/issues/59631). It is related to, but more specific than, the existing [camunda/camunda#15730](https://github.com/camunda/camunda/issues/15730), which already flagged in general terms that polling backfills jobs created before any streams existed.

Worth noting for completeness: the polling path itself is not buggy in its ordering. [`DbJobState#forEachActivatableJobs`](https://github.com/camunda/camunda/blob/a40b238a4e9c431761cee7a25c8808aba7dd2004/zeebe/engine/src/main/java/io/camunda/zeebe/engine/state/instance/DbJobState.java#L452-L470) walks the backlog in three phases, highest priority first and oldest job key first within a priority band. If polling gets a turn, it drains the right jobs. The problem is how often it gets a turn.

### Layer 2: three defects in the client's polling path

We expected to find that push simply outraces polling for the worker's capacity. That is true, but it is not the interesting part, and on its own it would only slow the backlog down. What we actually found is that the polling path is far more fragile than the push path.

Worth knowing before the details: in a default modern Java client the two paths do not even share a transport. `DEFAULT_PREFER_REST_OVER_GRPC` is [`true`](https://github.com/camunda/camunda/blob/7965bc72ba24349c074921da8e699929b8d2042f/clients/java/src/main/java/io/camunda/client/impl/CamundaClientBuilderImpl.java#L95), so polling goes over REST while streaming is gRPC-only.

When streaming is enabled, both paths funnel activated jobs into one [`BlockingExecutor`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L38-L58), which wraps the handler thread pool in a semaphore sized by `maxJobsActive` ([`JobWorkerBuilderImpl`](https://github.com/camunda/camunda/blob/c4844344227ebbe3db3dc0b84ab4879607aab3c3/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerBuilderImpl.java#L243-L277)). Aggregate capacity is genuinely respected. The stream's `onNext` blocks on that semaphore, which stalls gRPC's inbound flow control, which makes the gateway's [`responseObserver.isReady()`](https://github.com/camunda/camunda/blob/85d6b556712c4be6f7ada0f98338b5654142b82f/zeebe/gateway-grpc/src/main/java/io/camunda/zeebe/gateway/impl/stream/StreamJobsHandler.java#L154-L160) go false, so a full worker gets no more pushes. There is no missing capacity bound. What is missing is any arbitration of *order*.

That starts with the semaphore itself. It is constructed as [`new Semaphore(maxActivate)`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L34), the single-argument form, which the [`Semaphore(int)` javadoc](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/Semaphore.html#%3Cinit%3E(int)) defines as creating a semaphore with a "nonfair fairness setting". Under that setting the [class documentation](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/Semaphore.html) states that "barging is permitted", meaning a thread arriving at [`tryAcquire`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L41) can be allocated a permit ahead of a thread that has been parked there waiting. The same documentation recommends initialising semaphores that guard resource access as *fair*, precisely so that no thread is starved out.

That is exactly the shape we have. A pushed job arrives fresh on every `onNext`, so it can barge past a poll-delivered job that is already waiting. On top of that ordering asymmetry sit three separate accounting bugs in the poll path.


![08-job-push-poll-mechanism](08-job-push-poll-mechanism.svg)


**Defect 1: the poll budget cannot see pushed jobs.** [`JobWorkerImpl`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L204-L221) tracks in-flight work in `remainingJobs`, but only poll responses increment it and only `handleJobFinished` decrements it. Pushed jobs route to [`handleStreamJobFinished`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L282-L284), which touches metrics only. So when the worker sizes its next request as `maxJobsActive - remainingJobs` ([L195](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L185-L202)), it asks for the full capacity even when push already holds every permit. The client systematically requests jobs it cannot accept, and the broker has already marked every one of them `ACTIVATED` with a running deadline before the client discovers it.

**Defect 2: rejected jobs are never given back, and polling stops for good.** A job that cannot get a permit within its timeout is dropped with a `RejectedExecutionException`, [logged, and forgotten](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L255-L272). Its runnable never runs, so `handleJobFinished` never fires, so its `+1` on `remainingJobs` is never returned. The counter ratchets upward permanently. This is cumulative, not a single catastrophic batch. A poll batch is sized to the free capacity the worker believes it has, so the front of a batch normally gets permits and only the tail gets rejected, leaking one job at a time. Once the accumulated leak exceeds [`activationThreshold`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L96-L98), which is 30% of `maxJobsActive` and so 19 jobs at the capacity of 60 we were running, [`shouldPoll`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L156-L158) is false forever, and [`onScheduledPoll`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L147-L154) simply returns without rescheduling. Nothing re-arms the loop.


**Defect 3: a poll batch is dispatched sequentially, blocking on the semaphore.** [`JobPollerImpl`](https://github.com/camunda/camunda/blob/0e583a991bf6c37331e325b0268ac49b57d2803b/clients/java/src/main/java/io/camunda/client/impl/worker/JobPollerImpl.java#L145-L161) does `jobs.forEach(jobConsumer)` and only afterwards reports the count. Each of those calls parks on `tryAcquire(timeout)`, so job N in a batch waits for jobs 1 to N-1 to each burn their full timeout first. Every one of those jobs had its `1800ms` deadline started when the broker built the batch. The tail of a large batch is therefore guaranteed to be past its deadline before it even starts, which means the handler runs and completes a job the broker has already timed out and possibly re-activated. That is duplicate execution, and it is the same symptom family as [camunda/camunda#42244](https://github.com/camunda/camunda/issues/42244).

All three are filed separately from the engine issue, since they are client bugs under the umbrella issue [camunda/camunda#59635](https://github.com/camunda/camunda/issues/59635):

* [camunda/camunda#59632](https://github.com/camunda/camunda/issues/59632), the blind poll budget.
* [camunda/camunda#59633](https://github.com/camunda/camunda/issues/59633), the counter leak that stops polling permanently.
* [camunda/camunda#59634](https://github.com/camunda/camunda/issues/59634), the sequential blocking dispatch.

### Why every recovery path leads back to the broken one

This is the part that turns a slowdown into a stall. Both failure modes recover into the polling path:

* A push that gets blocked at the gateway fails immediately, with no queueing or retry ([`RemoteStreamPusher`](https://github.com/camunda/camunda/blob/a0ee80db9873fc62be829e44b666d749c79a8d53/zeebe/transport/src/main/java/io/camunda/zeebe/transport/stream/impl/RemoteStreamPusher.java) documents itself as performing no retries of any kind), and the broker writes a [`YIELD`](https://github.com/camunda/camunda/blob/ff8dbe135d1523283ba1324ed42c98824150432d/zeebe/broker/src/main/java/io/camunda/zeebe/broker/jobstream/YieldingJobStreamErrorHandler.java#L20-L25) that puts the job back into `ACTIVATABLE`.
* A job that times out on the broker goes back to `ACTIVATABLE` too. [`JobTimeOutProcessor`](https://github.com/camunda/camunda/blob/a40b238a4e9c431761cee7a25c8808aba7dd2004/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobTimeOutProcessor.java#L61) only notifies workers, it does not push. That is deliberate, and was [changed on purpose](https://github.com/camunda/camunda/pull/46641) after the duplicate-completion problems in the issue above.

`ACTIVATABLE` is served by polling and nothing else. So once a worker has stopped polling, "the job goes back to the backlog for another attempt" is not a recovery, it is a dead letter. The result is a closed loop: activate, reject, wait out the deadline, time out, re-activate, which writes records on every lap and makes no forward progress.

### Which of these actually fired on the day

It is worth separating what the code makes possible from what we can show happened, because they are not the same set.

Defect 2 needs that `RejectedExecutionException` to fire repeatedly, and that turns out to be a much narrower condition than "this job missed its deadline". [`BlockingExecutor`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L39-L45) never receives a per-job deadline. It always waits the full configured `timeout`, counted from the moment `execute()` is invoked for that particular job. And the dispatch loop that calls it, [`jobs.forEach(jobConsumer)`](https://github.com/camunda/camunda/blob/0e583a991bf6c37331e325b0268ac49b57d2803b/clients/java/src/main/java/io/camunda/client/impl/worker/JobPollerImpl.java#L149), carries no notion of how much of a job's real deadline is already gone.

So there are two clocks, and they do not start together. Job N in a batch, when its turn finally comes, gets a fresh full 1800ms to acquire a permit, measured from that moment. The broker's deadline clock for that same job started much earlier, when the batch was built. A job can therefore be far past its real deadline, producing exactly the `JOB.TIME_OUT` and `JOB.COMPLETE` rejections we saw, while still acquiring its permit comfortably inside its own locally reset window. Nothing throws, nothing gets logged, nothing leaks.

The metrics say that is what happened here. On the single `dispute-process-request-proof-from-vendor` pod that ran from 11:04 to 14:52, the cumulative `jobActivated` minus `jobHandled` gap moves up and down all afternoon and touches **0** at 13:50 CEST. A leaked job is never handled, so any accumulated leak puts a permanent floor under that difference. A gap that returns to zero means the leak was zero. And poll traffic never stopped: `JOB_BATCH#ACTIVATE` requests to the gateway ran at 10 to 18/s through the afternoon, and were still at 4 to 8/s between 16:02 and 16:08, when five of the six job workers were scaled to zero and this one was doing nearly all of the polling.

So the stall we actually observed is explained by the architectural inversion plus defects 1 and 3, with defect 2 never triggering. Defect 2 is still a real bug, and it is the worst of the three when it does fire, but we found it by reading the code and reproduced it in a test, not by watching it happen.

### It gets worse with more workers and more job types

The single-worker view above understates the problem.

* **Replicas do not isolate.** [`RemoteStreamerImpl#pickStream`](https://github.com/camunda/camunda/blob/8f2a2fda80659787ed9437ff2d4ddec6cb251b27/zeebe/transport/src/main/java/io/camunda/zeebe/transport/stream/impl/RemoteStreamerImpl.java#L65-L83) shuffles the registered consumers and picks one at random. Every replica gets pushed to, so scaling out adds more workers competing for pushed jobs rather than giving you a clean spare that only drains the backlog. And if defect 2 does fire, it multiplies the number of workers that can stop polling instead of leaving one healthy.
* **Turning streaming off only helps if you do it everywhere.** Push eligibility is aggregated per job type across the cluster, so one opted-out replica changes nothing while a sibling still streams.
* **Job types compete inside the broker.** There is one stream processor actor per partition handling every job type's records, so push traffic for unrelated job types eats the same processing budget your backlog drain needs. This throttles the drain, it does not starve it, and it is worth keeping separate from the push-versus-poll problem because the fixes differ.
* **Job types also compete inside the client.** A single `CamundaClient` shares one handler thread pool across all its workers ([`CamundaClientImpl`](https://github.com/camunda/camunda/blob/d47275235bbc04f7b350f4931481d9e6bd1eafcf/clients/java/src/main/java/io/camunda/client/impl/CamundaClientImpl.java#L614-L652)). Each worker has its own semaphore, but they contend for the same threads, so one job type stuck in the reject-and-retry loop burns thread time its healthy siblings need.

## What you can actually do about it

**Detect.** All of these work today, without new instrumentation:

* Watch the gap between the client's [`jobActivated` and `jobHandled` counters](https://github.com/camunda/camunda/blob/dc62083c576e4acbc956e1abb068edc25fbae5d5/clients/java/src/main/java/io/camunda/client/impl/worker/metrics/MicrometerJobWorkerMetrics.java#L42-L49), and read it as a fill level rather than an error count. `jobActivated` is incremented [as soon as a job arrives](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L255-L258) and `jobHandled` only when its handler returns, so the cumulative difference is how many jobs the worker is holding right now, plus any it has dropped for good.

  Plot it as a rate and all of that disappears into scrape noise around zero, which is how we first looked at it and saw nothing:

  ![dropping-request-overtime](dropping-request-overtime.png)

  Plot the same two counters cumulatively and it becomes very legible:

  ![dropping-request-overtime-no-rate](dropping-request-overtime-no-rate.png)

  `dispute_process_request_proof_from_vendor` pins to 60 from 11:00 onwards, which is exactly its `maxJobsActive`. That worker was completely full, continuously, for five hours. When we raised the capacity at 14:52 the ceiling simply moved with it, to 100. A worker parked on its configured ceiling indefinitely is one that is permanently in the regime where the semaphore has nothing to hand out, which is the precondition for all three client defects, so this is a good thing to alert on.

  It is not, by itself, evidence that jobs were dropped: a held job and a dropped job look identical in that number. The gap only proves drops when it climbs *above* `maxJobsActive`, or when it fails to fall back toward zero after the work stops.

* Note what the pair does not catch at all. Because `jobHandled` counts handler returns and not accepted completions, a job that runs past its deadline and has its completion rejected is counted as handled. Wasted work looks identical to useful work in this metric, so pair it with the broker-side view below.
* Broker side, `zeebe_log_appender_record_appended_total{recordType="COMMAND_REJECTION"}` broken down by intent gives you the other half. Rising `JOB.TIME_OUT` rejections mean jobs are finishing right at their deadline boundary; rejected `JOB.COMPLETE` commands mean they finished past it and the work was thrown away. Neither is visible from the client's own counters.
* Alert on the client log line [`reached maximum capacity (maxJobsActive)`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L55-L58). It currently reads like a benign tuning hint. It is not.
* A worker that has hit defect 2 emits no `ActivateJobs` requests at all while still holding a healthy stream. Absence of poll traffic from a live worker is an unambiguous signature, and it is cheap to check: `zeebe_gateway_total_requests_total{requestType="JOB_BATCH#ACTIVATE"}` never went quiet for us, which is how we ruled defect 2 out of this run.
* Broker side: job timeout rate per type, and created-minus-completed for the job type. Backlog age per job type is not exposed anywhere today, which is a real gap.

**Prevent.**

* **Size `maxJobsActive` so that a full worker can still meet its deadlines**, rather than raising it reactively when things look slow.

  Picture the worker completely full: it has accepted `maxJobsActive` jobs and every thread is busy. A job at the back of that queue has to wait for everything ahead of it to finish first. The threads work in parallel, so the queue drains in rounds of `execution-threads` jobs, each round taking one handler duration:

  ```
  waitForTheLastJob = (maxJobsActive / executionThreads) x handlerDuration
  ```

  For our load test that is 60 jobs over 10 threads, so 6 rounds, and 6 x 300ms is 1800ms. Turn it around so it bounds the capacity instead, and you get:

  ```
  maxJobsActive < executionThreads x (timeout / handlerDuration)
                = 10 x (1800 / 300)
                = 60
  ```

  **Why the job `timeout` is what the wait has to fit inside.** That one configured value is quietly doing two different things. On the broker it is the job's deadline: once it expires the broker takes the job back and hands it to someone else. In the client it is *also* the longest a job will sit waiting for a free thread, because the same value is passed to the [`BlockingExecutor`](https://github.com/camunda/camunda/blob/c4844344227ebbe3db3dc0b84ab4879607aab3c3/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerBuilderImpl.java#L273) and used as its [semaphore acquire timeout](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L41).

  Both clocks start at the same moment, when the broker hands the batch out, and they are the same length, so they run down together. A job that spends its entire waiting allowance has therefore spent its entire deadline. At the instant it finally gets a thread it has already expired, and it runs anyway, and the completion is rejected at the end. Waiting the full timeout can never pay off, which is why the wait has to be a fraction of the timeout rather than equal to it.

  Which is exactly the trap we were in. At `maxJobsActive` of 60 we were sitting on the boundary, so the last job of a full batch expired at the moment it started. Raising it to 100 when the backlog would not drain made it worse rather than better: 100 jobs over 10 threads is 10 rounds, 10 x 300ms is 3000ms, so anything admitted past the first 60 was already dead before it began. Treat the number as a ceiling and leave room underneath it, especially with streaming enabled, where pushed jobs take permits from the same pool and the poll path gets less than this formula assumes.
* Keep the job `timeout` comfortably above your *worst-case* handler duration, not just the average. The bound above assumes every job takes about the same time, so a long tail quietly eats the waiting budget of every job queued behind it.

**Recover.**

* Restart the worker pods. The leaked counter is in-process state, so a restart is currently the only reliable reset for a worker that has stopped polling.
* Disabling streaming for the job type, on every client at once, removes the `BlockingExecutor` entirely, so there is nothing left to reject against.
* Pausing new instance creation is what finally cleared it for us. We stopped the starter at 16:15 and the queues were empty by roughly 16:50. We had also restarted every worker minutes before that, so this run cannot separate the two.

## Open Questions

* Would the backlog still balloon like this with a shorter outage, or does it need a large enough head start to outrun the polling path?
* Can the system recover without intervention under sustained load, once the three client defects are fixed? This is the interesting re-run, because it isolates the architectural layer from the bugs.
* Does the engine's record processing duration degrade as the backlog grows, or does it stay flat while only the queue in front of it gets longer? We did not have that panel open during the run, so we cannot say either way, and it matters for whether a large backlog is purely a queueing problem or also a processing-cost one.
* Does starting a cluster with more partitions upfront change backlog recovery time? More partitions mean more independent stream-processor actors, so it may raise the aggregate ceiling, but it does nothing about the push-versus-poll inversion, which is per-partition independent.

## Conclusion

Coming back online after a dependency outage is the easy part to verify: every job type's throughput bounced back within seconds. What is harder to see, and what we only caught because we happened to have the active process instances panel open next to it, is that the backlog kept growing underneath that healthy-looking throughput number for hours.

The mechanism turned out to have two layers. Architecturally, job push has no concept of a standing backlog: a pushed job never enters the queue the backlog lives in, so an outage-induced backlog can only be drained by a path that is competing for the capacity push keeps taking. That alone would make recovery slow. What kept it from catching up is the client's poll path, which requests work without counting what push already delivered and then dispatches each batch one blocking job at a time, so the tail of every batch burns its deadline before it runs and the completion is thrown away. A third defect, a capacity leak that stops a worker polling for good, is worse than either of those, and every recovery path in the system funnels straight into it. It simply did not fire on the day, which we only established afterwards by going back to the metrics.

The reminder for us is that "throughput recovered" is not "fully recovered", and that backlog age and active process instances deserve a panel next to throughput in every future chaos day. We will pick this back up in a follow-up experiment covering the open questions above.
