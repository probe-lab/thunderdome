# Thunderdome 

Thunderdome is a system to compare the performance of different versions of IPFS gateways by using real HTTP traffic streamed from the public gateways operated by Protocol Labs.

## What does it do?

A user can define an experiment that details what software they want to put under test, with any special configuration and test conditions. Each combination of software and configuration is called a *target*. Thunderdome deploys each target, connects them to the IPFS network and begins sending real-world requests at the chosen rate. Each target in the experiment receives exactly the same request at the same time. 

Thunderdome collects metrics from each target and sends them to Grafana where they can be graphed and analysed. Once the experiment is done the deployed target are shut down to save resources, but they can be started once again if the experiment, or a variant, needs to be repeated. We use it to compare different implementations of the gateway spec, the impact of changing configuration settings or look for performance changes between releases.

Thunderdome differs from other tooling such as testground because it aims to simulate realistic load conditions using close to real time requests.

## What is in this repo?

This repository contains everything needed to setup and operate Thunderdome. 
The infrastructure is managed using Terraform and the tooling is written in Go.

### Thunderdome Client

The thunderdome client is in [/cmd/thunderdome](cmd/thunderdome/README.md). 
It's a command line utility that allows users to deploy and manage Thunderdome experiments.
See the [Thunderdome Client Documentation](cmd/thunderdome/README.md) for information on how to use the client and the [Experiment File Syntax](https://github.com/ipfs-shipyard/thunderdome/blob/main/cmd/thunderdome/README.md#experiment-file-syntax) to understand how to define an experiment.

### Thunderdome Infrastructure

The Terraform definition of the base infrastructure needed to run experiments is held in [/tf](tf/README.md). 
It can be used to set up a new Thunderdome environment from scratch and is also used to deploy upgraded versions of each tool.

Thunderdome uses three service components that are written in Go and deployed by Terraform:

 - [/cmd/skyfish](cmd/skyfish/README.md) - skyfish is responsible for transmitting requests from the Protocol Labs gateway infrastructure to Thunderdome. The logs are currently relayed from a sample of gateways to Grafana Loki and skyfish tails these logs and announces them in batches on an SNS topic. 
 - [/cmd/dealgood](cmd/dealgood/README.md) - dealgood is the component that sends requests to each target in an experiment. It reports metrics on the performance of the targets by measuring timings, numbers of requests sent and the types of response received. Each experiment deploys its own instance of dealgood. Dealgood receives requests via a dedicated SQS queue connected to the skyfish SNS topic.
 - [/cmd/ironbar](cmd/ironbar/README.md) - ironbar manages the shutdown of experiments. When an experiment is deployed by the thunderdome client a manifest of the deployed resources is sent to ironbar. After a defined lifetime ironbar will shut down the resources to terminate the experiment. One instance of ironbar manages all the running experiments.


![Diagram of Thunderdome Architecture](/architecture.png?raw=true "Thunderdome Architecture")

