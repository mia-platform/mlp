# Kustomize

The `kustomize` subcommand builds a set of KRM resources using a `kustomization.yaml` file, equivalent to
running `kustomize build`. It can optionally inflate Helm charts as part of the build.

## Usage

```sh
mlp kustomize [DIR] [flags]
```

If `DIR` is omitted, the current directory is used.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `--output` | | Write output to the specified file path instead of stdout |
| `--load-restrictor` | `rootOnly` | Set the file loading restrictor: `rootOnly` or `none` |
| `--enable-helm` | `false` | Enable Helm chart inflation |
| `--helm-command` | `helm` | Path or name of the helm binary |
| `--helm-api-versions` | | Kubernetes API versions used for Helm `Capabilities.APIVersions` |
| `--helm-kube-version` | | Kubernetes version used for Helm `Capabilities.KubeVersion` |

## Examples

```sh
# Build the current working directory
mlp kustomize

# Build a specific path
mlp kustomize /home/config/project

# Save output to a file
mlp kustomize --output /home/config/build-results.yaml

# Build with Helm chart inflation
mlp kustomize --enable-helm

# Build with Helm using a specific binary and Kubernetes version
mlp kustomize --enable-helm --helm-command /usr/local/bin/helm --helm-kube-version 1.30.0
```

## Helm Support

When `--enable-helm` is set, `mlp kustomize` will inflate any Helm charts referenced in your
`kustomization.yaml` using the [Helm chart inflation generator](https://kubectl.docs.kubernetes.io/references/kustomize/builtins/#_helmchartinflationgenerator_).

The `helm` binary must be available in `PATH` or specified via `--helm-command`. When using the
Docker image, use one of the Helm-enabled variants:

- `ghcr.io/mia-platform/mlp:v2.6.0-helm3` — includes Helm 3
- `ghcr.io/mia-platform/mlp:v2.6.0-helm4` — includes Helm 4
- `ghcr.io/mia-platform/mlp:v2.6.0-helm` — alias for `-helm4`
