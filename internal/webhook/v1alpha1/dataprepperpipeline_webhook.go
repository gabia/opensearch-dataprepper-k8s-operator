/*
Copyright 2026 Gabia, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dataprepperv1alpha1 "github.com/gabia/opensearch-dataprepper-k8s-operator/api/v1alpha1"
)

// SetupDataPrepperPipelineWebhookWithManager registers the webhook for DataPrepperPipeline in the manager.
func SetupDataPrepperPipelineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &dataprepperv1alpha1.DataPrepperPipeline{}).
		WithValidator(&DataPrepperPipelineCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-dataprepper-gabia-com-v1alpha1-dataprepperpipeline,mutating=false,failurePolicy=fail,sideEffects=None,groups=dataprepper.gabia.com,resources=dataprepperpipelines,verbs=create;update,versions=v1alpha1,name=vdataprepperpipeline-v1alpha1.kb.io,admissionReviewVersions=v1

// DataPrepperPipelineCustomValidator validates DataPrepperPipeline pipeline definitions on create and update.
type DataPrepperPipelineCustomValidator struct {
	Client client.Client
}

func (v *DataPrepperPipelineCustomValidator) ValidateCreate(ctx context.Context, obj *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, v.validate(ctx, obj)
}

func (v *DataPrepperPipelineCustomValidator) ValidateUpdate(ctx context.Context, _, newObj *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, v.validate(ctx, newObj)
}

func (v *DataPrepperPipelineCustomValidator) ValidateDelete(_ context.Context, _ *dataprepperv1alpha1.DataPrepperPipeline) (admission.Warnings, error) {
	return nil, nil
}

func (v *DataPrepperPipelineCustomValidator) validate(ctx context.Context, pipeline *dataprepperv1alpha1.DataPrepperPipeline) error {
	if err := validatePipelineDefinition(pipeline); err != nil {
		return err
	}
	if v.Client == nil {
		return nil
	}

	pipelines := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := v.Client.List(ctx, pipelines, client.InNamespace(pipeline.Namespace)); err != nil {
		return fmt.Errorf("could not list DataPrepperPipelines: %w", err)
	}
	for _, existing := range pipelines.Items {
		if existing.Name == pipeline.Name {
			continue
		}
		if existing.Spec.ClusterRef == pipeline.Spec.ClusterRef && existing.Spec.YAMLKey == pipeline.Spec.YAMLKey {
			return fmt.Errorf("spec.yamlKey %q is already used by DataPrepperPipeline %q for cluster %q", pipeline.Spec.YAMLKey, existing.Name, pipeline.Spec.ClusterRef)
		}
	}
	return nil
}

func validatePipelineDefinition(pipeline *dataprepperv1alpha1.DataPrepperPipeline) error {
	if strings.TrimSpace(pipeline.Spec.YAMLKey) == "" {
		return fmt.Errorf("spec.yamlKey must not be empty")
	}

	var definition map[string]json.RawMessage
	if err := json.Unmarshal(pipeline.Spec.Pipeline.Raw, &definition); err != nil {
		return fmt.Errorf("spec.pipeline must be an object: %w", err)
	}
	if len(definition) == 0 {
		return fmt.Errorf("spec.pipeline must define at least one field")
	}
	if _, ok := definition["source"]; !ok {
		return fmt.Errorf("spec.pipeline missing required key 'source'")
	}
	if _, ok := definition["sink"]; !ok {
		return fmt.Errorf("spec.pipeline missing required key 'sink'")
	}
	return nil
}
