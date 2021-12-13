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

package deploy

import (
	"context"
	"path/filepath"
	"time"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var testEnv *envtest.Environment
var cfg *rest.Config
var clients *k8sClients

var _ = BeforeSuite(func() {

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	clients = &k8sClients{
		dynamic:   dynamic.NewForConfigOrDie(cfg),
		discovery: discovery.NewDiscoveryClientForConfigOrDie(cfg),
	}
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
}, 60)

var _ = Describe("deploy on mock kubernetes", func() {
	deployConfig := utils.DeployConfig{
		DeployType:              deployAll,
		ForceDeployWhenNoSemver: true,
		EnsureNamespace:         true,
	}
	currentTime := time.Now()

	Context("apply resources", func() {
		It("creates non existing secret without namespace in metadata", func() {
			err := doRun(clients, "test1", []string{"testdata/integration/apply-resources/docker.secret.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			_, err = clients.dynamic.Resource(gvrSecrets).Namespace("test1").
				Get(context.Background(), "docker", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
		It("creates non existing secret overriding namespace in metadata", func() {
			err := doRun(clients, "test2", []string{"testdata/integration/apply-resources/docker-ns.secret.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			_, err = clients.dynamic.Resource(gvrSecrets).Namespace("test2").
				Get(context.Background(), "docker", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
		It("gives error with no given namespace and no namespace in metadata", func() {
			err := doRun(clients, "", []string{"testdata/integration/apply-resources/docker-no-ns.secret.yaml"}, deployConfig, currentTime)
			Expect(err).To(HaveOccurred())
		})
		It("updates secret", func() {
			err := doRun(clients, "test3", []string{"testdata/integration/apply-resources/opaque-1.secret.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test3", []string{"testdata/integration/apply-resources/opaque-2.secret.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			sec, err := clients.dynamic.Resource(gvrSecrets).Namespace("test3").
				Get(context.Background(), "opaque", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Datakey, _, err := unstructured.NestedString(sec.Object, "data", "key")
			Expect(err).NotTo(HaveOccurred())
			Expect(Datakey).To(Equal("YW5vdGhlcnZhbHVl"))
			Expect(sec.GetLabels()["app.kubernetes.io/managed-by"]).To(Equal("mia-platform"))
			By("No annotation last applied for configmap and secrets")
			Expect(sec.GetLabels()[corev1.LastAppliedConfigAnnotation]).To(Equal(""))
		})
		It("updates configmap", func() {
			err := doRun(clients, "test3", []string{"testdata/integration/apply-resources/literal-1.configmap.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test3", []string{"testdata/integration/apply-resources/literal-2.configmap.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			sec, err := clients.dynamic.Resource(gvrConfigMaps).Namespace("test3").
				Get(context.Background(), "literal", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Datakey, _, err := unstructured.NestedString(sec.Object, "data", "unaKey")
			Expect(err).NotTo(HaveOccurred())
			Expect(Datakey).To(Equal("differentValue1"))
			Expect(sec.GetLabels()["app.kubernetes.io/managed-by"]).To(Equal("mia-platform"))
			By("No annotation last applied for configmap and secrets")
			Expect(sec.GetLabels()[corev1.LastAppliedConfigAnnotation]).To(Equal(""))
		})
		It("creates and updates depoyment", func() {
			err := doRun(clients, "test3", []string{"testdata/integration/apply-resources/test-deployment-1.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test3", []string{"testdata/integration/apply-resources/test-deployment-2.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			dep, err := clients.dynamic.Resource(gvrDeployments).
				Namespace("test3").
				Get(context.Background(), "test-deployment", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.GetLabels()["app.kubernetes.io/managed-by"]).To(Equal("mia-platform"))
			Expect(dep.GetAnnotations()[corev1.LastAppliedConfigAnnotation]).NotTo(Equal(""))
		})
		It("is respected mia-platform.eu/once annotation", func() {
			err := doRun(clients, "test3", []string{"testdata/integration/apply-resources/test-cronjob-1.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test3", []string{"testdata/integration/apply-resources/test-cronjob-2.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			cron, err := clients.dynamic.Resource(gvrV1beta1Cronjobs).
				Namespace("test3").
				Get(context.Background(), "test-cronjob", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			containers, _, err := unstructured.NestedSlice(cron.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers")
			Expect(err).NotTo(HaveOccurred())
			Expect(containers[0].(map[string]interface{})["image"].(string)).To(Equal("busybox"))
		})
		It("creates job from cronjob", func() {
			err := doRun(clients, "test4", []string{"testdata/integration/apply-resources/test-cronjob-1.yaml"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			jobList, err := clients.dynamic.Resource(gvrJobs).
				Namespace("test4").
				List(context.Background(), metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(jobList.Items[0].GetLabels()["job-name"]).To(ContainSubstring("test-cronjob"))
		})
	})
	Context("smart deploy", func() {
		deployConfig := utils.DeployConfig{
			DeployType:              smartDeploy,
			ForceDeployWhenNoSemver: true,
			EnsureNamespace:         true,
		}
		It("changes a pod annotation in deployment if configmap associated changes", func() {
			err := doRun(clients, "test6", []string{"testdata/integration/smart-deploy/stage1"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test6", []string{"testdata/integration/smart-deploy/stage2"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("deletes resources", func() {
		deployConfig := utils.DeployConfig{
			DeployType:              smartDeploy,
			ForceDeployWhenNoSemver: true,
			EnsureNamespace:         true,
		}
		It("deletes deployment not in current directory", func() {
			err := doRun(clients, "test5", []string{"testdata/integration/delete-resources/stage1"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test5", []string{"testdata/integration/delete-resources/stage2/"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			_, err = clients.dynamic.Resource(gvrDeployments).
				Namespace("test5").
				Get(context.Background(), "test-deployment-2", metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err))
		})
		It("deletes resource even if secret is in v0 version", func() {
			err := doRun(clients, "test6", []string{"testdata/integration/delete-resources/stage1"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			sec, err := clients.dynamic.Resource(gvrSecrets).
				Namespace("test6").
				Get(context.Background(), "resources-deployed", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			resources := make(map[string]string)
			resources["resources"] = "eyJEZXBsb3ltZW50Ijp7ImtpbmQiOiJEZXBsb3ltZW50IiwiTWFwcGluZyI6eyJHcm91cCI6ImFwcHMiLCJWZXJzaW9uIjoidjEiLCJSZXNvdXJjZSI6ImRlcGxveW1lbnRzIn0sInJlc291cmNlcyI6WyJ0ZXN0LWRlcGxveW1lbnQiLCJ0ZXN0LWRlcGxveW1lbnQtMiJdfX0="
			unstructured.SetNestedStringMap(sec.Object, resources, "data")
			_, err = clients.dynamic.Resource(gvrSecrets).
				Namespace("test6").
				Update(context.Background(), sec, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			err = doRun(clients, "test6", []string{"testdata/integration/delete-resources/stage2"}, deployConfig, currentTime)
			Expect(err).NotTo(HaveOccurred())
			_, err = clients.dynamic.Resource(gvrDeployments).
				Namespace("test5").
				Get(context.Background(), "test-deployment-2", metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err))
		})
	})
})
