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

//nolint:thelper
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

type namespaceCtxKey string

var (
	testenv env.Environment
)

func TestMain(m *testing.M) {
	testenv = env.New()

	// Specifying a run ID so that multiple runs wouldn't collide.
	runID := envconf.RandomName("ns", 4)

	kindClusterName := "mlp-e2e-tests"
	kindImageName := "kindest/node:v1.34.3@sha256:08497ee19eace7b4b5348db5c6a1591d7752b164530a36f855cb0f2bdcbadd48"
	if nameFromEnv, found := os.LookupEnv("KIND_NODE_IMAGE"); found {
		kindImageName = nameFromEnv
	}

	imageOpts := kind.WithImage(kindImageName)

	kindConfigPath := filepath.Join("testdata", "kind.yaml")
	crdsPath := filepath.Join("testdata", "crds")

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		envfuncs.CreateClusterWithConfig(kind.NewProvider(), kindClusterName, kindConfigPath, imageOpts),
		envfuncs.SetupCRDs(crdsPath, "*"),
	)

	testenv.BeforeEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return createNSForTest(ctx, t, cfg, runID)
	})

	testenv.AfterEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return deleteNSForTest(ctx, t, cfg)
	})

	// Use pre-defined environment funcs to teardown kind cluster after tests
	testenv.Finish(
		// envfuncs.ExportClusterLogs(kindClusterName, "logs"),
		envfuncs.TeardownCRDs(crdsPath, "*"),
		// envfuncs.DestroyCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

// CreateNSForTest creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func createNSForTest(ctx context.Context, t *testing.T, cfg *envconf.Config, runID string) (context.Context, error) {
	ns := envconf.RandomName(runID, 10)
	t.Logf("Creating NS %q for test %q", ns, t.Name())
	ctx = context.WithValue(ctx, getNamespaceKey(t), ns)

	return envfuncs.CreateNamespace(ns)(ctx, cfg)
}

// DeleteNSForTest looks up the namespace corresponding to the given test and deletes it.
func deleteNSForTest(ctx context.Context, t *testing.T, cfg *envconf.Config) (context.Context, error) {
	ns := fmt.Sprint(ctx.Value(getNamespaceKey(t)))
	t.Logf("Deleting NS %q for test %q", ns, t.Name())
	return envfuncs.DeleteNamespace(ns)(ctx, cfg)
}

// GetNamespaceKey returns the context key for a given test
func getNamespaceKey(t *testing.T) namespaceCtxKey {
	// When we pass t.Name() from inside an `assess` step, the name is in the form TestName/Features/Assess
	if strings.Contains(t.Name(), "/") {
		return namespaceCtxKey(strings.Split(t.Name(), "/")[0])
	}

	// When pass t.Name() from inside a `testenv.BeforeEachTest` function, the name is just TestName
	return namespaceCtxKey(t.Name())
}
