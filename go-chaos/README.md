# Zbchaos — A new fault injection tool for Zeebe

During Summer Hackdays 2022, I worked on a project called “Zeebe chaos” (zbchaos), a fault injection CLI tool. 
This allows us engineers to more easily run chaos experiments against Zeebe, build up confidence in the system’s capabilities, 
and discover potential weaknesses.

Read more about it on [medium.com](https://medium.com/@zelldon91/zbchaos-a-new-fault-injection-tool-for-zeebe-cbda56c5ba8d)

## Building the project

You can use the existing Makefile to build the project:

```
make build
```

## Run tests

You can use the existing Makefile to execute the tests, it will build the project before

```
make test
```

## Release

The release is fully automated with the corresponding `./release.sh` script.

You can run it locally or you use the corresponding github action https://github.com/camunda/zeebe-chaos/actions/workflows/release.yaml.
