# Thunderdome 

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