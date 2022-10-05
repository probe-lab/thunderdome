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

The `image` command prepares docker images for use in experiments. Thunderdome expects images to be configured for the deployment environment and type of traffic sent by `dealgood`. This command wraps a base image in the necessary configuration to produce an image that can be used in Thunderdome.

It can use an existing published image as the base or build one from a git repository using the head of the repo, a specific commit, a tag or a branch. 

Specify the tag used for the created image using `--tag`. All tags are prefixed by `thunderdome:` so using `--tag=kubo-test` will result in an image named `thunderdome:kubo-test`.

The following options control the origin of the base image:

	--from-repo      Build the base image from this git repository
	--from-image     Use a published docker image as the base 

When building from a git repository, the following options control the commit to use. If none are specified then the default branch is used.

	--branch value   Switch to this branch
	--commit value   Checkout this commit
	--git-tag value  Checkout this tag

The Thunderdome image may include a script that maps an environment variable to a Kubo config option. The value of the option will be set to the value of the environment variable on start up. Use `--env-config` to specify a mapping and repeat for each mapping required. Separate the name of the environment variable from the config option with a colon like this: `EnvVar:ConfigOption`.

The following options add metadata to the Thunderdome image: 

	--maintainer     Email address of the maintainer
	--description    Human readable description of the image and its purpose

The `image` command can push the image to an AWS ECR repo. This assumes that you have the necessary `AWS_PROFILE` environment variable set to a profile that has permissions to push to the repo:

    --push-to        Push built image to this docker repo

#### Examples

Build an image from the head of the kubo Git repo, tag it as `kubo-test` and push to a container registry:

```sh
ironbar image --from-repo=https://github.com/ipfs/kubo \
              --tag kubo-test \
              --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from commit `826c79c95` in the kubo Git repo, tag it as `kubo-826c79c95` and push to a container registry:

```sh
ironbar image --from-repo=https://github.com/ipfs/kubo \
              --commit 826c79c95 \
              --tag kubo-826c79c95 \
              --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from branch `paramtest` in the kubo Git repo, tag it as `kubo-paramtest`:

```sh
ironbar image --from-repo=https://github.com/ipfs/kubo \
              --branch paramtest \
              --tag kubo-paramtest
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-v0.16.0` and push to a container registry:

```sh
ironbar image --from-image ipfs/kubo:v0.16.0  \
              --tag kubo-v0.16.0  \
              --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-highlow` and enable the `Swarm.ConnMgr.HighWater` and `Swarm.ConnMgr.LowWater` config options to be configured by environment `$CONNMGR_HIGHWATER` and `$CONNMGR_LOWWATER` variables:

```sh
ironbar image --from-image ipfs/kubo:v0.16.0  \
              --tag kubo-highlow  \
              --env-config=CONNMGR_HIGHWATER:Swarm.ConnMgr.HighWater \
              --env-config=CONNMGR_LOWWATER:Swarm.ConnMgr.LowWater
```
