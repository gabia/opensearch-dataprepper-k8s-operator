package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dataprepperv1alpha1 "github.com/gabia/dataprepper-operator/api/v1alpha1"
)

var _ = Describe("DataPrepperPipeline Controller", func() {
	Context("When a pipeline targets an existing cluster", func() {
		const (
			clusterName  = "pipe-test-cluster"
			pipelineName = "pipe-test-pipeline"
			testImage    = "opensearchproject/data-prepper:2.15.0"
			pipelineYaml = "demo-pipeline:\n  source:\n    http:\n      port: 2021\n  sink:\n    - stdout:\n"
		)

		ctx := context.Background()
		clusterKey := types.NamespacedName{Name: clusterName, Namespace: "default"}
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: "default"}

		BeforeEach(func() {
			By("creating a DataPrepperCluster and reconciling it once so the ConfigMap exists")
			cluster := &dataprepperv1alpha1.DataPrepperCluster{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: "default"},
				Spec: dataprepperv1alpha1.DataPrepperClusterSpec{
					Image:    testImage,
					Replicas: 1,
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			clusterReconciler := &DataPrepperClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := clusterReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
			Expect(err).NotTo(HaveOccurred())

			By("creating a DataPrepperPipeline targeting the cluster")
			pipeline := &dataprepperv1alpha1.DataPrepperPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: pipelineName, Namespace: "default"},
				Spec: dataprepperv1alpha1.DataPrepperPipelineSpec{
					ClusterRef:   clusterName,
					PipelineYaml: pipelineYaml,
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up cluster, pipeline, and owned objects")
			pipeline := &dataprepperv1alpha1.DataPrepperPipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				// Drop the finalizer so envtest (no controller manager loop) can delete.
				pipeline.Finalizers = nil
				_ = k8sClient.Update(ctx, pipeline)
				_ = k8sClient.Delete(ctx, pipeline)
			}
			cluster := &dataprepperv1alpha1.DataPrepperCluster{}
			if err := k8sClient.Get(ctx, clusterKey, cluster); err == nil {
				_ = k8sClient.Delete(ctx, cluster)
			}
			for _, obj := range []client.Object{
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: "default"}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: "default"}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: clusterName + "-headless", Namespace: "default"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterName + "-pipelines", Namespace: "default"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterName + "-peer-config", Namespace: "default"}},
			} {
				_ = k8sClient.Delete(ctx, obj)
			}
		})

		It("merges pipeline content into the cluster ConfigMap and triggers a rolling restart", func() {
			reconciler := &DataPrepperPipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("reconciling once to attach the finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: pipelineKey})
			Expect(err).NotTo(HaveOccurred())

			By("reconciling again to apply the pipeline content")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: pipelineKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the cluster ConfigMap contains the pipeline YAML")
			cm := &corev1.ConfigMap{}
			cmKey := types.NamespacedName{Name: clusterName + "-pipelines", Namespace: "default"}
			Expect(k8sClient.Get(ctx, cmKey, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("pipelines.yaml"))
			Expect(cm.Data["pipelines.yaml"]).To(ContainSubstring("demo-pipeline:"))

			By("verifying the Deployment has a content-hash annotation")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, clusterKey, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Annotations).To(HaveKey(pipelinesHashAnnotation))

			By("verifying the pipeline status moved to Applied")
			pipeline := &dataprepperv1alpha1.DataPrepperPipeline{}
			Expect(k8sClient.Get(ctx, pipelineKey, pipeline)).To(Succeed())
			Expect(pipeline.Status.Phase).To(Equal(dataprepperv1alpha1.DataPrepperPipelinePhaseApplied))
		})
	})
})
