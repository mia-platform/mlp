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

package generate

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()
	assert.NotNil(t, cmd)

	configurationPath := filepath.Join("testdata", "configuration.yaml")
	outputPath := filepath.Join(t.TempDir(), "generate-cmd-outptut")
	cmd.SetArgs([]string{
		fmt.Sprintf("--config-file=%s", configurationPath),
		fmt.Sprintf("--out=%s", outputPath),
	})
	assert.NoError(t, cmd.Execute())
}

func TestOptions(t *testing.T) {
	t.Parallel()

	fSys := filesys.MakeEmptyDirInMemory()
	expectedOpts := &Options{
		configFiles: []string{"file.yaml"},
		prefixes:    []string{"prefix"},
		outputPath:  "output",
		fSys:        fSys,
	}

	flag := &Flags{
		prefixes:    []string{"prefix"},
		configFiles: []string{"file.yaml"},
		outputPath:  "output",
	}

	opts, err := flag.ToOptions(fSys)
	require.NoError(t, err)

	assert.Equal(t, expectedOpts, opts)
	assert.NoError(t, opts.Validate())
	opts.configFiles = []string{}

	assert.ErrorContains(t, opts.Validate(), "at least one config file must be specified")
}

func TestRun(t *testing.T) {
	key, cert := generateCertificates(t)
	encodedKey := base64.StdEncoding.EncodeToString([]byte(key))
	encodedCert := base64.StdEncoding.EncodeToString([]byte(cert))
	t.Setenv("MLP_DOCKER_PASSWORD", "password")
	t.Setenv("MLP_CERTIFICATE", cert)

	fSys := testFilesys(t)

	tlsSecret := fmt.Sprintf(tlsSecretFormat, encodedCert, encodedKey)

	require.NoError(t, fSys.WriteFile(filepath.Join("output", "tls.secret.yaml"), []byte(tlsSecret)))
	require.NoError(t, fSys.WriteFile("key.pem", []byte(key)))

	tests := map[string]struct {
		options             *Options
		expectedError       string
		expectedResultsPath string
	}{
		"creating resources": {
			options: &Options{
				prefixes:    []string{"MLP_"},
				configFiles: []string{"configuration.yaml", "cert.pem"},
				outputPath:  "generate-output",
				fSys:        fSys,
			},
			expectedResultsPath: "output",
		},
		"missing configuration": {
			options: &Options{
				prefixes:    []string{"MLP_"},
				configFiles: []string{"configuration.yml"},
				outputPath:  "missing-config",
				fSys:        fSys,
			},
			expectedError: "'configuration.yml' doesn't exist",
		},
		"error in interpolation": {
			options: &Options{
				prefixes:    []string{"MLP_MISSING_"},
				configFiles: []string{"configuration.yaml"},
				outputPath:  "interpolation-error",
				fSys:        fSys,
			},
			expectedError: `environment variable "DOCKER_PASSWORD" not found`,
		},
		"error validating certificates": {
			options: &Options{
				prefixes:    []string{"MLP_"},
				configFiles: []string{"broken-certificates.yaml"},
				outputPath:  "broken-certificates",
				fSys:        fSys,
			},
			expectedError: `tls: failed to find any PEM data in key input`,
		},
		"error reading file": {
			options: &Options{
				prefixes:    []string{"MLP_"},
				configFiles: []string{"missing-file.yaml"},
				outputPath:  "missing-file",
				fSys:        fSys,
			},
			expectedError: `'missing' doesn't exist`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.options.Run(context.TODO())

			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
				testStructure(t, fSys, test.options.outputPath, test.expectedResultsPath)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func testStructure(t *testing.T, fSys filesys.FileSystem, pathToTest, expectationPath string) {
	t.Helper()

	_ = fSys.Walk(pathToTest, func(path string, info fs.FileInfo, err error) error {
		require.NoError(t, err)
		cleanPath := strings.TrimPrefix(path, pathToTest)

		testPath := filepath.Join(expectationPath, cleanPath)
		if info.IsDir() {
			require.True(t, fSys.Exists(testPath))
		} else {
			data, err := fSys.ReadFile(path)
			require.NoError(t, err)
			expectedData, err := fSys.ReadFile(testPath)
			require.NoError(t, err)
			assert.Equal(t, string(expectedData), string(data), testPath)
		}

		return nil
	})
}

// generateCertificates generates certificate chain for testing purposes
func generateCertificates(t *testing.T) (keydata, certdata string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Generate a pem block with the private key
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	tml := x509.Certificate{
		// you can add any attr that you need
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(5, 0, 0),
		// you have to generate a different serial number each execution
		SerialNumber: big.NewInt(123123),
		Subject: pkix.Name{
			CommonName:   "New Name",
			Organization: []string{"New Org."},
		},
		BasicConstraintsValid: true,
	}
	cert, err := x509.CreateCertificate(rand.Reader, &tml, &tml, &key.PublicKey, key)
	require.NoError(t, err)

	// Generate a pem block with the certificate
	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})

	return string(keyPem), string(certPem)
}

func testFilesys(t *testing.T) filesys.FileSystem {
	t.Helper()

	fSys := filesys.MakeEmptyDirInMemory()
	require.NoError(t, fSys.MkdirAll("output"))
	require.NoError(t, fSys.MkdirAll("generate-output"))
	require.NoError(t, fSys.WriteFile("configuration.yaml", []byte(configurationFile)))
	require.NoError(t, fSys.WriteFile("broken-certificates.yaml", []byte(brokenCertificates)))
	require.NoError(t, fSys.WriteFile("missing-file.yaml", []byte(missingFile)))
	require.NoError(t, fSys.WriteFile("cert.pem", []byte(certificate)))
	require.NoError(t, fSys.WriteFile(filepath.Join("output", "docker.secret.yaml"), []byte(dockerSecret)))
	require.NoError(t, fSys.WriteFile(filepath.Join("output", "opaque.secret.yaml"), []byte(opaqueSecret)))
	require.NoError(t, fSys.WriteFile(filepath.Join("output", "files.configmap.yaml"), []byte(fileConfigMap)))
	require.NoError(t, fSys.WriteFile(filepath.Join("output", "literal.configmap.yaml"), []byte(literalConfigMap)))
	require.NoError(t, fSys.WriteFile("binary", []byte{0xff, 0xfd}))

	return fSys
}

const (
	dockerSecret = `apiVersion: v1
data:
  .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLmNvbSI6eyJ1c2VybmFtZSI6InVzZXJuYW1lIiwicGFzc3dvcmQiOiJwYXNzd29yZCIsImVtYWlsIjoiZW1haWxAZXhhbXBsZS5jb20iLCJhdXRoIjoiZFhObGNtNWhiV1U2Y0dGemMzZHZjbVE9In19fQ==
kind: Secret
metadata:
  annotations:
    mia-platform.eu/deploy: always
  creationTimestamp: null
  name: docker
type: kubernetes.io/dockerconfigjson
`
	tlsSecretFormat = `apiVersion: v1
data:
  tls.crt: %s
  tls.key: %s
kind: Secret
metadata:
  annotations:
    mia-platform.eu/deploy: always
  creationTimestamp: null
  name: tls
type: kubernetes.io/tls
`
	literalConfigMap = `apiVersion: v1
data:
  key: value
  otherKey: value
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: literal
`
	opaqueSecret = `apiVersion: v1
data:
  cert.pem: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZaVENDQTAyZ0F3SUJBZ0lVVTB4S0N0Vy8xeWVNcjVVY0p1alp1a2gwelZZd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1FqRUxNQWtHQTFVRUJoTUNXRmd4RlRBVEJnTlZCQWNNREVSbFptRjFiSFFnUTJsMGVURWNNQm9HQTFVRQpDZ3dUUkdWbVlYVnNkQ0JEYjIxd1lXNTVJRXgwWkRBZUZ3MHlNREV3TURFeE5ESXlNemhhRncweU1URXdNREV4Ck5ESXlNemhhTUVJeEN6QUpCZ05WQkFZVEFsaFlNUlV3RXdZRFZRUUhEQXhFWldaaGRXeDBJRU5wZEhreEhEQWEKQmdOVkJBb01FMFJsWm1GMWJIUWdRMjl0Y0dGdWVTQk1kR1F3Z2dJaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQwpEd0F3Z2dJS0FvSUNBUURsYnFVZFh4enNPaGQvbS8vMkxtYSs5eE1sTUF6MzJVRmNYZm5hdjRpK3dnV21UNUhBCmNpQXRvRWovTUQzSXBsM005Sm5nZ0ppK2EyRGNXOWd3OXZsa2dldVFnVHZaMVo3WExENnhlc1hjeTZmempWM2sKU2ZwNS9ncWM2M3cwRjhMNCtPYXhxdndWaElHQjl6VlFnMUpBcDc0N2duWWV6OGZHcEl0Ynhiam9VTFYwNmx6UApwbFg0VWY0VUhYNzU2aDUvSElHRnZBdlpUNkhpaGFueEVCZDVUbERsUEtSNEZYbmtJRUNHOEFBSERVMzBsc0dwClVZU242YXVIcERWeXJ6R1lnaThEU0tZdTByRm5iUDh1VVFUZ2xyNjZZUkdzNFRwTGRZOUI4ZmU2SHVCNmlzZG0KQ1BQS0FPM2doS3gwdk1NaVAxNW83MVBiKzV4RjhVdnhNYmRHcUJnTkIvWW5tSnR1aW5abEw2Uk5RRzN2dm5rcApBQU9YZmZpU25qQXNSR2g1L1hDOGVZeE04d1VFRGdROTgydWdrZFZPSUwwd3BMU2R2clVSNmNFY295SXVLVTJHClhBOVczTWlqYlo4NVE5dVlqL2Y4UzBreU5kMGZFbnNWMU1Rc1BubFA0UTdxQkJTaW5WUGI3WlgxYUd5ZkczdGwKT3doODBISzVPMkV1WWFkdVkwbVgyQkxTRjg1VXVDRndNZWIvWjdoazhSc1lNdUo1alYwU3BwTHlDb2UxbFgxVgpldnRZaThIcXQyY1dDTDFQMmZEM2VvOVJOMmtldmVFcGw0N3Y3TStwamF5QWdja3JPWHdDdjUxU2dxaFpkQ3ZMCjkwK2hxRHpNNWJqUnB0cXE4eUwwNWVrWWozemk3VnlBRFEwZXVBOVJoWEZXV0tJQnhlMDJkODdGRXdJREFRQUIKbzFNd1VUQWRCZ05WSFE0RUZnUVVDVHNMK0VWaUl1d3l2RzErSmpNNmlQMFpIVTR3SHdZRFZSMGpCQmd3Rm9BVQpDVHNMK0VWaUl1d3l2RzErSmpNNmlQMFpIVTR3RHdZRFZSMFRBUUgvQkFVd0F3RUIvekFOQmdrcWhraUc5dzBCCkFRc0ZBQU9DQWdFQTVJTG9NVGJiSlA1dTNWczRuajB5bVp5VE8wNUZiQ2k3ZGZBQXdDQTJrYjR5Vis3KzladGEKRmd6Q3MzQS8xb3JEUTc2c0ZFUyttSE5ud1lmSWg2YW1Ncm9HbGk2U0RCOGdqUUpxR0RyNUJpMlBkeUV5RHhOYgpuaXFnYm55bzNzME1ZN1hDNXpsc2grdVBVd3NxV1QzbVVNczRyZFZmZWVlSUFTY1QrWnJrVzVreUNIN0FuT2pSCkNYN1VvKzlEMHpMN3J1SlpKL3AzZEo3Z1dnWG1SN2F2MW1idXdyZlN4UDhiamlJd2NOaGw2NTYvMStBWlp2MGsKQjlzVGtYTVdhVitYYjUrem56NHJScDAydk5BVkM2bnpwVjBXME1CK3BjRWcrTnZnK1dyeHJYTUZ2Q3crOWZEQgpUYzZiRVRUc0s3KzNBSXdScjdYSzJraFFiZzhuUk83Skd0VjBiYmhtT0dkU1ZJd1VsSGJ3aGkrOUN2VDNFRnJpClMyVUFubG9ySHJDR2RmdEZvK0djUVREUW9BeTdyaUhBYlNLS1RRWVlpVjV5c24rRlVGTExWNWQxanM1V2lBdUUKYzcyV3dqZXhXVzd5anFPVFBXZTlhcW5laFFMZVdacG45WHp6UmduQTgvZTZxYy9xZndqbkx6ellDZnBUNjhndQpLenJmU3lISUZCay9YNjRDbUFlVEREcjcyQkxuUDdVVmhTU0F1RzVvU0hFM0duaTZ2dmtwanVVenNsRmE0YVJsClNEejB2ejFiclg4SkY3ZUVzT3RGWC8xVS9WUVZOWWVRbEtrMFhnZnRRWVJpT0Vvd3JmVkhuQVluRHgxOTVzV2sKRG1ld0pLZi9QNktPVGc0akV5TkMxV0lrQjhLaWl2b3hyUGxvNE93RU5uK0hybzFzTENWSnRwVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
  key: dmFsdWU=
kind: Secret
metadata:
  annotations:
    mia-platform.eu/deploy: always
  creationTimestamp: null
  name: opaque
type: Opaque
`
	fileConfigMap = `apiVersion: v1
binaryData:
  binary: //0=
data:
  cert.pem: |
    -----BEGIN CERTIFICATE-----
    MIIFZTCCA02gAwIBAgIUU0xKCtW/1yeMr5UcJujZukh0zVYwDQYJKoZIhvcNAQEL
    BQAwQjELMAkGA1UEBhMCWFgxFTATBgNVBAcMDERlZmF1bHQgQ2l0eTEcMBoGA1UE
    CgwTRGVmYXVsdCBDb21wYW55IEx0ZDAeFw0yMDEwMDExNDIyMzhaFw0yMTEwMDEx
    NDIyMzhaMEIxCzAJBgNVBAYTAlhYMRUwEwYDVQQHDAxEZWZhdWx0IENpdHkxHDAa
    BgNVBAoME0RlZmF1bHQgQ29tcGFueSBMdGQwggIiMA0GCSqGSIb3DQEBAQUAA4IC
    DwAwggIKAoICAQDlbqUdXxzsOhd/m//2Lma+9xMlMAz32UFcXfnav4i+wgWmT5HA
    ciAtoEj/MD3Ipl3M9JnggJi+a2DcW9gw9vlkgeuQgTvZ1Z7XLD6xesXcy6fzjV3k
    Sfp5/gqc63w0F8L4+OaxqvwVhIGB9zVQg1JAp747gnYez8fGpItbxbjoULV06lzP
    plX4Uf4UHX756h5/HIGFvAvZT6HihanxEBd5TlDlPKR4FXnkIECG8AAHDU30lsGp
    UYSn6auHpDVyrzGYgi8DSKYu0rFnbP8uUQTglr66YRGs4TpLdY9B8fe6HuB6isdm
    CPPKAO3ghKx0vMMiP15o71Pb+5xF8UvxMbdGqBgNB/YnmJtuinZlL6RNQG3vvnkp
    AAOXffiSnjAsRGh5/XC8eYxM8wUEDgQ982ugkdVOIL0wpLSdvrUR6cEcoyIuKU2G
    XA9W3MijbZ85Q9uYj/f8S0kyNd0fEnsV1MQsPnlP4Q7qBBSinVPb7ZX1aGyfG3tl
    Owh80HK5O2EuYaduY0mX2BLSF85UuCFwMeb/Z7hk8RsYMuJ5jV0SppLyCoe1lX1V
    evtYi8Hqt2cWCL1P2fD3eo9RN2keveEpl47v7M+pjayAgckrOXwCv51SgqhZdCvL
    90+hqDzM5bjRptqq8yL05ekYj3zi7VyADQ0euA9RhXFWWKIBxe02d87FEwIDAQAB
    o1MwUTAdBgNVHQ4EFgQUCTsL+EViIuwyvG1+JjM6iP0ZHU4wHwYDVR0jBBgwFoAU
    CTsL+EViIuwyvG1+JjM6iP0ZHU4wDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B
    AQsFAAOCAgEA5ILoMTbbJP5u3Vs4nj0ymZyTO05FbCi7dfAAwCA2kb4yV+7+9Zta
    FgzCs3A/1orDQ76sFES+mHNnwYfIh6amMroGli6SDB8gjQJqGDr5Bi2PdyEyDxNb
    niqgbnyo3s0MY7XC5zlsh+uPUwsqWT3mUMs4rdVfeeeIAScT+ZrkW5kyCH7AnOjR
    CX7Uo+9D0zL7ruJZJ/p3dJ7gWgXmR7av1mbuwrfSxP8bjiIwcNhl656/1+AZZv0k
    B9sTkXMWaV+Xb5+znz4rRp02vNAVC6nzpV0W0MB+pcEg+Nvg+WrxrXMFvCw+9fDB
    Tc6bETTsK7+3AIwRr7XK2khQbg8nRO7JGtV0bbhmOGdSVIwUlHbwhi+9CvT3EFri
    S2UAnlorHrCGdftFo+GcQTDQoAy7riHAbSKKTQYYiV5ysn+FUFLLV5d1js5WiAuE
    c72WwjexWW7yjqOTPWe9aqnehQLeWZpn9XzzRgnA8/e6qc/qfwjnLzzYCfpT68gu
    KzrfSyHIFBk/X64CmAeTDDr72BLnP7UVhSSAuG5oSHE3Gni6vvkpjuUzslFa4aRl
    SDz0vz1brX8JF7eEsOtFX/1U/VQVNYeQlKk0XgftQYRiOEowrfVHnAYnDx195sWk
    DmewJKf/P6KOTg4jEyNC1WIkB8KiivoxrPlo4OwENn+Hro1sLCVJtpU=
    -----END CERTIFICATE-----
  key: value
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: files
`
	certificate = `-----BEGIN CERTIFICATE-----
MIIFZTCCA02gAwIBAgIUU0xKCtW/1yeMr5UcJujZukh0zVYwDQYJKoZIhvcNAQEL
BQAwQjELMAkGA1UEBhMCWFgxFTATBgNVBAcMDERlZmF1bHQgQ2l0eTEcMBoGA1UE
CgwTRGVmYXVsdCBDb21wYW55IEx0ZDAeFw0yMDEwMDExNDIyMzhaFw0yMTEwMDEx
NDIyMzhaMEIxCzAJBgNVBAYTAlhYMRUwEwYDVQQHDAxEZWZhdWx0IENpdHkxHDAa
BgNVBAoME0RlZmF1bHQgQ29tcGFueSBMdGQwggIiMA0GCSqGSIb3DQEBAQUAA4IC
DwAwggIKAoICAQDlbqUdXxzsOhd/m//2Lma+9xMlMAz32UFcXfnav4i+wgWmT5HA
ciAtoEj/MD3Ipl3M9JnggJi+a2DcW9gw9vlkgeuQgTvZ1Z7XLD6xesXcy6fzjV3k
Sfp5/gqc63w0F8L4+OaxqvwVhIGB9zVQg1JAp747gnYez8fGpItbxbjoULV06lzP
plX4Uf4UHX756h5/HIGFvAvZT6HihanxEBd5TlDlPKR4FXnkIECG8AAHDU30lsGp
UYSn6auHpDVyrzGYgi8DSKYu0rFnbP8uUQTglr66YRGs4TpLdY9B8fe6HuB6isdm
CPPKAO3ghKx0vMMiP15o71Pb+5xF8UvxMbdGqBgNB/YnmJtuinZlL6RNQG3vvnkp
AAOXffiSnjAsRGh5/XC8eYxM8wUEDgQ982ugkdVOIL0wpLSdvrUR6cEcoyIuKU2G
XA9W3MijbZ85Q9uYj/f8S0kyNd0fEnsV1MQsPnlP4Q7qBBSinVPb7ZX1aGyfG3tl
Owh80HK5O2EuYaduY0mX2BLSF85UuCFwMeb/Z7hk8RsYMuJ5jV0SppLyCoe1lX1V
evtYi8Hqt2cWCL1P2fD3eo9RN2keveEpl47v7M+pjayAgckrOXwCv51SgqhZdCvL
90+hqDzM5bjRptqq8yL05ekYj3zi7VyADQ0euA9RhXFWWKIBxe02d87FEwIDAQAB
o1MwUTAdBgNVHQ4EFgQUCTsL+EViIuwyvG1+JjM6iP0ZHU4wHwYDVR0jBBgwFoAU
CTsL+EViIuwyvG1+JjM6iP0ZHU4wDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B
AQsFAAOCAgEA5ILoMTbbJP5u3Vs4nj0ymZyTO05FbCi7dfAAwCA2kb4yV+7+9Zta
FgzCs3A/1orDQ76sFES+mHNnwYfIh6amMroGli6SDB8gjQJqGDr5Bi2PdyEyDxNb
niqgbnyo3s0MY7XC5zlsh+uPUwsqWT3mUMs4rdVfeeeIAScT+ZrkW5kyCH7AnOjR
CX7Uo+9D0zL7ruJZJ/p3dJ7gWgXmR7av1mbuwrfSxP8bjiIwcNhl656/1+AZZv0k
B9sTkXMWaV+Xb5+znz4rRp02vNAVC6nzpV0W0MB+pcEg+Nvg+WrxrXMFvCw+9fDB
Tc6bETTsK7+3AIwRr7XK2khQbg8nRO7JGtV0bbhmOGdSVIwUlHbwhi+9CvT3EFri
S2UAnlorHrCGdftFo+GcQTDQoAy7riHAbSKKTQYYiV5ysn+FUFLLV5d1js5WiAuE
c72WwjexWW7yjqOTPWe9aqnehQLeWZpn9XzzRgnA8/e6qc/qfwjnLzzYCfpT68gu
KzrfSyHIFBk/X64CmAeTDDr72BLnP7UVhSSAuG5oSHE3Gni6vvkpjuUzslFa4aRl
SDz0vz1brX8JF7eEsOtFX/1U/VQVNYeQlKk0XgftQYRiOEowrfVHnAYnDx195sWk
DmewJKf/P6KOTg4jEyNC1WIkB8KiivoxrPlo4OwENn+Hro1sLCVJtpU=
-----END CERTIFICATE-----
`
	configurationFile = `secrets:
- name: "opaque"
  when: "always"
  data:
  - from: "literal"
    key: key
    value: value
  - from: "file"
    file: "cert.pem"
- name: "docker"
  when: "always"
  docker:
    username: "username"
    password: "{{DOCKER_PASSWORD}}"
    email: "email@example.com"
    server: "example.com"
- name: "tls"
  when: "always"
  tls:
    cert:
      from: "literal"
      value: "{{CERTIFICATE}}"
    key:
      from: "file"
      file: key.pem
config-maps:
- name: "literal"
  data:
  - from: "literal"
    key: key
    value: value
  - from: "literal"
    key: otherKey
    value: value
- name: "files"
  data:
  - from: "literal"
    key: key
    value: value
  - from: "file"
    file: "cert.pem"
  - from: "file"
    file: "binary"
`
	brokenCertificates = `secrets:
- name: "tls"
  when: "always"
  tls:
    cert:
      from: "literal"
      value: "{{CERTIFICATE}}"
`
	missingFile = `config-maps:
- name: "files"
  data:
  - from: "file"
    file: "missing"`
)
