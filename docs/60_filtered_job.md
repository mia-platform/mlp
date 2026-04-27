# Filtered Jobs

`mlp` supports the execution of Kubernetes `Job` resources before the main deploy phase. These filtered jobs
can be used to run database migrations, data transformations, or any other task that must complete successfully
before the rest of the resources are applied to the cluster.

## How It Works

Filtered jobs are identified by the annotation `mia-platform.eu/deploy` on a `Job` resource. When the
`--filtered-job-annotation` flag is provided to the `deploy` command, `mlp` will scan the resources and
separate all `Job` resources whose annotation value matches the one provided.

These jobs are executed before the remaining resources are applied. If the flag is not provided, any `Job`
resource carrying the `mia-platform.eu/deploy` annotation will be stripped from the resource list and not
applied at all.

## Annotating a Job

To mark a `Job` as a filtered job, add the `mia-platform.eu/deploy` annotation with the desired value:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  annotations:
    mia-platform.eu/deploy: pre-deploy
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: migrate
        image: my-app:latest
        command: ["./migrate"]
```

Then pass the matching value to the deploy command:

```sh
mlp deploy --filtered-job-annotation pre-deploy ...
```

## Optional Jobs

A filtered job can be marked as optional by adding the annotation `mia-platform.eu/deploy-optional: "true"`.
Optional jobs are non-blocking: if they fail, the failure is logged as a warning and the deploy process
continues normally. Mandatory jobs (those without the optional annotation) will block and fail the deploy if
they cannot complete successfully.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: optional-cleanup
  annotations:
    mia-platform.eu/deploy: pre-deploy
    mia-platform.eu/deploy-optional: "true"
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: cleanup
        image: my-app:latest
        command: ["./cleanup"]
```

## Retry and Timeout

Each filtered job is retried automatically on failure. Before each retry the failed job is deleted from
the cluster so that a fresh instance can be created. The number of retries and the per-execution timeout
can be controlled via dedicated flags:

| Flag | Default | Description |
|---|---|---|
| `--filtered-job-annotation` | _(empty)_ | Annotation value used to identify filtered jobs |
| `--filtered-job-max-retries` | `3` | Maximum number of retry attempts for a failed job |
| `--filtered-job-timeout` | `30s` | Timeout for a single job execution attempt |

If a job exceeds the configured timeout it is considered failed and the retry logic applies as normal.

## Dry Run

When the `--dry-run` flag is active, no jobs are created on the cluster. Instead, `mlp` will print a message
for each job that would have been executed, allowing you to verify the configuration without side effects.
