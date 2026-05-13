package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dataprepperv1alpha1 "github.com/gabia/opensearch-dataprepper-k8s-operator/api/v1alpha1"
)

var _ = Describe("DataPrepperCluster Webhook", func() {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	Expect(dataprepperv1alpha1.AddToScheme(scheme)).To(Succeed())

	existingClass := &dataprepperv1alpha1.DataPrepperClass{
		ObjectMeta: metav1.ObjectMeta{Name: "good-class"},
		Spec: dataprepperv1alpha1.DataPrepperClassSpec{
			Image: "opensearchproject/data-prepper:2.15.0",
		},
	}

	validator := func() *DataPrepperClusterCustomValidator {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingClass).Build()
		return &DataPrepperClusterCustomValidator{Client: c}
	}

	cluster := func(spec dataprepperv1alpha1.DataPrepperClusterSpec) *dataprepperv1alpha1.DataPrepperCluster {
		return &dataprepperv1alpha1.DataPrepperCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec:       spec,
		}
	}

	Context("ValidateCreate", func() {
		It("admits a cluster with image set", func() {
			_, err := validator().ValidateCreate(ctx, cluster(dataprepperv1alpha1.DataPrepperClusterSpec{
				Image: "opensearchproject/data-prepper:2.15.0", Replicas: 1,
			}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("admits a cluster with an existing classRef", func() {
			_, err := validator().ValidateCreate(ctx, cluster(dataprepperv1alpha1.DataPrepperClusterSpec{
				ClassRef: "good-class", Replicas: 1,
			}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects when neither image nor classRef is set", func() {
			_, err := validator().ValidateCreate(ctx, cluster(dataprepperv1alpha1.DataPrepperClusterSpec{Replicas: 1}))
			Expect(err).To(MatchError(ContainSubstring("spec.image or spec.classRef must be set")))
		})

		It("rejects when classRef does not exist", func() {
			_, err := validator().ValidateCreate(ctx, cluster(dataprepperv1alpha1.DataPrepperClusterSpec{
				ClassRef: "missing-class", Replicas: 1,
			}))
			Expect(err).To(MatchError(ContainSubstring(`DataPrepperClass "missing-class" does not exist`)))
		})

		It("rejects when autoscaling.maxReplicas is below spec.replicas", func() {
			_, err := validator().ValidateCreate(ctx, cluster(dataprepperv1alpha1.DataPrepperClusterSpec{
				Image:    "x:y",
				Replicas: 5,
				Autoscaling: &dataprepperv1alpha1.AutoscalingSpec{
					MaxReplicas: 3,
				},
			}))
			Expect(err).To(MatchError(ContainSubstring("must be >= spec.replicas")))
		})
	})
})
