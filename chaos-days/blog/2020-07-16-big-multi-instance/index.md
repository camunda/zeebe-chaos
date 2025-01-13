---
layout: posts
title:  "Big Multi Instance"
date:   2020-07-16
categories: 
  - chaos_experiment 
  - bpmn 
authors: zell
---

# Chaos Day Summary

 * investigate and fix automated chaos experiments - works again with [88c404f](https://github.com/camunda/zeebe-chaos/commit/88c404f97514d4a7a511ce9751085acdd1720cd9) and [cd8d685](https://github.com/camunda/zeebe-chaos/commit/cd8d685b83eaa1ac9050ad3d16868389e1c0c36d)
 * Closed some issues in the backlog
 * Run a chaos experiment with bigger multi instance to reach `maxMessageSize`

<!--truncate-->

## Chaos Experiment

 We wanted to run a chaos experiment, which covers [#33](https://github.com/camunda/zeebe-chaos/issues/33).

### Expected

 If we reach `maxMessageSize` in a workflow instance, for example via variables we expect that an incident is created or at least an error event and we can see that in operate. Furthermore we expect that the partition processing is not stopped, which means we can still create new instances.

### Actual

 The experiment uses parallel multiInstance service tasks, to create a lot of tasks which should be completed with big variables.

 ![multiInstance](multiInstance.png)

 On collecting the output the `maxMessageSize` should be reached and we expected either an incident or exception for this instance. This should not affect other workflow instance creations.

 ![overview](overview.png)

 In operate we can see that we have two running workflow instances, one was started after the first failed. Later we created multiple instance's in a loop, without issues. This means we are not breaking partition processing, otherwise we would see timeouts.
 
 The problem we have is that we are not able to see in Operate that the workflow instance is actually broken. We have no indication for that.

 ![broken-multi](broken-multi.png)

 The instances seem still to be in starting the multi instance, but actually they are already blacklisted. If we check the logs we can find the following:

```
I 2020-07-16T13:05:20.630315Z Error event was committed, we continue with processing. 
E 2020-07-16T13:05:26.699139Z Expected to write one or more follow up events for event 'LoggedEvent [type=0, version=0, streamId=2, position=567, key=4503599627370769, timestamp=1594904720629, sourceEventPosition=565]' without errors, but exception was thrown. 
E 2020-07-16T13:05:26.735966Z Expected to process event 'TypedEventImpl{metadata=RecordMetadata{recordType=EVENT, intentValue=255, intent=ELEMENT_ACTIVATED, requestStreamId=-2147483648, requestId=-1, protocolVersion=1, valueType=WORKFLOW_INSTANCE, rejectionType=NULL_VAL, rejectionReason=}, value={"bpmnProcessId":"chaos","version":2,"workflowKey":2251799813685359,"workflowInstanceKey":4503599627370761,"elementId":"chaosTask","flowScopeKey":4503599627370769,"bpmnElementType":"SERVICE_TASK","parentWorkflowInstanceKey":-1,"parentElementInstanceKey":-1}}' without errors, but exception occurred with message 'Expected to claim segment of size 8439432, but can't claim more than 4194304 bytes.' . 
W 2020-07-16T13:05:26.737532Z Blacklist workflow instance 4503599627370761, due to previous errors. 
I 2020-07-16T13:05:26.738421Z Error record was written at 568, we will continue with processing if event was committed. Current commit position is 567. 
I 2020-07-16T13:05:26.793523Z Error event was committed, we continue with processing.

```

I created a feature request for operate [OPE-1037](https://jira.camunda.com/browse/OPE-1037)

In general chaos experiment succeeded, since we not breaking processing and we are still able to process other instances, but we only see that the instance is blacklisted in the logs not in operate, which is a problem from my POV. Furthermore a bit unexpected was, that we already failed before, we are not able to activate jobs, since the size of the multi instance was already to big.

### Code

```csharp
    // create zeebe client
    var client = ZeebeClient.Builder()
        .UseLoggerFactory(new NLogLoggerFactory())
        .UseGatewayAddress(ZeebeUrl)
        .UseTransportEncryption()
        .UseAccessTokenSupplier(
            CamundaCloudTokenProvider.Builder()
                .UseAuthServer(AuthServer)
                .UseClientId(ClientId)
                .UseClientSecret(ClientSecret)
                .UseAudience(Audience)
                .Build())
        .Build();

    var topology = await client.TopologyRequest().Send();

    Logger.Info(topology.ToString);
    Console.WriteLine(topology);
    // deploy
    var deployResponse = await client.NewDeployCommand()
        .AddResourceFile(DemoProcessPath)
        .Send();

    // create workflow instance
    var intArray = Enumerable.Range(0, 10_000).ToArray();
    var jsonObject = new {list = intArray};
    var jsonString = JsonConvert.SerializeObject(jsonObject);

    await client.NewCreateWorkflowInstanceCommand()
                .BpmnProcessId("chaos").LatestVersion()
                .Variables(jsonString)
                .Send();

    // open job worker
    using (var signal = new EventWaitHandle(false, EventResetMode.AutoReset))
    {
        client.NewWorker()
              .JobType(JobType)
              .Handler(HandleJob)
              .MaxJobsActive(120)
              .Name(WorkerName)
              .AutoCompletion()
              .PollInterval(TimeSpan.FromMilliseconds(100))
              .Timeout(TimeSpan.FromSeconds(10))
              .PollingTimeout(TimeSpan.FromSeconds(30))
              .Open();

        // blocks main thread, so that worker can run
        signal.WaitOne();
    }

private static void HandleJob(IJobClient jobClient, IJob job)
{
    Logger.Debug("Handle job {job}", job.Key);

    var bigPayload = File.ReadAllText(PayloadPath);
    jobClient.NewCompleteJobCommand(job).Variables(bigPayload).Send();
}
  
```

## Participants

 * @zelldon
