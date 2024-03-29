<div align="center">

# mlp

[![Go Report Card](https://goreportcard.com/badge/github.com/mia-platform/mlp)](https://goreportcard.com/report/github.com/mia-platform/mlp)
[![Test Build Docker image and Release](https://github.com/mia-platform/mlp/actions/workflows/test-build-docker-release.yml/badge.svg)](https://github.com/mia-platform/mlp/actions/workflows/test-build-docker-release.yml)
[![codecov](https://codecov.io/gh/mia-platform/mlp/branch/main/graph/badge.svg)](https://codecov.io/gh/mia-platform/mlp)

</div>


`mlp` is a command line tool responsible for creating, updating and deleting kubernetes resources based on files
generated by Mia-Platform Console.
The main subcommands that the tool has are:

- `interpolate`: fill placeholders in kubernetes files with the values of ENV variables
- `generate`: create files for kubernetes `ConfigMap` and `Secret` based on files and/or ENV values
- `deploy`: create and/or update resources in a kubernetes namespace with the intepolated/generated files
- `kustomize`: run kustomize build
- `hydrate`: helper to fill kustomization.yml with resources and patches
- `completion`: generate the autocompletion

## Install

### Using Brew

```
brew install mia-platform/tap/mlp
```

## Using Docker

```
docker run -it --rm ghcr.io/mia-platform/mlp
```

### Using Go

```
go get -u github.com/mia-platform/mlp
```

## Docs

If you want to read detailed documentation on all the available subcommands please refer to the [doc](./docs)

## Gitlab-CI usage

In order to use `mlp` in a pipeline these are the steps to follow:

- In the configuration files use `{{VAR}}` to reference the environment variable `VAR`
- Prepare a `mlp.yaml` file with all the secrets and configmaps to generate and apply inside the cluster (see file structure in [generate](./docs/40_generate.md)).
- run `mlp generate` with the flags:
  - `--config-file`: the configuration files containing secrets and configmaps structure.
  - `--env-prefix`: the prefixes to use while performing the environment variables lookup inside the configuration files.
  - `--out`: output directory in which generated files will be placed
- run `mlp interpolate` with:
  - `--filename`: the files/folders containing files to interpolate. The command does not look into sub-dirs.
  - `--env-prefix`: the prefixes to use while performing the environment variables lookup.
  - `--out`: output directory in which generated files will be placed
- run `mlp deploy` with:
  - `--filename`: the files/folders containing files to deploy into the Kubernetes cluster.
  - `--server`: Kubernetes URL
  - `--certificate-authority`: Kubernetes CA
  - `--token`: Kubernetes token
  - `--namespace`: the namespace in which the resources will be deployed
  - `--deploy-type`: the deploy type used
  - `--force-deploy-when-no-semver`: flag used to force deploy of services that are not following semantic versioning.
  - `--ensure-namespace` (default to `true`): set if the namespace existence should be ensured. By default it is set to true so that the namespace existence is checked and, if it not exists, created. If set to false, it throws if namespace not already exists.

Example:

The `script` section of the CI file should look like this:

```yaml
script:
  - mkdir OUTPUT_DIR
  - mlp generate -c config-file.yaml -e FIRST_PREFIX -e SECOND_PREFIX -o OUTPUT_DIR
  - mlp interpolate -f SOURCE_PATH -e FIRST_PREFIX -e SECOND_PREFIX -o OUTPUT_DIR
  - mlp deploy --server KUBERNETES_URL --certificate-authority /path/to/kubernetes/ca.pem --token KUBERNETES_TOKEN -f OUTPUT_DIR -n KUBERNETES_NAMESPACE --deploy-type DEPLOY_TYPE --force-deploy-when-no-semver=FORCE_DEPLOY_WHEN_NO_SEMVER
```

Note that `mlp` suppose that the output directory already exists so it needs to be created before using the command.

## Testing

To run the tests use the command:

```sh
make test
```

There are also some integration test, which you can run with

```sh
make test-integration
```

Before sending a PR be sure that all the linter pass with success:

```sh
make lint
```

## Contributing

Participation to the project is governed by the [Code of Conduct](./CODE_OF_CONDUCT.md) and you can read
how you can improve the project in the [Contributing](./CONTRIBUTING.md) guidelines.
