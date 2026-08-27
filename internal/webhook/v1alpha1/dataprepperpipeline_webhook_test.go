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

var _ = Describe("DataPrepperPipeline Webhook", func() {
	ctx := context.Background()
	validator := &DataPrepperPipelineCustomValidator{}

	pipeline := func(yamlKey, definition string) *dataprepperv1alpha1.DataPrepperPipeline {
		return &dataprepperv1alpha1.DataPrepperPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: "default"},
			Spec: dataprepperv1alpha1.DataPrepperPipelineSpec{
				ClusterRef: "test-cluster",
				YAMLKey:    yamlKey,
				Pipeline:   runtime.RawExtension{Raw: []byte(definition)},
			},
		}
	}

	const validDefinition = `{"source":{"http":{"port":2021}},"sink":[{"stdout":null}]}`

	Context("ValidateCreate", func() {
		It("admits a well-formed pipeline", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("demo", validDefinition))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a pipeline missing source", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("broken", `{"sink":[{"stdout":null}]}`))
			Expect(err).To(MatchError(ContainSubstring("spec.pipeline missing required key 'source'")))
		})

		It("rejects a pipeline missing sink", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("broken", `{"source":{"http":{"port":2021}}}`))
			Expect(err).To(MatchError(ContainSubstring("spec.pipeline missing required key 'sink'")))
		})

		It("rejects a non-object pipeline", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("broken", `[]`))
			Expect(err).To(MatchError(ContainSubstring("spec.pipeline must be an object")))
		})

		It("rejects an empty pipeline definition", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("empty", `{}`))
			Expect(err).To(MatchError(ContainSubstring("spec.pipeline must define at least one field")))
		})

		It("rejects duplicate yamlKey values for a cluster", func() {
			existing := pipeline("demo", validDefinition)
			existing.Name = "existing-pipeline"
			scheme := runtime.NewScheme()
			Expect(dataprepperv1alpha1.AddToScheme(scheme)).To(Succeed())
			validator := &DataPrepperPipelineCustomValidator{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
			}

			candidate := pipeline("demo", validDefinition)
			candidate.Name = "candidate-pipeline"
			_, err := validator.ValidateCreate(ctx, candidate)
			Expect(err).To(MatchError(ContainSubstring(`spec.yamlKey "demo" is already used by DataPrepperPipeline "existing-pipeline"`)))
		})
	})

	Context("ValidateUpdate", func() {
		It("uses the new object's pipeline definition", func() {
			old := pipeline("demo", validDefinition)
			updated := pipeline("demo", `{"sink":[{"stdout":null}]}`)
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(MatchError(ContainSubstring("missing required key 'source'")))
		})
	})

	Context("ValidateDelete", func() {
		It("never blocks deletion", func() {
			_, err := validator.ValidateDelete(ctx, pipeline("", ""))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
