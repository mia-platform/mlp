// Copyright Mia srl
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build conformance

package e2e

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/mia-platform/mlp/v2/pkg/cmd/deploy"
	"github.com/mia-platform/mlp/v2/pkg/extensions"
)

func TestDeployOnEmptyCluster(t *testing.T) {
	deploymentFeature := features.New("deployment on empty cluster").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Helper()
			t.Logf("starting test with kubeconfig %q and namespace %q", cfg.KubeconfigFile(), cfg.Namespace())

			buffer := new(bytes.Buffer)
			deployCmd := deploy.NewCommand(genericclioptions.NewConfigFlags(false))
			deployCmd.SetErr(buffer)
			deployCmd.SetOut(buffer)

			deployCmd.SetArgs([]string{
				"--kubeconfig",
				cfg.KubeconfigFile(),
				"--namespace",
				cfg.Namespace(),
				"--filename",
				filepath.Join("testdata", "apply-resources"),
			})

			assert.NoError(t, deployCmd.ExecuteContext(ctx))
			t.Log(buffer.String())
			buffer.Reset()
			return ctx
		}).
		Assess("resoures are being created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Helper()

			deployment := new(appsv1.Deployment)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), deployment))
			t.Logf("deployment found: %s", deployment.Name)
			assert.NotEmpty(t, deployment.Spec.Template.Annotations["mia-platform.eu/deploy-checksum"])
			assert.NotEmpty(t, deployment.Spec.Template.Annotations["mia-platform.eu/dependencies-checksum"])

			configMap := new(corev1.ConfigMap)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "literal", cfg.Namespace(), configMap))
			t.Logf("configmap found: %s", configMap.Name)
			assert.Empty(t, configMap.Annotations["mia-platform.eu/deploy-checksum"])
			assert.Empty(t, configMap.Annotations["mia-platform.eu/dependencies-checksum"])

			secret1 := new(corev1.Secret)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "docker", cfg.Namespace(), secret1))
			t.Logf("secret found: %s", secret1.Name)
			assert.Empty(t, secret1.Annotations["mia-platform.eu/deploy-checksum"])
			assert.Empty(t, secret1.Annotations["mia-platform.eu/dependencies-checksum"])

			secret2 := new(corev1.Secret)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "opaque", cfg.Namespace(), secret2))
			t.Logf("secret found: %s", secret2.Name)
			assert.Empty(t, secret2.Annotations["mia-platform.eu/deploy-checksum"])
			assert.Empty(t, secret2.Annotations["mia-platform.eu/dependencies-checksum"])

			cronjob := new(batchv1.CronJob)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), cronjob))
			t.Logf("cronjob found: %s", cronjob.Name)
			assert.Empty(t, cronjob.Spec.JobTemplate.Spec.Template.Annotations["mia-platform.eu/deploy-checksum"])
			assert.Empty(t, cronjob.Spec.JobTemplate.Spec.Template.Annotations["mia-platform.eu/dependencies-checksum"])

			jobList := new(batchv1.JobList)
			require.NoError(t, cfg.Client().Resources(cfg.Namespace()).List(ctx, jobList))
			t.Logf("jobs found: %d", len(jobList.Items))
			manualJobFound := false
			for _, job := range jobList.Items {
				if value, found := job.Annotations["cronjob.kubernetes.io/instantiate"]; found && value == "manual" {
					manualJobFound = true
					break
				}
			}
			assert.True(t, manualJobFound)

			inventory := new(corev1.ConfigMap)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "eu.mia-platform.mlp", cfg.Namespace(), inventory))
			t.Logf("inventory found: %s", configMap.Name)
			assert.Len(t, inventory.Data, 6)

			return ctx
		}).
		Feature()

	testenv.Test(t, deploymentFeature)
}

func TestSmartDeploy(t *testing.T) {
	deploying := func(ctx context.Context, cfg *envconf.Config, deployType, stage string) {
		buffer := new(bytes.Buffer)
		deployCmd := deploy.NewCommand(genericclioptions.NewConfigFlags(false)) //nolint:contextcheck
		deployCmd.SetErr(buffer)
		deployCmd.SetOut(buffer)

		deployCmd.SetArgs([]string{
			"--deploy-type",
			deployType,
			"--kubeconfig",
			cfg.KubeconfigFile(),
			"--namespace",
			cfg.Namespace(),
			"--filename",
			filepath.Join("testdata", "smart-deploy", stage),
		})

		assert.NoError(t, deployCmd.ExecuteContext(ctx))
		t.Log(buffer.String())
		buffer.Reset()
	}

	var deployChecksum, dependenciesChecksum string

	smartDeployPhase1 := features.New("deploy phase 1").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Helper()
			t.Logf("starting test with kubeconfig %q and namespace %q", cfg.KubeconfigFile(), cfg.Namespace())
			deploying(ctx, cfg, extensions.DeployAll, "stage1")
			return ctx
		}).
		Assess("deploy phase 1", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Helper()

			deployment := new(appsv1.Deployment)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), deployment))
			t.Logf("deployment found: %s", deployment.Name)
			deployChecksum = deployment.Spec.Template.Annotations["mia-platform.eu/deploy-checksum"]
			assert.NotEmpty(t, deployChecksum)
			dependenciesChecksum = deployment.Spec.Template.Annotations["mia-platform.eu/dependencies-checksum"]
			assert.NotEmpty(t, dependenciesChecksum)

			secret := new(corev1.Secret)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), secret))
			t.Logf("secret found: %s", secret.Name)

			return ctx
		}).
		Assess("smart deploy phase 2", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Helper()
			deploying(ctx, cfg, extensions.DeploySmart, "stage2")
			time.Sleep(2 * time.Second) // sleep to wait background deletions

			deployment := new(appsv1.Deployment)
			require.NoError(t, cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), deployment))
			t.Logf("deployment found: %s", deployment.Name)
			assert.Equal(t, deployChecksum, deployment.Spec.Template.Annotations["mia-platform.eu/deploy-checksum"])
			assert.NotEqual(t, dependenciesChecksum, deployment.Spec.Template.Annotations["mia-platform.eu/dependencies-checksum"])
			assert.EqualValues(t, 2, deployment.Generation)

			secret := new(corev1.Secret)
			assert.True(t, apierrors.IsNotFound(cfg.Client().Resources().Get(ctx, "test", cfg.Namespace(), secret)))

			return ctx
		}).
		Feature()

	testenv.Test(t, smartDeployPhase1)
}
