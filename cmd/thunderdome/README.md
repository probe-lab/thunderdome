# Thunderdome Client

thunderdome is a client tool for managing experiments

## Usage

Invoke with a command:

    thunderdome <command> <flags>...

Commands:

	deploy    Deploy an experiment
	teardown  Teardown an experiment
	status    Report on the operational status of an experiment
	image     Build a docker image for an experiment
	validate  Validate an experiment definition

See the [Experiment File Syntax](#experiment-file-syntax) section below for more details on how to create an experiment file.

### Credentials

To use the client you need to have deployer or admin rights in the AWS infrastructure. 
The user needs to be created in the Terraform and then an access key and secret can be generated in the AWS console.

Add the user and credentials to an AWS profile

Create an AWS profile for thunderdome by adding the following to `~/.aws/config`:

	[profile thunderdome]
	region = eu-west-1
	output = json

Add credentials to `~/.aws/credentials`:

	[thunderdome]
	aws_access_key_id=......
	aws_secret_access_key=....

Then ensure that the AWS_PROFILE environment variable is set when you invoke the `thunderdome` command line tool:

	AWS_PROFILE=thunderdome thunderdome deploy ...


### deploy

	thunderdome deploy [command options] EXPERIMENT-FILENAME

Deploy deploys the experiment defined by the supplied file. 
The `--duration/-d` option must be supplied, specifying how long the experiment should run, in minutes.

The steps the deploy takes are:

 1. reads the experiment file and determines a list of docker images that must be built or used for each target
 2. builds each image in turn and pushes them to the Thunderdome ECR docker repo
 3. creates an ECS task definition for each target and runs a task using it
 4. creates an SQS queue for the experiment and subscribes it to the gateway requests topic
 5. creates an ECS task definition for [dealgood](/cmd/dealgood/README.md) connecting it to the queue and runs a task
 6. registers the experiment with [ironbar](/cmd/ironbar/README.md) which will manage its termination

At this point the experiment will be running. 
A link to the Grafana dashboard for the experiment is logged.
Dealgood will pause for a few minutes to before sending requests to the targets so some charts will have a delay before populating.


**Note:** in the future the build and deployment of an experiment will be delegated to `ironbar`.

### teardown

	thunderdome deploy [command options] EXPERIMENT-FILENAME

Teardown stops an experiment and removes all resources (tasks, task definitions and queues) used.
It's only needed if you need to cancel an experiment part way through. 
`ironbar` will take care of shutting down an experiment at the end of it's configured duration.

### status

	thunderdome status [command options]

Status reports on the status of running or recently stopped experiments.
Without any options it prints a list of known experiments and whether they are stopped or not.
When an experiment name is specified with the `--experiment/-e` option it prints the status of the requested experiment, asking `ironbar` to perform a full check on the operational status of each resource used.

### validate

	thunderdome validate [command options] EXPERIMENT-FILENAME

Validate checks an experiment file for errors. 
It also prints the canonical version of the experiment, with the exact build steps for each target.

### image

The `image` command prepares docker images for use in experiments. The deploy command does this automatically but this command can be used to pre-build images for later use. Thunderdome expects images to be configured for the deployment environment and type of traffic sent by `dealgood`. This command wraps a base image in the necessary configuration to produce an image that can be used in Thunderdome.

It can use an existing published image as the base or build one from a git repository using the head of the repo, a specific commit, a tag or a branch. 

Specify the tag used for the created image using `--tag`. All image names are prefixed by `thunderdome:` so using `--tag=kubo-test` will result in an image named `thunderdome:kubo-test`.

The following options control the origin of the base image:

	--from-repo      Build the base image from this git repository
	--from-image     Use a published docker image as the base 

When building from a git repository, the following options control the commit to use. If none are specified then the default branch is used.

	--branch value   Switch to this branch
	--commit value   Checkout this commit
	--git-tag value  Checkout this tag

The Thunderdome image can be configured to maps environment variables to Kubo config options. The value of the option will be set to the value of the environment variable on start up. Two command line options control this, `--env-config` for numeric and boolean values, and `--env-config-quoted` for Kubo config options that require quoting such as strings and durations. Separate the name of the environment variable from the config option with a colon like this: `EnvVar:ConfigOption`.

	--env-config          Map an environment variable to a kubo config option
	--env-config-quoted   Quotes the mapped environment value

The following options add metadata to the Thunderdome image: 

	--maintainer     Email address of the maintainer
	--description    Human readable description of the image and its purpose

The `image` command can push the image to an AWS ECR repo. This assumes that you have the necessary `AWS_PROFILE` environment variable set to a profile that has permissions to push to the repo:

    --push-to        Push built image to this docker repo

#### Examples

Build an image from the head of the kubo Git repo, tag it as `kubo-test` and push to a container registry:

```sh
thunderdome image --from-repo=https://github.com/ipfs/kubo \
                  --tag kubo-test \
                  --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from commit `826c79c95` in the kubo Git repo, tag it as `kubo-826c79c95` and push to a container registry:

```sh
thunderdome image --from-repo=https://github.com/ipfs/kubo \
                  --commit 826c79c95 \
                  --tag kubo-826c79c95 \
                  --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from branch `paramtest` in the kubo Git repo, tag it as `kubo-paramtest`:

```sh
thunderdome image --from-repo=https://github.com/ipfs/kubo \
                  --branch paramtest \
                  --tag kubo-paramtest
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-v0.16.0` and push to a container registry:

```sh
thunderdome image --from-image ipfs/kubo:v0.16.0  \
                  --tag kubo-v0.16.0  \
                  --push-to 123456789.dkr.ecr.eu-west-1.amazonaws.com
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-highlow` and enable the `Swarm.ConnMgr.HighWater` and `Swarm.ConnMgr.LowWater` config options to be configured by environment variables `$CONNMGR_HIGHWATER` and `$CONNMGR_LOWWATER`:

```sh
thunderdome image --from-image ipfs/kubo:v0.16.0  \
                  --tag kubo-highlow  \
                  --env-config=CONNMGR_HIGHWATER:Swarm.ConnMgr.HighWater \
                  --env-config=CONNMGR_LOWWATER:Swarm.ConnMgr.LowWater
```

Build an image from the official v0.16.0 Kubo image, tag it as `kubo-reposize` and enable the `Datastore.StorageMax` config option to be configured by the `$STORAGEMAX` environment variable. Since this config option requires a string, we must use `--env-config-quoted`:

```sh
thunderdome image --from-image ipfs/kubo:v0.16.0  \
                  --tag kubo-reposize  \
                  --env-config-quoted=STORAGEMAX:Datastore.StorageMax 
```


## Experiment File Syntax

The experiment file is a JSON document that describes the setup for the experiment.

Use the `thunderdome validate FILENAME` command to validate a file. 
The command also expands each target's configuration, taking into account defaults and shared configuration.

### Name and Description

The following top level fields provide metadata about the experiment:

 - `name` - a short name for the experiment, it must contain only lowercase letters, numbers and hyphens and must start with a letter.
 - `description` - a free form description, used for documentation of the purpose of the experiment.

### Request Stream

The following top level fields configure the characteristics of the request stream sent to each target:

 - `max_request_rate` - the maximum number of requests per second to send to each target. Must be a positive integer.
 - `max_concurrency` - the maximum number of requests that may be in flight at any one time for each target. Must be a positive integer. This should be tuned to the expected capacity of the target. Each request increments the in-flight count, and each received response decreases it. If the configured maximum concurrency is reached, subsequent requests will be dropped.
 - `request_filter` - the type of filtering to apply on the stream of gateway logs. Valid values are:
   - `none` - no filtering is applied.
   - `pathonly` - only requests with a path prefix of `/ipfs` or `/ipns` will be sent to the target.
   - `validpathonly` - same filtering as `pathonly` but the path is also pre-parsed to ensure it is valid.

### Target Configuration

Targets are defined in the `targets` top level field, which takes an array of target definitions that describe how the docker image for the target should be built.

The following fields describe the target:

 - `name` (required) - a short name for the target. Like experiment names it must contain only lowercase letters, numbers and hyphens and must start with a letter.
 - `description` (optional) - a free-form description that will be included in the target's docker image.

These fields configure the docker image for the target. Only one of `use_image`, `base_image` or `build_from_git` can be supplied.

 - `use_image` (optional) - the pre-built docker image to use for this target. This overrides any `use_image` value set in the `defaults` section of the experiment. WARNING: only use images that have been built by thunderdome and contain the additional thunderdome configuration.
 - `base_image` (optional) - a docker image that will be used as a base. The build process wraps the docker image with an init section that configures it for use in thunderdome and executes any defined `init_commands`.
 - `build_from_git` (optional) - instructions for building an image contained in a Git repository. See below for details.
 - `init_commands` (optional) - a list of commands that will be run in the container at init time before the target daemon is executed. These override any specified in the `defaults` section of the experiment and are appended to any in the `shared` section, so they are executed in-order, after the shared commands. Each entry is a string containing a single command. Only one of `init_commands` or `init_commands_from` may be specified.
- `init_commands_from` (optional) -  a filename containing commands that will be run in the container at init time before the target daemon is executed. This overrides any `init_commands` or `init_commands_from` specified in the `defaults` section of the experiment and are appended to any in the `shared` section. Only one of `init_commands` or `init_commands_from` may be specified.


The following fields provide additional configuration for the target's execution environment:

 - `instance_type` (optional) - the type of instance to use. This overrides any instance type specified in the `defaults` section of the experiment. See [list of instance types](/tf/README.md#instance-types) for allowed values.
 - `environment`(optional) - a list of environment variables that will be passed to the container when it is executed. These override any environment specified in the `defaults` section of the experiment and are merged with any in the `shared` section, overwriting any entries with duplicate names. Each entry is specified as a JSON object with a `name` field and a `value` field. For example: `{ "name": "IPFS_PROFILE", "value": "server" }`.

### Target Defaults and Shared Configuration

The top-level `shared` field is used to specify configuration that is applied to all targets. It expects an object with the following fields:

 - `environment` (optional) - a list of environment variables that will be passed to the container when it is executed. These are merged with any defined by the target, with the target's taking precedent if there are any equal names. Each entry is specified as a JSON object with a `name` field and a `value` field.
 - `init_commands` (optional) - a list of commands that will be run in the container at init time before the target daemon is executed. These are merged with any defined by the target and are executed in-order, before the target's commands. Each entry is a string containing a single command. 
- `init_commands_from` (optional) -  a filename containing commands that will be run in the container at init time before the target daemon is executed. These are merged with any defined by the target and are executed in-order, before the target's commands. Only one of `init_commands` or `init_commands_from` may be specified.


The top-level `defaults` field is used to specify configuration that is applied to targets if they don't override it. It expects an object with the following fields:

 - `instance_type` (optional) - the type of instance to use. This is used as a fallback for any target that does not specify its own value.  See [list of instance types](/tf/README.md#instance-types) for allowed values.
 - `environment` (optional) - a list of environment variables that will be passed to the container when it is executed. These are ignored if the target defines any of its own, otherwise they are merged with any shared variables, taking precedent if there are any equal names. Each entry is specified as a JSON object with a `name` field and a `value` field.
 - `init_commands` (optional) - a list of commands that will be run in the container at init time before the target daemon is executed. These are ignored if the target defines any of its own, otherwise they are executed in-order, after the shared commands. Each entry is a string containing a single command. 
- `init_commands_from` (optional) -  a filename containing commands that will be run in the container at init time before the target daemon is executed. This is ignored if the target defines `init_commands` or `init_commands_from` of its own, otherwise the commands are executed in-order, after any shared commands. Only one of `init_commands` or `init_commands_from` may be specified.

Like targets, only one of `use_image`, `base_image` or `build_from_git` can be supplied as `defaults`:

 - `use_image` (optional) - the pre-built docker image to use if the target does not provide one and does not provide values for its `base_image` or `build_from_git` fields. WARNING: only use images that have been built by thunderdome and contain the additional thunderdome configuration.
 - `base_image` (optional) - a docker image that will be used as a base if the target does not provide one. The build process wraps the docker image with an init section that configures it for use in thunderdome and executes any defined `init_commands`.
 - `build_from_git` (optional) - instructions for building an image contained in a Git repository that will be used if the target does not provide any. See below for details.

### Building from Git

Thunderdome can build a docker image from a Git repository. The `build_from_git` field expects an object with the following fields. Only one of `commit`, `tag` or `branch` can be specified:

 - `repo` (required) - the repository to clone
 - `commit` (optional) - the commit to checkout in the repository
 - `tag` (optional) -  the tag to checkout in the repository
 - `branch` (optional) -  the branch to switch to in the repository

### Experiment File Examples

The following simple experiment defines one target based on the `ipfs/kubo:v0.18.1` image, that will be sent up to 10 requests per second, keeping up to 100 in flight at any one time:

```json
{
	"name": "simple",
	"max_request_rate": 10,
	"max_concurrency": 100,
	"request_filter": "pathonly",

	"targets": [
		{
			"name": "first"
			"instance_type": "io_medium",
			"base_image": "ipfs/kubo:v0.18.1"
		}
	]
}
```

The following experiment compares a pre-release of Kubo with the previous released version. Up to 20 requests per second are sent and each instance is configured with the same Kubo settings:

```json
{
	"name": "kubo-release-19",
	"max_request_rate": 20,
	"max_concurrency": 100,
	"request_filter": "pathonly",

	"defaults": {
		"instance_type": "io_medium"
	},

	"shared": {
		"init_commands" : [
			"ipfs config --json AutoNAT '{\"ServiceMode\": \"disabled\"}'",
			"ipfs config --json Datastore.BloomFilterSize '268435456'",
			"ipfs config --json Datastore.StorageGCWatermark 90",
			"ipfs config --json Datastore.StorageMax '\"160GB\"'",
			"ipfs config --json Pubsub.StrictSignatureVerification false",
			"ipfs config --json Reprovider.Interval '\"0\"'",
			"ipfs config --json Swarm.ConnMgr.GracePeriod '\"2m\"'",
			"ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true",
			"ipfs config --json Experimental.AcceleratedDHTClient true",
			"ipfs config --json Experimental.StrategicProviding true"
		]
	},

	"targets": [
		{
			"name": "kubo190rc1",
			"description": "kubo 0.19.0-rc1",
			"build_from_git": {
				"repo": "https://github.com/ipfs/kubo.git",
				"tag":"v0.19.0-rc1"
			}
		},
		{
			"name": "kubo181",
			"description": "kubo 0.18.",
			"build_from_git": {
				"repo": "https://github.com/ipfs/kubo.git",
				"tag":"v0.18.1"
			}
		}
	]
}
```

The following experiment compares a version of Kubo operating in a desktop style setting with one operating in a server setting:

```json
{
	"name": "kubo-181-server-vs-desktop",
	"max_request_rate": 20,
	"max_concurrency": 100,
	"request_filter": "pathonly",

	"shared": {
		"init_commands" : [
			"ipfs config --json AutoNAT '{\"ServiceMode\": \"disabled\"}'",
			"ipfs config --json Datastore.BloomFilterSize '268435456'",
			"ipfs config --json Datastore.StorageGCWatermark 90",
			"ipfs config --json Datastore.StorageMax '\"160GB\"'",
			"ipfs config --json Pubsub.StrictSignatureVerification false",
			"ipfs config --json Swarm.ConnMgr.GracePeriod '\"2m\"'",
			"ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true",
			"ipfs config --json Experimental.StrategicProviding true"
		]
	},

	"targets": [
		{
			"name": "kubo181-desktop",
			"description": "kubo 0.18.1 configured as a desktop",
			"instance_type": "io_medium",
			"build_from_git": {
				"repo": "https://github.com/ipfs/kubo.git",
				"tag":"v0.18.1"
			},
			"init_commands" : [
			]
		},
		{
			"name": "kubo181-server",
			"description": "kubo 0.18.1 configured as a server",
			"instance_type": "io_large",
			"build_from_git": {
				"repo": "https://github.com/ipfs/kubo.git",
				"tag":"v0.18.1"
			},
			"init_commands" : [
				"ipfs config --json Experimental.AcceleratedDHTClient true",
				"ipfs config --json Swarm.ConnMgr.HighWater 900",
				"ipfs config --json Swarm.ConnMgr.LowWater 600"
			],
			"environment" : [
				{ "name": "IPFS_PROFILE", "value": "server" }
			]
		}
	]
}
```
