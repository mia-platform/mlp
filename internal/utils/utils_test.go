// Copyright 2020 Mia srl
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
package utils

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const testdata = "testdata/"

func TestExtractYAMLFiles(t *testing.T) {
	folder := filepath.Join(testdata, "folder")
	singlefile := filepath.Join(testdata, "file.yml")
	singlefileAbsPath, _ := filepath.Abs(singlefile)
	barAbsPath, _ := filepath.Abs(filepath.Join(folder, "bar.yaml"))
	fooAbsPath, _ := filepath.Abs(filepath.Join(folder, "foo.yml"))

	t.Run("call function without files", func(t *testing.T) {
		paths := []string{}
		files, err := ExtractYAMLFiles(paths)
		require.Equal(t, paths, files)
		require.Nil(t, err, "if paths are returned error must be nil")
	})

	t.Run("call function with file", func(t *testing.T) {
		paths := []string{singlefile}
		files, err := ExtractYAMLFiles(paths)
		require.Equal(t, []string{singlefileAbsPath}, files)
		require.Nil(t, err, "if paths are returned error must be nil")
	})

	t.Run("call function with folder", func(t *testing.T) {
		paths := []string{folder}
		files, err := ExtractYAMLFiles(paths)
		require.Equal(t, []string{barAbsPath, fooAbsPath}, files)
		require.Nil(t, err, "if paths are returned error must be nil")
	})

	t.Run("call function with both file and folder", func(t *testing.T) {
		paths := []string{folder, singlefile}
		files, err := ExtractYAMLFiles(paths)
		require.Equal(t, []string{barAbsPath, fooAbsPath, singlefileAbsPath}, files)
		require.Nil(t, err, "if paths are returned error must be nil")
	})

	t.Run("call function with non existent path", func(t *testing.T) {
		paths := []string{"im-not-here.yaml"}
		files, err := ExtractYAMLFiles(paths)
		require.Nil(t, files, "when the function return an error the returned paths must be nil")
		require.Error(t, err, "with no file existed return the underling error")
	})

	t.Run("call function with empty directory return empty array", func(t *testing.T) {
		path, _ := fs.TempDir("", "temp")
		t.Cleanup(func() {
			fs.RemoveAll(path)
		})

		files, err := ExtractYAMLFiles([]string{path})
		require.Equal(t, []string{}, files)
		require.Nil(t, err, "if paths are returned error must be nil")
	})
}

func TestReadFile(t *testing.T) {
	t.Run("read a file ", func(t *testing.T) {
		file, _ := fs.TempFile(".", "tempfile")
		file.WriteString("prova")
		t.Cleanup(func() {
			fs.Remove(file.Name())
		})

		data, err := ReadFile(file.Name())
		require.Equal(t, "prova", string(data))
		require.Nil(t, err, "if paths are returned error must be nil")
	})

	t.Run("read a file that cannot be read", func(t *testing.T) {
		data, err := ReadFile("file-doesnt-exists")
		require.Nil(t, data, "cannot read file in write only")
		require.Error(t, err, "with no file that can be read return the underling error")
	})
}
