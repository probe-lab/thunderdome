# Thunderdome Infrastructure

This directory contains a Terraform definition of the infrastructure used by Thunderdome.

## Instance Types

The following instance types are currently defined for use by experiments. 
They are assigned using the `instance_type` field in the experiment file.

(Instance types correspond to ECS auti-scaling group capacity providers)

 - `compute_large`
   - EC2 instance type: c6id.8xlarge
   - 64GB RAM, 32 CPU, 12.5 Gigabit, ~$1.61 hourly
 - `compute_medium`
   - EC2 instance type: c6id.4xlarge
   - 32GB RAM, 16 CPU, Up to 12.5 Gigabit, ~$0.81 hourly
 - `compute_small`
   - EC2 instance type: c6id.2xlarge
   - 16GB RAM, 8 CPU, Up to 12.5 Gigabit, ~$0.40 hourly
 - `io_large`
   - EC2 instance type: i3en.2xlarge
   - 64GB RAM, 8 CPU, Up to 25 Gigabit, ~$0.62 hourly
 - `io_medium`
   - EC2 instance type: i3en.xlarge
   - 32GB RAM, 4 CPU, Up to 25 Gigabit, ~$0.31 hourly

## Setup

We use [asdf](https://asdf-vm.com/) to pin versions of the tools we are using. 

We use [direnv](https://direnv.net/)'s `dotenv` module to configure the
environment automatically when you enter the project folder . We set in `.env`
AWS profile / region info so that tooling such as the AWS cli works. Copy
`.env.example` to `.env` to enable it

## Terraform

### Formatting 

We format with `terraform fmt`, in vscode you can do it automatically with:

```json
  "[terraform]": {
    "editor.defaultFormatter": "hashicorp.terraform",
    "editor.formatOnSave": true,
    "editor.formatOnSaveMode": "file"
  },
```

### Usage

```
terraform init
```

```
terraform plan
```

```
terraform apply
```

As usual


### Grafana Agent Config

The Grafana agent sidecar is configured for targets and dealgood using separate config files held in an S3 bucket:

	http://pl-thunderdome-public.s3.amazonaws.com/grafana-agent-config/

The config files can be found in ./tf/files/grafana-agent-config


## Getting a console on a running container

ECS can inject an SSM agent into any running container so that you can
effectively "SSH" into it.

* Setup your credentials for an IAM user/role that has SSM permissions
* [Install AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
* [Install the Session Manager plugin for AWS CLI](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)
* Find the ECS task ID that you want to SSH into:
  - Log in to the AWS Console
  - Go to ECS
  - Select the eu-west-1 region
  - Select Clusters -> thunderdome
  - Select the Tasks tab
  - The Task ID is the UUID in the first column
* `export TASK_ID=<task_id> CONTAINER=gateway`
* `aws ecs execute-command --task $TASK_ID  --cluster thunderdome --container $CONTAINER --command '/bin/sh' --interactive`
