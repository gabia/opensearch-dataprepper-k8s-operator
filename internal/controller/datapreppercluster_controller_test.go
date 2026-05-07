package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dataprepperv1alpha1 "github.com/gabia/dataprepper-operator/api/v1alpha1"
)

var _ = Describe("DataPrepperCluster Controller", func() {
	Context("When reconciling a cluster", func() {
		const (
			resourceName = "test-cluster"
			testImage    = "opensearchproject/data-prepper:2.15.0"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating a DataPrepperCluster")
			cluster := &dataprepperv1alpha1.DataPrepperCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, cluster)
			if err != nil && errors.IsNotFound(err) {
				cluster = &dataprepperv1alpha1.DataPrepperCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: dataprepperv1alpha1.DataPrepperClusterSpec{
						Image:    testImage,
						Replicas: 1,
					},
				}
				Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("cleaning up resources owned by the cluster")
			cluster := &dataprepperv1alpha1.DataPrepperCluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, cluster)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cluster)).To(Succeed())

			// envtest does not run garbage collection, so child objects must be
			// removed explicitly to keep tests independent.
			for _, obj := range []client.Object{
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-headless", Namespace: "default"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-pipelines", Namespace: "default"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-peer-config", Namespace: "default"}},
			} {
				_ = k8sClient.Delete(ctx, obj)
			}
		})

		It("creates a Deployment, Service, and pipelines ConfigMap", func() {
			reconciler := &DataPrepperClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Deployment was created with the cluster image")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal(testImage))
			Expect(*deploy.Spec.Replicas).To(Equal(int32(1)))

			By("verifying the Service was created with the http port")
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, svc)).To(Succeed())
			Expect(svc.Spec.Ports).To(ContainElement(HaveField("Port", int32(4900))))

			By("verifying the pipelines ConfigMap exists with a placeholder pipeline")
			cm := &corev1.ConfigMap{}
			cmKey := types.NamespacedName{Name: resourceName + "-pipelines", Namespace: "default"}
			Expect(k8sClient.Get(ctx, cmKey, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("pipelines.yaml"))
		})
	})
})
