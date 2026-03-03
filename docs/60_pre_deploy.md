# Pre-Deploy Jobs

The `deploy` command now supports executing Jobs marked as pre-deploy jobs before the main deployment pipeline.
This feature allows you to run initialization or validation tasks (such as database migrations, secret provisioning,
or health checks) prior to deploying your main workloads.

## Identifying Pre-Deploy Jobs

A Job is identified as a pre-deploy job by annotating it with the `mia-platform.eu/deploy` annotation and setting
its value to `pre-deploy`. This annotation value can be customized via the `--pre-deploy-job-annotation` CLI flag.

Example Job manifest:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration-job
  annotations:
    mia-platform.eu/deploy: pre-deploy
spec:
  template:
    spec:
      containers:
      - name: migrator
        image: migrations:latest
      restartPolicy: Never
```

## Pre-Deploy Job Execution Flow

When the `deploy` command runs, it performs the following steps:

1. **Extraction**: Pre-deploy jobs are extracted from the resource list based on the configured annotation.
2. **Execution**: Each pre-deploy job is applied to the cluster in sequence.
3. **Monitoring**: The command waits for each job to complete with a configurable timeout.
4. **Retry Logic**: If a job fails, the command automatically retries the execution up to a maximum number of attempts.
5. **Main Deployment**: After all pre-deploy jobs successfully complete, the main deployment pipeline proceeds.
6. **Filtering**: Pre-deploy jobs are filtered out and do not participate in the main deployment phase.

## Configuration

The behavior of pre-deploy jobs can be configured via the following CLI flags:

### `--pre-deploy-job-timeout`

The maximum duration to wait for a single pre-deploy job to complete.

- **Type**: Duration
- **Default**: `30s`
- **Usage**: `mlp deploy --pre-deploy-job-timeout 2m`

Non-zero values should contain a corresponding time unit (e.g., `1s`, `2m`, `3h`).

### `--pre-deploy-job-max-retries`

The maximum number of attempts for executing a pre-deploy job before aborting the deploy.

- **Type**: Integer
- **Default**: `3`
- **Usage**: `mlp deploy --pre-deploy-job-max-retries 5`

### `--pre-deploy-job-annotation`

The value of the `mia-platform.eu/deploy` annotation used to identify pre-deploy jobs.
This allows you to customize the annotation value used to mark jobs as pre-deploy jobs.

- **Type**: String
- **Default**: `pre-deploy`
- **Usage**: `mlp deploy --pre-deploy-job-annotation init-job`

## Optional Pre-Deploy Jobs

Pre-deploy jobs can be marked as optional using the `mia-platform.eu/deploy-optional` annotation set to `true`.
Optional jobs that fail will log a warning but will not block the main deployment pipeline.

Example:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: optional-config-job
  annotations:
    mia-platform.eu/deploy: pre-deploy
    mia-platform.eu/deploy-optional: "true"
spec:
  template:
    spec:
      containers:
      - name: config
        image: config-tool:latest
      restartPolicy: Never
```

## Error Handling

### Mandatory Pre-Deploy Job Failure

If a mandatory pre-deploy job fails after all retry attempts are exhausted, the deploy process is aborted
and an error is returned. This ensures that critical initialization tasks are completed before proceeding
with the main deployment.

### Optional Pre-Deploy Job Failure

If an optional pre-deploy job fails, a warning is logged and the deploy process continues.
This allows for graceful handling of non-critical pre-deployment tasks.

## Dry-Run Mode

When using the `--dry-run` flag, pre-deploy jobs are applied to the cluster with the dry-run option enabled.
In this mode, job status polling is skipped, and the command does not wait for job completion.

## Polling Behavior

The command polls the status of each pre-deploy job every 5 seconds until:

- The job completes successfully
- The job exceeds the configured timeout
- The maximum number of retries is reached

