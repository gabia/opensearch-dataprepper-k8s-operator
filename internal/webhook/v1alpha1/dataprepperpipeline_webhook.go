package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	dataprepperv1alpha1 "github.com/gabia/opensearch-dataprepper-k8s-operator/api/v1alpha1"
)

// SetupDataPrepperPipelineWebhookWithManager registers the webhook for DataPrepperPipeline in the manager.
func SetupDataPrepperPipelineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &dataprepperv1alpha1.DataPrepperPipeline{}).
		WithValidator(&DataPrepperPipelineCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-dataprepper-gabia-com-v1alpha1-dataprepperpipeline,mutating=false,failurePolicy=fail,sideEffects=None,groups=dataprepper.gabia.com,resources=dataprepperpipelines,verbs=create;update,versions=v1alpha1,name=vdataprepperpipeline-v1alpha1.kb.io,admissionReviewVersions=v1

// DataPrepperPipelineCustomValidator validates DataPrepperPipeline pipelineYaml on create and update.
type DataPrepperPipelineCustomValidator struct{}

func (v *DataPrepperPipelineCustomValidator) ValidateCreate(_ context.Context, obj *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, validatePipelineYaml(obj.Spec.PipelineYaml)
}

func (v *DataPrepperPipelineCustomValidator) ValidateUpdate(_ context.Context, _, newObj *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, validatePipelineYaml(newObj.Spec.PipelineYaml)
}

func (v *DataPrepperPipelineCustomValidator) ValidateDelete(_ context.Context, _ *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, nil
}

func validatePipelineYaml(s string) error {
	var pipelines map[string]map[string]any
	if err := yaml.Unmarshal([]byte(s), &pipelines); err != nil {
		return fmt.Errorf("spec.pipelineYaml is not valid YAML: %w", err)
	}
	if len(pipelines) == 0 {
		return fmt.Errorf("spec.pipelineYaml must define at least one pipeline")
	}
	for name, def := range pipelines {
		if _, ok := def["source"]; !ok {
			return fmt.Errorf("pipeline %q missing required key 'source'", name)
		}
		if _, ok := def["sink"]; !ok {
			return fmt.Errorf("pipeline %q missing required key 'sink'", name)
		}
	}
	return nil
}
