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

package main

import (
	"testing"

	"github.com/mia-platform/mlp/internal/cli"
	"github.com/stretchr/testify/require"
)

func Test_versionFormat(t *testing.T) {
	t.Run("version string with no args", func(t *testing.T) {
		expected := "mlp version: "
		version := versionFormat("", "")
		require.Equal(t, expected, version, "version expected is different from output")
	})

	t.Run("version string with only version string", func(t *testing.T) {
		expected := "mlp version: 1.0.0"
		version := versionFormat("1.0.0", "")
		require.Equal(t, expected, version, "version expected is different from output")
	})

	t.Run("version string with version and date strings", func(t *testing.T) {
		expected := "mlp version: 1.0.0 (2020-01-01)"
		version := versionFormat("1.0.0", "2020-01-01")
		require.Equal(t, expected, version, "version expected is different from output")
	})

	t.Run("version from cli module defaults", func(t *testing.T) {
		expected := "mlp version: DEV"
		version := versionFormat(cli.Version, cli.BuildDate)
		require.Equal(t, expected, version, "version expected is different from output")
	})
}
