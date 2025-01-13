---
layout: posts
title:  "BPMN meets Chaos Engineering"
date:   2021-04-03
categories:
  - chaos_experiment
  - bpmn
tags:
  - tools
authors: zell
---

# BPMN meets Chaos Engineering

On the first of April (2021) we ran our Spring Hackday at Camunda. This is an event where the developers at camunda come together to work on projects they like or on new ideas/approaches they want to try out. This time we ([Philipp](https://github.com/saig0) and [me](https://github.com/zelldon)) wanted to orchestrate our Chaos Experiments with BPMN. If you already know how we automated our chaos experiments before, you can skip the next section
and jump directly to the [Hackday Project section](#hackday-project).

In order to understand this blogpost make sure that you have a little understanding of Zeebe, Camunda Cloud and Chaos Engineering. Read the following resources to get a better understanding.

 * [Get Started with Camund cloud](https://docs.camunda.io/docs/guides/)
 * [Quickstart Guide](https://docs.camunda.io/docs/product-manuals/clients/cli-client/get-started)
 * [Camunda Cloud](https://camunda.com/de/products/cloud/)
 * [Zeebe Process Engine](https://docs.camunda.io/docs/product-manuals/zeebe/zeebe-overview/)
 * [BPMN 2.0](https://www.omg.org/spec/BPMN/2.0/About-BPMN/)
 * [Principles of Chaos](https://principlesofchaos.org/)

<!--truncate-->

## Previous Chaos Automation 

In the previous Chaos Day summaries I described that we use [ChaosToolkit](https://chaostoolkit.org/) to run our chaos experiments. The chaos experiments have as prerequisite that an Zeebe cluster is already running, on which they should be executed. ChaosToolkit needs/uses a specific DSL to describe and execute Chaos Experiments. An example experiment looks like the following:

```json
{
    "version": "0.1.0",
    "title": "Zeebe follower restart non-graceful experiment",
    "description": "Zeebe should be fault-tolerant. Zeebe should be able to handle followers terminations.",
    "contributions": {
        "reliability": "high",
        "availability": "high"
    },
    "steady-state-hypothesis": {
        "title": "Zeebe is alive",
        "probes": [
            {
                "name": "All pods should be ready",
                "type": "probe",
                "tolerance": 0,
                "provider": {
                    "type": "process",
                    "path": "verify-readiness.sh",
                    "timeout": 900
                }
            },
            {
                "name": "Should be able to create workflow instances on partition 1",
                "type": "probe",
                "tolerance": 0,
                "provider": {
                    "type": "process",
                    "path": "verify-steady-state.sh",
                    "arguments": "1",
                    "timeout": 900
                }
            }
        ]
    },
    "method": [
        {
            "type": "action",
            "name": "Terminate follower of partition 1",
            "provider": {
                "type": "process",
                "path": "terminate-partition.sh",
                "arguments": [ "Follower", "1"]
            }
        }
    ],
    "rollbacks": []
}
```

This JSON describes a chaos experiment where a follower of partition one is terminated, and where we expect that we can create a new process instance before and after this termination. The follower termination should not affect our steady state.

In the JSON structure we can see the defined steady state of the Zeebe Cluster and the method/action which should be executed (the chaos which should be injected). The defined steady state is verified at the beginning of the experiment and at the end, after the methods are executed. The execution logic is quite simple. You can also define rollback actions, which should be executed if the experiment fails. Timeouts can be defined for each action and probe. Since the `chaosToolkit` is written in Python you can reference python modules/functions, which should be called during execution. Additionally, it supports bash scripts, which we normally use. Unfortunately bash scripts are sometimes not easy to understand and to maintain. This is one of the reason why we already thought more than once to replace the `chaosToolkit` with something different.

The `chaosToolkit` has more features and extensions, but these are not used by us. 

### List of Chaos Experiments

The experiment above is just one experiment of our continuous growing collection of chaos experiments, which we have already defined. There exist chaos experiments for the helm charts, but also for camunda cloud, for each cluster plan separately. You can find them [here](https://github.com/camunda/zeebe-chaos/tree/master/chaos-experiments).

### Automated Chaos Experiments

Chaos experiments need to be executed continously, not only once. For that we have build an automated pipeline, which runs the chaos experiments every night or if requested. We did that with help of the [Zeebe Cluster Testbench](https://github.com/zeebe-io/zeebe-cluster-testbench), we call it just `testbench`. The `testbench` creates for each cluster plan, in camunda cloud, a Zeebe cluster and runs the corresponding experiments against these clusters. The process model looks quite simple.

![chaos-test](chaos-test.png)

It is executed via a [zbctl chaos worker](https://github.com/zeebe-io/zeebe-cluster-testbench/tree/develop/core/chaos-workers), which is part of the `testbench`. The `chaos worker` polls for new jobs at the `testbench`. On new jobs it executes, based on the cluster plan, against the given/created Zeebe cluster the chaos experiments, via the `chaostoolkit`.

In general this was a good first solution, which is quite extensible since we just needed to add new experiments in the [zeebe-chaos](https://github.com/camunda/zeebe-chaos) repository and on the next run the experiments are executed, without any further adjustments. 

### Challenges

Still, we had some challenges with this approach. 

#### Additional Dependency

With the `chaosToolkit` we had an additional dependency. If you want to implement/write new chaos experiments you need to set up the `chaosToolkit` on your machine, with the correct Python etc. In general the setup guide was straight forward, but still it was something you need to have. It made the adoption harder.

#### Root Cause Analysis

Due to our setup it was a bit more challenging todo the root cause analysis.

![ChaosOutput](ChaosOutput.png)

We run a `zbctl` worker in a docker image, which picks up the `chaos` typed jobs. An `zbctl` worker will complete jobs with the output of the called handler script. This means that everything, which you want to log, needs to be logged in a separate file. The `chaosToolkit` will print its output into an own log file. The output of the bash scripts, which are executed by the `chaosToolkit`, will also end in that `chaosToolkit.log` file. I tried to visualize this a bit with the image above.

![chaos-test](failed-chaos-experiment-testbench.png)

If the chaos worker completes a job, the process instance in `testbench` is continued. If a chaos experiment fails, then the job is still completed normally, but with an error result. In the process instance execution this means that a different path is taken. The `testbench` will write a slack notification to a specific channel, such that the current Zeebe medic can look at it. 

![run-test](run-test-in-camunda-cloud.png)

After the notification the medic needs to find out which experiment has failed, this is part of the payload of the completed job at least, but he also needs to find out why the experiment failed. For this root cause analysis he has to check the log of the `chaostoolkit`, which is stored somewhere in the chaos worker pod (`data/chaostoolkit.log` it is an ever growing log).

## Hackday Project

With our Hackday Project we had two goals:

 1. lower the bar for team adoption 
 2. make root cause analysis easier
    
For that we wanted to replace `chaosToolkit` with a BPMN Process, which should be executed by Zeebe. We wanted to stick with the experiment description (the `chaosToolkit`/openchaos DSL) for our chaos experiments.

We modeled two processes. One root process, which reads for a given cluster plan all experiments and runs then each experiment. This is done via a [multi instance](https://docs.camunda.io/docs/0.25/product-manuals/zeebe/bpmn-workflows/multi-instance/multi-instance/) [call activity](https://docs.camunda.io/docs/reference/bpmn-workflows/call-activities/call-activities/).

![ChaosOutput](chaosToolkit.png)

The other process model is used for the real chaos experiment execution. As the `chaosToolkit` execution itself was quite simple, the resulting BPMN model is as well. All activities are
sequential multi instances, since we can have multiple probes/actions for the steady state, but also for the injection of chaos. On the root level of the process we have an interrupting [event sub process](https://docs.camunda.io/docs/reference/bpmn-workflows/event-subprocesses/event-subprocesses/) to timeout the chaos experiment if the experiment takes to long.

![ChaosExperiment](chaosExperiment.png)

As payload of the process instances we have the defined chaos experiment in JSON, which we have seen earlier. In this JSON we have all information we need to orchestrate this experiment.

We have implemented two Kotlin workers, one to read all experiment JSON files and one to execute the bash scripts, which are referenced in the chaos experiment descriptions. You can find the code [here](https://github.com/camunda/zeebe-chaos/tree/master/chaos-model/chaos-worker), it is just 100 lines long.

### Results

We orchestrated the follower/leader termination and graceful shutdown experiments via Camunda Cloud and the created process models. The experiments have been executed against another Zeebe cluster successfully.

![success](success-run.png)

To see how we improved the observability, we provoked an experiment to fail. Operate shows use via an incident that an experiment failed, and exactly at which step. 

![failure](fail-run.png)

We can even see the standard output and error output of the executed script in operate, without searching different log files.

![failuremsg](fail-run-output.png)

An timeout of an experiment will look like this:

![timeout](timeout-run.png)

As we can see the observability improved here a lot. We are able to understand why it failed based on the error message, since the complete error output is printed, and we can see the progress of the process instance on running the chaos experiment. 

With these models we were able to completely replace the `chaosToolkit` usage, so in the end we can remove an additional dependency.

### Further Work

Next step would be to integrate this in `testbench`, such that we can replace the old `chaos worker`.

Furthermore, we plan to replace step by step the new worker, which calls the scripts, by workers which have the script logic inside. For example with workers written in go or kotlin. This should improve the adoption and maintainability further.

For simplicity and to make progress we modeled quite generic process models, which are feed with the chaos experiment DSL. In the future we can also think of modeling the chaos experiments directly as BPMN model.

**Thanks to [Peter](https://github.com/pihme) for giving the impulse and his awesome work on `testbench` and [Philipp](https://github.com/saig0) which worked with me on this Hackday project.**


