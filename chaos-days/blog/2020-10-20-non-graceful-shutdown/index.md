---
layout: posts
title:  "Non-graceful Shutdown Broker"
date:   2020-10-20
categories: 
  - chaos_experiment 
  - broker
authors: zell
---

# Chaos Day Summary

Today I had not much time for the chaos day, because of writing Gameday Summary, Incident review, taking part of incidents etc. So enough chaos for one day :)

But I wanted to merge the PR from Peter and test how our brokers behave if they are not gracefully shutdown. 
I did that on Wednesday (21-10-2020).

<!--truncate-->

## PR Merge

I tried again the new chaos experiment with a Production M cluster, before merging. It worked quite smooth.
PR is merged [#41](https://github.com/camunda/zeebe-chaos/pull/41) :tada:

## Non-graceful shutdown

Currently in our experiments we do a normal `kubectl delete pod`, which does an graceful shutdown. The application has time to stop it's services etc. It would be interesting how Zeebe handles non-graceful shutdowns. In order to achieve that we can use the option `--grace-period=0`. For more information you can read for example [this](https://kubernetes.io/docs/tasks/run-application/force-delete-stateful-set-pod/#force-deletion)

I added additional experiments to our normal follower and leader restarts experiments, such that we have both graceful and non-graceful restarts.
Both seem to work without any issues. I was also able to fix some bash script error with the help of [shellcheck](https://github.com/koalaman/shellcheck). Related issue https://github.com/camunda/zeebe-chaos/issues/42.


Example output:

```
(chaostk) [zell kubernetes/ ns:f45d4dee-f73a-4733-9cd4-a4aa8b022376-zeebe]$ chaos run leader-terminate/experiment.json 
[2020-10-21 15:57:23 INFO] Validating the experiment's syntax
[2020-10-21 15:57:23 INFO] Experiment looks valid
[2020-10-21 15:57:23 INFO] Running experiment: Zeebe Leader restart non-graceful experiment
[2020-10-21 15:57:23 INFO] Steady-state strategy: default
[2020-10-21 15:57:23 INFO] Rollbacks strategy: default
[2020-10-21 15:57:23 INFO] Steady state hypothesis: Zeebe is alive
[2020-10-21 15:57:23 INFO] Probe: All pods should be ready
[2020-10-21 15:57:23 INFO] Probe: Should be able to create workflow instances on partition 3
[2020-10-21 15:57:27 INFO] Steady state hypothesis is met!
[2020-10-21 15:57:27 INFO] Playing your experiment's method now...
[2020-10-21 15:57:27 INFO] Action: Terminate leader of partition 3 non-gracefully
[2020-10-21 15:57:33 INFO] Steady state hypothesis: Zeebe is alive
[2020-10-21 15:57:33 INFO] Probe: All pods should be ready
[2020-10-21 15:58:28 INFO] Probe: Should be able to create workflow instances on partition 3
[2020-10-21 15:58:32 INFO] Steady state hypothesis is met!
[2020-10-21 15:58:32 INFO] Let's rollback...
[2020-10-21 15:58:32 INFO] No declared rollbacks, let's move on.
[2020-10-21 15:58:32 INFO] Experiment ended with status: completed
```

Related commits:

 * [Restart leader non-gracefully](https://github.com/camunda/zeebe-chaos/commit/e6260cb8612a983c8ed74fd2a37a249987ad3d3d)
 * [Restart follower non-gracefully](https://github.com/camunda/zeebe-chaos/commit/63c481c0c7dd7026f03be4e51d61a918613b0140)

## Participants

  * @zelldon


