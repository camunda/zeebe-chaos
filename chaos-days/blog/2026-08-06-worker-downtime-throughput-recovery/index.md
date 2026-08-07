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

In today's Chaos Day, we simulated an extended outage of a job worker in our realistic "bank customer complaint/dispute handling" load test, to see how well the system recovers once the worker comes back.

**TL;DR:** We took a job worker down for 75 minutes. Export throughput recovered within minutes and even overshot, but the backlog of jobs created during the outage never drained, and the broker's p99 batch processing duration stayed 9x higher for the rest of the day. Adding worker capacity and replicas made it worse, not better. Tracing the code afterwards gave us a mechanism: job push hands newly created jobs straight to a worker and they never enter the queue the backlog sits in, so the backlog can only be drained by the polling path, which is competing for the very capacity push keeps consuming. On top of that architectural inversion, we found three separate defects in the Java client's polling path, one of which permanently disables polling on a worker after a single bad batch.

<!--truncate-->

## Chaos Experiment

We used our `c8-chaos-w32` namespace running the realistic benchmark load test: a 3-partition Zeebe cluster with a set of dedicated job worker Deployments (one per job type) driving the "Bank: Customer complaint/dispute handling" process, plus a `starter` Deployment continuously creating new process instances.

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

Note that `capacity` and threads are not the same thing, which we got wrong during the experiment itself.

### Expected

1. Throughput goes back to the previous steady state, or higher.
2. Recovery takes at most as long as the outage did, ideally less.
3. Increasing worker thread count increases throughput.
4. Increasing worker capacity (batch size) increases throughput, up to a point where it starts to slow the system down instead.

### Actual

#### The outage

Around 09:48 CEST, we scaled the `extract-data-from-document` worker Deployment down to 0 replicas, simulating a client-side outage. We kept it down for roughly 75 minutes before scaling it back up around 10:58 CEST.

Looking at the export throughput for the namespace over the whole day confirms the timeline:

![01-export-throughput](01-export-throughput.png)

Export throughput dropped from its ~7000-7300 records/s steady state to nearly zero for the whole outage window, then, within a few minutes of scaling the worker back up, shot up to almost 10,000 records/s as the broker caught up on the backlog, before settling into a new, noisier steady state around 7500-8500 records/s. By this measure alone, the system looked like it recovered well, even overshooting its previous throughput as expected.

#### Recovery... but something's off

Export throughput was not the only thing we had open, though. The count of active process instances tells a different story:

![02-active-process-instances](02-active-process-instances.png)

Active instances climb steadily through the whole outage, as the starter keeps creating new instances at 1/s while the ones already stuck behind `extract-data-from-document` cannot finish. That much is expected. What is not expected is that the count keeps climbing for hours after the worker came back, past 7000, well beyond where it sat before the outage, and it does not meaningfully come down again until we stop the starter entirely just after 16:15 and let the backlog drain to zero. The two capacity changes around 14:45-15:30 do not show up as a turning point on this chart either.

Looking more closely at the per-activity breakdown of current events at the time, we also noticed the distribution had shifted: a much larger share of activity was now going into things other than completing service tasks, and jobs for the affected process branch were being created faster than they were being completed. The backlog was not actually shrinking, it kept accumulating even though the headline export-throughput number looked recovered.

This raised an obvious question: if a client can be down for an hour in production without us noticing anything worse than "throughput looks fine", is our configuration actually able to sustain and absorb that kind of failure, or are we just looking at a metric that hides the real problem?

#### Chasing worker capacity

Our first hypothesis was that the `dispute-process-request-proof-from-vendor` worker's capacity was now a bottleneck, since it hadn't been sized for catching up on an hour of backlog. Looking at the process model made the two job types' relationship clear: they are not independent parallel branches, `extract-data-from-document` runs earlier in the same process instance, inside the "Document Request Process" subprocess, and only once that finishes does the instance reach the "Fraud Claim Investigation" subprocess where `dispute-process-request-proof-from-vendor` runs.

![03-process-model-job-types](03-process-model-job-types.svg)

That relationship matters: a backlog stuck in `extract-data-from-document` is invisible to `dispute-process-request-proof-from-vendor`, since none of those instances have reached it yet. The moment the upstream worker came back, that entire stuck backlog moved forward together and arrived at the vendor-proof stage as one burst, on top of the steady 1/s of freshly created instances. That looked exactly like a capacity problem on the second worker, so we did a quick calculation at the time that seemed to back it up, and increased the capacity from 60 to 100:

```
$ k edit deployments.apps dispute-process-request-proof-from-vendor
deployment.apps/dispute-process-request-proof-from-vendor edited
```

This caused a short dip in throughput (visible in the chart above, just before 15:00 CEST) as the pods rolled, then recovered. But it didn't meaningfully help the backlog. So we went further and scaled the worker out to 3 replicas instead of 1. That also didn't clearly fix things, throughput stayed in the same noisy 7500-8500 records/s band it had already settled into after the initial recovery.

With hindsight, that calculation was wrong in a way that matters, and we come back to it in the analysis below. Raising `capacity` without raising threads does not add throughput. It adds queue depth in front of a fixed number of threads, and every job sitting in that queue is already counting down its `1800ms` timeout.

#### The real anomaly: processing duration

While debugging, we pulled up the broker's own record processing duration and noticed that some events were taking noticeably longer to process than before the outage, at the p90/p99 level. That pointed away from the worker entirely and toward the stream processor itself.

Re-plotting the broker's p99 batch processing duration for the same day makes this much clearer than the throughput chart does:

![04-batch-processing-duration](04-batch-processing-duration.png)

Before the outage, p99 batch processing duration was a flat ~10ms. During the outage, it drops to the same baseline since there was almost nothing to process. But from the moment the worker came back up, it jumped to ~85-90ms, roughly nine times higher than before, and it stayed there. Neither the capacity increase nor the extra replicas moved this number at all; you can see both changes on the chart and neither one shows up as a dip in processing duration.

We do not think this is fully explained yet. The reject-and-timeout churn described in the root cause section below is our leading hypothesis, since it means every rejected job gets processed twice, once when the broker times it out and again when it is reactivated, which is extra record-processing work a clean run would not do. But we have not ruled out contributing factors on the state side: 7000+ concurrently active process instances is a much larger working set than steady state, and RocksDB iterator cost, overall column-family size, and the batching behaviour for the process's multi-instance elements could all independently make each batch more expensive to process, regardless of the push/poll mechanism. Separating these two effects needs instrumentation we did not have running on the day.

#### Draining the backlog

By late afternoon, we hadn't isolated the cause, and running out of time, we took a pragmatic path: stop generating new load entirely and let the system fully drain.

```
$ timedatectl; k scale deployment starter --replicas=0
               Local time: Thu 2026-08-06 16:15:39 CEST
                Time zone: Europe/Berlin (CEST, +0200)
System clock synchronized: yes
deployment.apps/starter scaled
```

All three charts show this clearly: active instances and throughput both taper off over the following 40 minutes as the remaining backlog is processed, hit their floor once everything is drained ("All processed at 2026-08-06T16:55:59+02:00"), then climb back to their original baseline once the starter is scaled back to 1 replica. Interestingly, the p99 batch processing duration also settles to a somewhat lower ~75-85ms after this full drain and restart, still nowhere near the original ~10ms.

## Root cause

We picked this back up after the fact and traced the engine, gateway and client code, since "the backlog isn't shrinking while throughput looks fine" needed a mechanism, not just a description. We started from the metrics and a hypothesis that job push was being prioritised, then went looking for where that prioritisation actually lives.

It turned out to be two independent things stacked on top of each other. The first is architectural and is true of every Camunda cluster. The second is a set of client bugs that turn "the backlog drains slowly" into "the backlog never drains".

### Layer 1: push hands out jobs that never enter the backlog

[`BpmnJobActivationBehavior#publishWork`](https://github.com/camunda/camunda/blob/a79395b4d0af9a578aceb7f2f34e69371c302bea/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/bpmn/behavior/BpmnJobActivationBehavior.java#L78-L116) checks exactly one thing when a job becomes activatable: is there a live worker stream for this job type right now? If there is, the job is activated and pushed immediately. If there is not, the engine only records the job as `ACTIVATABLE` and notifies workers that work exists.

That check happens on job creation, on backoff-retry recurrence ([`JobRecurAfterBackoffProcessor`](https://github.com/camunda/camunda/blob/e931e33fcb21d14917f577defc1b00d03577a75d/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobRecurAfterBackoffProcessor.java#L59)), on incident resolution ([`IncidentResolveProcessor`](https://github.com/camunda/camunda/blob/fb6cfe1a4da708b955e880fcd5f3840082ac2e56/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/incident/IncidentResolveProcessor.java#L292)), and on a job failure that still has retries left ([`JobFailProcessor`](https://github.com/camunda/camunda/blob/41f13654c376d48a6e0bb3fce2db270e2103d058/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobFailProcessor.java#L150)). It never checks whether an older backlog of the same job type is already waiting.

The consequence is the whole story in one sentence: **a pushed job never enters the `ACTIVATABLE` pool at all**, so it never queues behind the backlog, because it is never in the same queue.

That lines up exactly with what we saw. Jobs created while `extract-data-from-document` was down had no stream to go to, so they landed in `ACTIVATABLE`. The moment the worker came back and registered its stream, every subsequently created job, from the ongoing new instance creation and from completing whatever backlog items did get through, was pushed straight out. The pre-existing backlog could only be served by the polling path, `ActivateJobs`, and polling was now competing for the exact capacity that push was continuously consuming.

![05-job-push-end-to-end](05-job-push-end-to-end.svg)

We filed this as [camunda/camunda#59631](https://github.com/camunda/camunda/issues/59631). It is related to, but more specific than, the existing [camunda/camunda#15730](https://github.com/camunda/camunda/issues/15730), which already flagged in general terms that polling backfills jobs created before any streams existed.

Worth noting for completeness: the polling path itself is not buggy in its ordering. [`DbJobState#forEachActivatableJobs`](https://github.com/camunda/camunda/blob/a40b238a4e9c431761cee7a25c8808aba7dd2004/zeebe/engine/src/main/java/io/camunda/zeebe/engine/state/instance/DbJobState.java#L452-L470) walks the backlog in three phases, highest priority first and oldest job key first within a priority band. If polling gets a turn, it drains the right jobs. The problem is how often it gets a turn.

### Layer 2: three defects in the client's polling path

We expected to find that push simply outraces polling for the worker's capacity. That is true, but it is not the interesting part, and on its own it would only slow the backlog down. What we actually found is that the polling path is far more fragile than the push path.

Worth knowing before the details: in a default modern Java client the two paths do not even share a transport. `DEFAULT_PREFER_REST_OVER_GRPC` is [`true`](https://github.com/camunda/camunda/blob/7965bc72ba24349c074921da8e699929b8d2042f/clients/java/src/main/java/io/camunda/client/impl/CamundaClientBuilderImpl.java#L95), so polling goes over REST while streaming is gRPC-only.

When streaming is enabled, both paths funnel activated jobs into one [`BlockingExecutor`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L38-L58), which wraps the handler thread pool in a semaphore sized by `maxJobsActive` ([`JobWorkerBuilderImpl`](https://github.com/camunda/camunda/blob/c4844344227ebbe3db3dc0b84ab4879607aab3c3/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerBuilderImpl.java#L243-L277)). Aggregate capacity is genuinely respected. The stream's `onNext` blocks on that semaphore, which stalls gRPC's inbound flow control, which makes the gateway's [`responseObserver.isReady()`](https://github.com/camunda/camunda/blob/85d6b556712c4be6f7ada0f98338b5654142b82f/zeebe/gateway-grpc/src/main/java/io/camunda/zeebe/gateway/impl/stream/StreamJobsHandler.java#L154-L160) go false, so a full worker gets no more pushes. There is no missing capacity bound. What is missing is any arbitration of *order*.

That starts with the semaphore itself. It is constructed as [`new Semaphore(maxActivate)`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L34), the single-argument form, which the [`Semaphore(int)` javadoc](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/Semaphore.html#%3Cinit%3E(int)) defines as creating a semaphore with a "nonfair fairness setting". Under that setting the [class documentation](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/Semaphore.html) states that "barging is permitted", meaning a thread arriving at [`tryAcquire`](https://github.com/camunda/camunda/blob/051b1c8efee654694d03dd4dbce3652e939c0128/clients/java/src/main/java/io/camunda/client/impl/worker/BlockingExecutor.java#L41) can be allocated a permit ahead of a thread that has been parked there waiting. The same documentation recommends initialising semaphores that guard resource access as *fair*, precisely so that no thread is starved out.

That is exactly the shape we have. A pushed job arrives fresh on every `onNext`, so it can barge past a poll-delivered job that is already waiting. On top of that ordering asymmetry sit three separate accounting bugs in the poll path.

**Defect 1: the poll budget cannot see pushed jobs.** [`JobWorkerImpl`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L204-L221) tracks in-flight work in `remainingJobs`, but only poll responses increment it and only `handleJobFinished` decrements it. Pushed jobs route to [`handleStreamJobFinished`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L282-L284), which touches metrics only. So when the worker sizes its next request as `maxJobsActive - remainingJobs` ([L195](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L185-L202)), it asks for the full capacity even when push already holds every permit. The client systematically requests jobs it cannot accept, and the broker has already marked every one of them `ACTIVATED` with a running deadline before the client discovers it.

**Defect 2: rejected jobs are never given back, and polling stops for good.** A job that cannot get a permit within its timeout is dropped with a `RejectedExecutionException`, [logged, and forgotten](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L255-L272). Its runnable never runs, so `handleJobFinished` never fires, so its `+1` on `remainingJobs` is never returned. The counter ratchets upward permanently. Once the leak exceeds [`activationThreshold`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L96-L98), which is 30% of `maxJobsActive`, [`shouldPoll`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L156-L158) is false forever, and [`onScheduledPoll`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L147-L154) simply returns without rescheduling. Nothing re-arms the loop.

We wrote a throwaway test against an in-process gateway to check this rather than trust the reading. Capacity 4, streaming on, handler blocked so permits stay held, counting polls at the gateway:

```
polls after the stream opens ................. 2
polls after 4 pushed jobs take all permits ... 4
+5s, poll now returning 4 backlog jobs ....... 5 polls, 4 handlers run
+5s, after releasing ALL permits ............. 5 polls, 4 handlers run
```

One poll fetched four backlog jobs, all four were rejected, and the worker never polled again, with its entire capacity free and nothing pushing. That is a terminal state, not a race it keeps losing. It also explains why our mitigations failed: fresh pods poll, get poisoned within one poll round, and stop. Raising capacity only made the poison dose bigger.

**Defect 3: a poll batch is dispatched sequentially, blocking on the semaphore.** [`JobPollerImpl`](https://github.com/camunda/camunda/blob/0e583a991bf6c37331e325b0268ac49b57d2803b/clients/java/src/main/java/io/camunda/client/impl/worker/JobPollerImpl.java#L145-L161) does `jobs.forEach(jobConsumer)` and only afterwards reports the count. Each of those calls parks on `tryAcquire(timeout)`, so job N in a batch waits for jobs 1 to N-1 to each burn their full timeout first. Every one of those jobs had its `1800ms` deadline started when the broker built the batch. The tail of a large batch is therefore guaranteed to be past its deadline before it even starts, which means the handler runs and completes a job the broker has already timed out and possibly re-activated. That is duplicate execution, and it is the same symptom family as [camunda/camunda#42244](https://github.com/camunda/camunda/issues/42244).

![06-job-push-poll-mechanism](06-job-push-poll-mechanism.svg)

All three are filed separately from the engine issue, since they are client bugs rather than engine
behaviour, under the umbrella issue [camunda/camunda#59635](https://github.com/camunda/camunda/issues/59635):

* [camunda/camunda#59632](https://github.com/camunda/camunda/issues/59632), the blind poll budget.
* [camunda/camunda#59633](https://github.com/camunda/camunda/issues/59633), the counter leak that stops polling permanently.
* [camunda/camunda#59634](https://github.com/camunda/camunda/issues/59634), the sequential blocking dispatch.

### Why every recovery path leads back to the broken one

This is the part that turns a slowdown into a stall. Both failure modes recover into the polling path:

* A push that gets blocked at the gateway fails immediately, with no queueing or retry ([`RemoteStreamPusher`](https://github.com/camunda/camunda/blob/a0ee80db9873fc62be829e44b666d749c79a8d53/zeebe/transport/src/main/java/io/camunda/zeebe/transport/stream/impl/RemoteStreamPusher.java) documents itself as performing no retries of any kind), and the broker writes a [`YIELD`](https://github.com/camunda/camunda/blob/ff8dbe135d1523283ba1324ed42c98824150432d/zeebe/broker/src/main/java/io/camunda/zeebe/broker/jobstream/YieldingJobStreamErrorHandler.java#L20-L25) that puts the job back into `ACTIVATABLE`.
* A job that times out on the broker goes back to `ACTIVATABLE` too. [`JobTimeOutProcessor`](https://github.com/camunda/camunda/blob/a40b238a4e9c431761cee7a25c8808aba7dd2004/zeebe/engine/src/main/java/io/camunda/zeebe/engine/processing/job/JobTimeOutProcessor.java#L61) only notifies workers, it does not push. That is deliberate, and was [changed on purpose](https://github.com/camunda/camunda/pull/46641) after the duplicate-completion problems in the issue above.

`ACTIVATABLE` is served by polling and nothing else. So once a worker has stopped polling, "the job goes back to the backlog for another attempt" is not a recovery, it is a dead letter. The result is a closed loop: activate, reject, wait out the deadline, time out, re-activate, which writes records on every lap and makes no forward progress. That is also our best current hypothesis for the 9x jump in p99 batch processing duration, though we have not yet correlated it against a rejection count or job-timeout rate from the actual run.

### So does push always win?

Not always, and the honest answer depends on the regime:

* **Spare capacity is opening up continuously.** Push claims each freed slot the instant the record is processed, and the backlog gets whatever is left over. This is the worst case, and it is exactly the post-outage recovery regime we were testing.
* **The worker is genuinely saturated.** Push stops winning. The client stops reading, flow control closes, `isReady()` goes false, and the pushed job yields into `ACTIVATABLE`. New work now joins the backlog too, and everything depends on the polling path, which is the fragile one.

Both regimes disadvantage the backlog, for different reasons. And the asymmetry runs deeper than who gets the permit. The semaphore's non-fair ordering, described above, means push does not have to be first in line to win. Losing also costs the two paths very different amounts. A blocked push costs one failed attempt. A blocked poll costs a dropped job, a wasted broker activation, a burnt deadline, and a permanent decrement of that worker's willingness to poll ever again.

One thing we want to be careful not to overclaim: we have shown that push is served ahead of the backlog, and that the client defects turn slow draining into no draining. We have *not* shown that a fully fixed client would fail to drain under sustained load. It might just drain slowly.

### It gets worse with more workers and more job types

The single-worker view above understates the problem.

* **Replicas do not isolate.** [`RemoteStreamerImpl#pickStream`](https://github.com/camunda/camunda/blob/8f2a2fda80659787ed9437ff2d4ddec6cb251b27/zeebe/transport/src/main/java/io/camunda/zeebe/transport/stream/impl/RemoteStreamerImpl.java#L65-L83) shuffles the registered consumers and picks one at random. Every replica gets pushed to, so every replica gets independently poisoned. Scaling out multiplies the number of workers that stop polling. It does not give you a clean replica that keeps draining the backlog. This is why going from 1 to 3 replicas did not help us.
* **Turning streaming off only helps if you do it everywhere.** Push eligibility is aggregated per job type across the cluster, so one opted-out replica changes nothing while a sibling still streams.
* **Job types compete inside the broker.** There is one stream processor actor per partition handling every job type's records, so push traffic for unrelated job types eats the same processing budget your backlog drain needs. This throttles the drain, it does not starve it, and it is worth keeping separate from the push-versus-poll problem because the fixes differ.
* **Job types also compete inside the client.** A single `CamundaClient` shares one handler thread pool across all its workers ([`CamundaClientImpl`](https://github.com/camunda/camunda/blob/d47275235bbc04f7b350f4931481d9e6bd1eafcf/clients/java/src/main/java/io/camunda/client/impl/CamundaClientImpl.java#L614-L652)). Each worker has its own semaphore, but they contend for the same threads, so one job type stuck in the reject-and-retry loop burns thread time its healthy siblings need.

### Why raising capacity made things worse

Coming back to the calculation we got wrong on the day. `maxJobsActive` is not throughput. It is how many jobs the client will *accept*, and accepted jobs land in the unbounded queue of an [`Executors.newFixedThreadPool`](https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/util/concurrent/Executors.html#newFixedThreadPool(int)) ([`CamundaClientImpl`](https://github.com/camunda/camunda/blob/d47275235bbc04f7b350f4931481d9e6bd1eafcf/clients/java/src/main/java/io/camunda/client/impl/CamundaClientImpl.java#L636-L647)), in front of a fixed number of threads. With `execution-threads: 10` and a 300ms handler, a replica retires about 33 jobs/s. At `capacity: 60`, up to 50 accepted jobs sit in that queue, waiting about 1.5s before they start, against an `1800ms` deadline. At `capacity: 100`, 90 jobs queue, and the wait passes 2.7s, so the tail of every batch is guaranteed to expire before it runs.

The useful rule that falls out of this:

```
maxJobsActive  <  executionThreads  x  (jobTimeout / handlerDuration)
```

With our numbers that ceiling is `10 x (1800 / 300) = 60`. We were already at it, and we raised it to 100. Raising capacity without raising threads or the timeout does not buy throughput, it buys timeouts.

## What you can actually do about it

**Detect.** All of these work today, without new instrumentation:

* The clearest signal is a divergence between the client's [`jobActivated` and `jobHandled` counters](https://github.com/camunda/camunda/blob/dc62083c576e4acbc956e1abb068edc25fbae5d5/clients/java/src/main/java/io/camunda/client/impl/worker/metrics/MicrometerJobWorkerMetrics.java#L42-L49). `jobActivated` is incremented [as soon as a job arrives](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L255-L258), `jobHandled` only when its handler completes, so a sustained gap between them is literally the count of dropped jobs.
* Alert on the client log line [`reached maximum capacity (maxJobsActive)`](https://github.com/camunda/camunda/blob/42743d6fe90d8487d9a1f929f6e0d02981f60b3c/clients/java/src/main/java/io/camunda/client/impl/worker/JobWorkerImpl.java#L55-L58). It currently reads like a benign tuning hint. It is not.
* A worker that has hit defect 2 emits no `ActivateJobs` requests at all while still holding a healthy stream. Absence of poll traffic from a live worker is an unambiguous signature.
* Broker side: job timeout rate per type, and created-minus-completed for the job type. Backlog age per job type is not exposed anywhere today, which is a real gap.

**Prevent.**

* Size `maxJobsActive` with the inequality above rather than raising it when things look slow.
* Keep the job `timeout` comfortably above worst-case handler duration, and remember it doubles as the semaphore acquire timeout.

**Recover.**

* Restart the worker pods. The leaked counter is in-process state, so a restart is currently the only reliable reset for a worker that has stopped polling.
* Disabling streaming for the job type, on every client at once, removes the `BlockingExecutor` entirely, so there is nothing left to reject against.
* Pausing new instance creation works, but only in combination with fresh workers, which is what we did without realising why it was necessary.

## Open Questions

* Does the 9x jump in p99 batch processing duration come from the reject-and-timeout churn described above, from state-side costs of running with 7000+ concurrently active instances (RocksDB iterator cost, column-family size, multi-instance batching), from exporter-side back-pressure, or from some combination? We now have concrete mechanisms to test against instrumentation, where before we only had a symptom.
* Would the same jump happen with a shorter outage, or does it need a large enough backlog to trigger?
* Can the system recover without intervention under sustained load, once the three client defects are fixed? This is the interesting re-run, because it isolates the architectural layer from the bugs.
* Does starting a cluster with more partitions upfront change backlog recovery time? More partitions mean more independent stream-processor actors, so it may raise the aggregate ceiling, but it does nothing about the push-versus-poll inversion, which is per-partition independent.

## Conclusion

Coming back online after a dependency outage is the easy part to verify: throughput bounced back and even overshot within minutes. What is harder to see, and what we only caught because we happened to have the active process instances panel open next to it, is that the backlog kept growing underneath that healthy-looking throughput number for hours, and that the broker's own processing latency quietly settled into a state roughly nine times worse than before, with no amount of worker-side tuning touching either problem.

The mechanism turned out to have two layers. Architecturally, job push has no concept of a standing backlog: a pushed job never enters the queue the backlog lives in, so an outage-induced backlog can only be drained by a path that is competing for the capacity push keeps taking. That alone would make recovery slow. What made it never finish is a set of client-side accounting bugs, the worst of which permanently stops a worker from polling after a single bad batch, and which every recovery path in the system funnels straight into.

The reminder for us is that "throughput recovered" is not "fully recovered", and that backlog age and record processing duration deserve a panel next to throughput in every future chaos day. We will pick this back up in a follow-up experiment covering the open questions above.
