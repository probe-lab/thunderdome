# ironbar

ironbar is a tool for managing experiments

## Usage

Invoke with a command:

    ironbar <command> <flags>...

Commands:

	status   Report on the operational status of an experiment
	deploy   Deploy an experiment
	image    Build a docker image for an experiment
	help     Shows a list of commands or help for one command


### deploy

TODO

### status

TODO


### image

The image command prepares docker images for use in experiments. It can build an new image from a git repository using the head of the repo, a specific commit, a tag or a branch. Pre-built images can also be used. Images are enhanced with the configuration needed to operate in Thunderdome.

Build an image from the head of the kubo Git repo, tag it as `kubo-test` and push to a container registry:

```shell
ironbar image --repo=https://github.com/ipfs/kubo -t kubo-test -p 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from commit `826c79c95` in the kubo Git repo, tag it as `kubo-826c79c95` and push to a container registry:

```shell
ironbar image --repo=https://github.com/ipfs/kubo --commit 826c79c95 -t kubo-826c79c95 -p 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from branch `paramtest` in the kubo Git repo, tag it as `kubo-paramtest`:

```shell
ironbar image --repo=https://github.com/ipfs/kubo --branch paramtest -t kubo-paramtest
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-v0.16.0` and push to a container registry:

```shell
ironbar image --from-image ipfs/kubo:v0.16.0 -t kubo-v0.16.0 -p 123456789.dkr.ecr.eu-west-1.amazonaws.com
```
