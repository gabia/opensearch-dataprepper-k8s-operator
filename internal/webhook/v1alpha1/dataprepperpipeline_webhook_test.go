package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dataprepperv1alpha1 "github.com/gabia/opensearch-dataprepper-k8s-operator/api/v1alpha1"
)

var _ = Describe("DataPrepperPipeline Webhook", func() {
	ctx := context.Background()
	validator := &DataPrepperPipelineCustomValidator{}

	pipeline := func(yaml string) *dataprepperv1alpha1.DataPrepperPipeline {
		return &dataprepperv1alpha1.DataPrepperPipeline{
			Spec: dataprepperv1alpha1.DataPrepperPipelineSpec{
				ClusterRef:   "test-cluster",
				PipelineYaml: yaml,
			},
		}
	}

	Context("ValidateCreate", func() {
		It("admits a well-formed pipeline", func() {
			yaml := "demo:\n  source:\n    http:\n      port: 2021\n  sink:\n    - stdout:\n"
			_, err := validator.ValidateCreate(ctx, pipeline(yaml))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a pipeline missing source", func() {
			yaml := "broken:\n  sink:\n    - stdout:\n"
			_, err := validator.ValidateCreate(ctx, pipeline(yaml))
			Expect(err).To(MatchError(ContainSubstring(`pipeline "broken" missing required key 'source'`)))
		})

		It("rejects a pipeline missing sink", func() {
			yaml := "broken:\n  source:\n    http:\n      port: 2021\n"
			_, err := validator.ValidateCreate(ctx, pipeline(yaml))
			Expect(err).To(MatchError(ContainSubstring(`pipeline "broken" missing required key 'sink'`)))
		})

		It("rejects malformed YAML", func() {
			_, err := validator.ValidateCreate(ctx, pipeline("not: valid: yaml: ["))
			Expect(err).To(MatchError(ContainSubstring("not valid YAML")))
		})

		It("rejects an empty pipelineYaml", func() {
			_, err := validator.ValidateCreate(ctx, pipeline(""))
			Expect(err).To(MatchError(ContainSubstring("at least one pipeline")))
		})
	})

	Context("ValidateUpdate", func() {
		It("uses the new object's pipelineYaml", func() {
			old := pipeline("old:\n  source:\n    http:\n      port: 2021\n  sink:\n    - stdout:\n")
			updated := pipeline("updated:\n  sink:\n    - stdout:\n")
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(MatchError(ContainSubstring("missing required key 'source'")))
		})
	})

	Context("ValidateDelete", func() {
		It("never blocks deletion", func() {
			_, err := validator.ValidateDelete(ctx, pipeline(""))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
