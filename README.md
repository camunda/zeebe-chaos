# Zeebe Chaos

In order to build confidence in our system (Zeebe) and to find issues/problems/bugs before our users do,
we started to invest in a discipline so called `Chaos Engineering`. This repository contains everything
related to that, like chaos experiments, an hypotheses backlog etc.

We encourage our users to contribute to this project as much as possible,
under consideration our [contribution](CONTRIBUTING.md) guide and [code of conduct](CODE_OF_CONDUCT.md).

## Chaos Engineering

**Definition of Chaos Engineering:**

> Chaos Engineering is the discipline of experimenting on a system
> in order to build confidence in the system’s capability
> to withstand turbulent conditions in production.

This definition and everything related to that can be found under [principlesofchaos](https://principlesofchaos.org/?lang=ENcontent).

### Resources

Resources which you might find interesting regarding that topic.

 * https://principlesofchaos.org/?lang=ENcontent
 * https://www.oreilly.com/library/view/chaos-engineering/9781491988459/
 * https://docs.chaostoolkit.org/reference/concepts/
 * https://www.oreilly.com/library/view/learning-chaos-engineering/9781492050995/

## Chaos Experiments

In order to test or execute chaos experiments we use an tool called [chaos-toolkit](https://chaostoolkit.org/), which
makes it easy to create chaos experiments and also automate them later on.

All our current experiments are located under `chaos-days/blog/`, for more 
details please have a look at the [README](chaos-days/blog/README.md).

Alternatively all our chaos-days experiments can be found [here](https://camunda.github.io/zeebe-chaos/) in blog 
format.

