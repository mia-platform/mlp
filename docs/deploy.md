# `deploy` Command

The `deploy` command deploys the specified files in a given namespace of a Kubernetes cluster.

Flags:
- `--filename`: file and/or folder paths containing data to interpolate
- `--deploy-type`: deploy type used 
- `--force-deploy-when-no-semver`: flag used to force deploy of services that are not following semantic versioning.

To make the command work, also the following flags described in [options](./options.md) are required:
- `--namespace`: to specify the namespace in which the resources are deployed
- The set of flags required to connect to the Kubernetes cluster
