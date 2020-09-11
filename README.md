# Deploy

This repository contains the `miadeploy` CLI Tool used for deploying to a k8s cluster. This tool is responsible of interpolating the variables between {{}} in the input file with the actual content of the environment variables.

## Interpolate command

Usage: `miadeploy interpolate --prefix <main-prefix> --alternative-prefix <fallback-prefix> file_to_interpolate`

where:
- `--prefix`: specify the prefix to use when looking for the environment variable
- `--alternative-prefix`: prefix to use when the variable with the main prefix does not exists

Both the two flags are mandatory.

The `interpolate` command will search for all the strings between `{{}}` and will replace them with the corresponding environment variabile content. To find the variable the script will first use the `--prefix` in order to get the variable name and it will use the `--alternative-prefix` whenever the primary one has no environment variables associated to it. 
The `interpolate` command will not consume special characters preceeded by `\` to avoid errors when doing a release with the pipelines.
