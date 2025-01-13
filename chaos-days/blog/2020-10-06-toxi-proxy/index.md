---
layout: posts
title:  "Play around with ToxiProxy"
date:   2020-10-06
categories: 
  - chaos_experiment
  - toxiProxy
tags:
  - tools
authors: zell
---

# Chaos Day Summary

First chaos day since my parental leave :tada:.

Today I wanted to play a bit with [ToxiProxy](https://github.com/Shopify/toxiproxy). Toxiproxy is a framework for simulating network conditions and ideal to do some chaos on the network.

<!--truncate-->

## Run ToxiProxy

Download from the [release page](https://github.com/Shopify/toxiproxy/releases) the latest version (server and cli).

Start a broker via docker.

```sh
docker pull camunda/zeebe:SNAPSHOT
docker run -p 26500:26500 camunda/zeebe:SNAPSHOT
```

Start the toxi proxy server.

```sh
./toxiproxy-server-linux-amd64 start
```

Create a proxy for zeebe
```sh
./toxiproxy-cli-linux-amd64 create zeebe-proxy -l localhost:26379 -u localhost:26500
Created new proxy zeebe-proxy
```

You should see something in the toxy proxy server log:

```sh
INFO[0031] Started proxy                                 name=zeebe-proxy proxy=127.0.0.1:26379 upstream=localhost:26500
```

Try zbctl to request the topology.

```sh
./zbctl --address localhost:26379 status --insecure

Cluster size: 1
Partitions count: 1
Replication factor: 1
Gateway version: 0.25.0-SNAPSHOT
Brokers:
  Broker 0 - 172.17.0.2:26501
    Version: 0.25.0-SNAPSHOT
    Partition 1 : Leader
```

In the toxy proxy server log it should be shown as:

```sh
INFO[0149] Accepted client                               client=127.0.0.1:41510 name=zeebe-proxy proxy=127.0.0.1:26379 upstream=localhost:26500
WARN[0149] Source terminated                             bytes=245 err=read tcp 127.0.0.1:56178->127.0.0.1:26500: use of closed network connection name=zeebe-proxy
```

Add latency to requests

```sh
$ ./toxiproxy-cli-linux-amd64 toxic add -t latency -a latency=5000 zeebe-proxy
Added downstream latency toxic 'latency_downstream' on proxy 'zeebe-proxy'
```

Running zbctl again:

```sh
 ./zbctl --address localhost:26379 status --insecure
Error: rpc error: code = DeadlineExceeded desc = context deadline exceeded
```

Updating existing toxy:

```sh
./toxiproxy-cli-linux-amd64 toxic update -n latency_downstream -t latency -a latency=500 zeebe-proxy
```

Running zbctl again:

```sh
time ./zbctl --address localhost:26379 status --insecure
Cluster size: 1
Partitions count: 1
Replication factor: 1
Gateway version: 0.25.0-SNAPSHOT
Brokers:
  Broker 0 - 172.17.0.2:26501
    Version: 0.25.0-SNAPSHOT
    Partition 1 : Leader

real	0m1.045s
user	0m0.012s
sys	0m0.021s

```

Inspect existing toxics:

```sh
$ ./toxiproxy-cli-linux-amd64 inspect zeebe-proxy
Name: zeebe-proxy	Listen: 127.0.0.1:26379	Upstream: localhost:26500
======================================================================
Upstream toxics:
Proxy has no Upstream toxics enabled.

Downstream toxics:
latency_downstream:	type=latency	stream=downstream	toxicity=1.00	attributes=[	jitter=0	latency=500	]

```

With toxicity we can change whether it should be applied on all requests or only on some. It is possible to add the latency instead of downstream to upstream. There also other things we can inject, like slicing and delaying packages, dropping packages and limiting the bandwith.


Possible new experiments:

 * introduce latency between one follower and leader - if only one follower experience delays we expect that no election is started
 * introduce latency betweem gw and broker - see whether command timeout
 * slice packages - drop packages, but not every packages - expect that command is send correctly since requests are retried


### Slice packages

Slices packages after 128 bytes:
```sh
./toxiproxy-cli-linux-amd64 toxic add zeebe-proxy -t slicer -a average_size=128
```

Publish message seem to work:
```sh
$ time ./zbctl --address localhost:26379 publish message failing --insecure --correlationKey key --variables "{}"
{
  "key": 2251799813685253
}
```

After limiting it to 32 bytes:

```sh
$ ./toxiproxy-cli-linux-amd64 toxic update -n slicer_downstream -a average_size=32 zeebe-proxy
Updated toxic 'slicer_downstream' on proxy 'zeebe-proxy'
```

The publish message seem to not work as expected.

```sh
$ time ./zbctl --address localhost:26379 publish message failing --insecure --correlationKey key --variables "{}"
null

real	0m0.039s
user	0m0.007s
sys	0m0.023s
```

Actually I would expect here an error instead of just returning null.

## Chaos Experiment 

### No Leader change on high load

 Peter volunteered for automating a new chaos experiment, where we put high load on a broker and expect that we have no leader change. This was previous an issue, since the leaders were not able to send heartbeats in time. Related issue #7.

### Time reset
I wanted to work on the clock reset [#3](https://github.com/camunda/zeebe-chaos/issues/3).
This seems to be not easily possible in kubernetes or at least with our current images, since we need for that root privilges.

```sh
root@zell-time-reset-zeebe-0:/usr/local/zeebe# date -s $(date +%Y-%m-%dT%H:%M:%S)
date: cannot set date: Operation not permitted
Tue Oct  6 11:51:19 UTC 2020
```

It seems that chaos mesh supports something like that for kubernetes maybe worth to look at
https://pingcap.com/blog/simulating-clock-skew-in-k8s-without-affecting-other-containers-on-node


## Participants

  * @pihme
  * @zelldon


