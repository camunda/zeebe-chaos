---
layout: posts
title:  "Building Confidence at Scale: How Camunda Ensures Platform Reliability Through Continuous Testing"
date:   2025-12-11
categories: 
  - organizational
tags:
  - reliability
  - chaos
  - performance
authors: zell
---

# Building Confidence at Scale: How Camunda Ensures Platform Reliability Through Continuous Testing

As businesses increasingly rely on process automation for their critical operations, the question of reliability becomes paramount. How can you trust that your automation platform will perform consistently under pressure, recover gracefully from failures, and maintain performance over time?

At Camunda, we've been asking ourselves these same questions for years, and today I want to share how our reliability testing practices have evolved to ensure our platform meets the demanding requirements of enterprise-scale deployments. I will also outline our plans to further invest in this crucial area.

## From Humble Beginnings to Comprehensive Testing

Our reliability testing journey began in [early 2019](https://github.com/zeebe-io/zeebe-benchmark) with what we then called "benchmarks" – simple load tests to validate basic performance of our [Zeebe engine](https://docs.camunda.io/docs/components/zeebe/zeebe-overview/).

Over time, we recognized that running such benchmarks alone wasn't enough. We needed to ensure that Zeebe could handle real-world conditions, including failures and long-term operation. This realization led us to significantly expand our testing approach.

We introduced endurance tests that run for weeks, simulating sustained load to uncover memory leaks and performance degradation. These tests helped us validate that Zeebe could maintain its performance characteristics over extended periods of time. Investing in these endurance tests paid off, as we identified and resolved several critical issues that only manifested under prolonged load. Additionally, it allows us to build up experience on what a healthy system looks like and what we need to investigate faulty systems. With this, we were able to create [Grafana dashboards](https://github.com/camunda/camunda/tree/main/monitor/grafana) that we can directly use to monitor our production systems and provide to our customers.

We embraced [chaos engineering](https://principlesofchaos.org/) principles, developing a suite of chaos experiments to simulate failures in a controlled manner. We created [zbchaos](https://camunda.github.io/zeebe-chaos/), an open-source fault injection tool tailored for Camunda, allowing us to automate and scale our chaos experiments. Automated chaos experiments now run daily against all supported versions of Camunda, covering a wide range of failure scenarios.

Additionally, we run semi-regular manual "chaos days" where we design and execute new chaos experiments, documenting our findings in our [chaos engineering blog](https://camunda.github.io/zeebe-chaos/).

What started as a straightforward performance validation tool has evolved into a comprehensive framework that combines load testing, chaos engineering, and end-to-end testing. This evolution wasn't just about adding more tests. It reflected our growing understanding that reliability isn't a single metric but a multifaceted quality that emerges from systematic validation across different dimensions: performance under load, behavior during failures, and consistency over time.

We combine all of the above under the umbrella of what we now call "reliability testing." We define reliability testing as a type of software testing and practice that validates system performance and reliability. It can thus be done over time and with injection failure scenarios (injecting chaos).

If you are interested in more of the evolution of our reliability testing, I gave several Camunda Con Talks and wrote blog posts over the years that you might find interesting:

* [Camunda Con 2020.2: Chaos Engineering Meets Zeebe](https://page.camunda.com/recording-chaos-engineering-meets-zeebe)
* [Camunda Con 2024: Drinking our own Champagne: Chaos Experiments with Zeebe against Zeebe](https://vimeo.com/947050323/ce692173b3)
* [Drinking Our Champagne: Chaos Experiments with Zeebe against Zeebe](https://medium.com/@zelldon91/drinking-our-champagne-chaos-experiments-with-zeebe-against-zeebe-57632dd2c280)
* [Zbchaos — A new fault injection tool for Zeebe](https://medium.com/@zelldon91/zbchaos-a-new-fault-injection-tool-for-zeebe-cbda56c5ba8d)
* or see all the other [Chaos days](https://camunda.github.io/zeebe-chaos/) we have published

### Why Reliability Testing Matters

We prepare customers for enterprise-scale operations. For this, we need to be confident in building a product that is fault-tolerant, reliable, and that performs well even under turbulent conditions.

For our customers running mission-critical processes, reliability testing provides several crucial benefits:

* **Proactive Issue Detection**: We identify problems before they impact production environments. Memory leaks, performance degradation, and distributed system failures that only manifest under specific conditions are caught early in our testing cycles.
* **Confidence in Long-Term Operation**: Our endurance tests validate that Camunda can run fault-free over extended periods, ensuring your automated processes won't degrade over time.
* **Graceful Failure Handling**: Through chaos engineering, we verify that the platform handles failures elegantly, maintaining data consistency and recovering automatically when possible.
* **Performance Assurance**: Continuous load testing ensures that Camunda meets performance expectations (e.g., number of Process Instances / second), even as new features are added and the codebase evolves.

## Our Current Testing Arsenal

Today, our reliability testing encompasses two main pillars: load tests and chaos engineering.

### Variations of Load Tests

We run different variants of load tests continuously:

- **Release Endurance Tests**: Every supported version undergoes continuous endurance testing with artificial workloads, updated with each patch release
- **Weekly Endurance Tests**: Based on our main branch, these tests run for four weeks to detect newly introduced instabilities or performance regressions
- **Daily Stress Tests**: Shorter tests that validate the latest changes in our main branch under high load conditions

Our workload varies from artificial load (simple process definitions with minimal logic) to typical and realistic, complex processes that mimic real-world usage patterns.

Examples of such processes are:

![typical process](typical_process.png)

![complex process](complexProcess.png)

### Chaos Engineering

Since late 2019, we've embraced [chaos engineering principles](https://principlesofchaos.org/) to build confidence in our system's resilience. Our approach includes:

- **Chaos Days**: Regular events where we manually design and execute chaos experiments, documenting findings in our [chaos engineering blog](https://camunda.github.io/zeebe-chaos/)
- **Game Days**: Regular events where we simulate an incident in our production SaaS environment to validate our incident response processes
- **Automated Chaos Experiments**: Daily execution of 16 different chaos scenarios across all supported versions using our [zbchaos tool](https://camunda.com/blog/2022/09/zbchaos-a-new-fault-injection-tool-for-zeebe/). We [drink our own champagne](https://camunda.com/blog/2023/08/automate-chaos-experiments/) by using Camunda 8 to orchestrate our chaos experiments against Camunda.

## Investing in the Future

With the foundation we’ve established through years of focused reliability testing on the Zeebe engine and its distributed architecture, we’re now expanding that maturity across the entire Camunda product. Our goal is to develop an even more robust and trustworthy product overall. To achieve this, we are consolidating the reliability testing efforts that have historically existed across individual components into a centralized team. This unified approach enables us to scale our testing capabilities more efficiently, ensure consistent best practices, and share insights across teams, ultimately strengthening the reliability of every part of the product.

Some of our upcoming initiatives driven by this team include:

* **Holistic Coverage**: We're extending our reliability testing to cover all components of the Camunda 8 platform via a central reliability testing framework.
* **Chaos Engineering**: We're planning to introduce new chaos experiments that simulate more complex failure modes, including network partitions, data corruption, and cascading failures.
* **Performance Optimization**: Beyond maintaining performance, we utilize our testing infrastructure to identify optimization opportunities and validate improvements.
* **Enhanced Observability**: Building on our extensive [Grafana dashboards](https://github.com/camunda/camunda/tree/main/monitor/grafana), we continually improve our ability to detect and diagnose issues quickly.
* **Establish Reliability Practices**: We're formalizing reliability testing practices and guidelines that can be adopted across all engineering teams at Camunda.
* **Enablement**: With the resources we want to enable all of our more than 15 product teams at Camunda to understand, implement, and execute reliability testing principles in their work. Allowing them to build more reliable software from the start and scaling our efforts.

## Building Trust Through Transparency

Our commitment to reliability testing isn't just about internal quality assurance – it's about building trust with our customers and the broader community. That's why we:

- Publish our testing methodologies and results openly
- Share our learnings through [blog posts](https://camunda.github.io/zeebe-chaos/) and conference talks
- Provide tools like [zbchaos](https://github.com/camunda/zeebe-chaos) as open source for the community

## Conclusion

Reliability testing at Camunda has evolved from simple benchmarks to a comprehensive practice that combines load testing, chaos engineering, and end-to-end validation. This evolution reflects our understanding that true reliability emerges from systematic testing across multiple dimensions.

For our customers, this means confidence that Camunda will perform reliably under their most demanding workloads. For engineers interested in [joining our team](https://camunda.com/careers/), it represents an opportunity to work with cutting-edge testing practices at scale.

As we continue to invest in reliability testing, we remain committed to transparency and sharing our learnings with the community. After all, the reliability of process automation platforms isn't just a technical challenge – it's fundamental to the digital transformation of businesses worldwide.

---

*Interested in learning more about our reliability testing practices? Check out our [detailed documentation](https://github.com/camunda/camunda/blob/main/docs/testing/reliability-testing.md), explore our [chaos engineering experiments](https://camunda.github.io/zeebe-chaos/), or reach out to discuss how Camunda's reliability testing ensures your critical processes run smoothly.*