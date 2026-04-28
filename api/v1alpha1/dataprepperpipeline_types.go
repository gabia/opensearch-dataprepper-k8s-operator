/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DataPrepperPipelineSpec defines the desired state of DataPrepperPipeline
type DataPrepperPipelineSpec struct {
	// clusterRef is the name of the DataPrepperCluster (in the same namespace) this pipeline belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterRef string `json:"clusterRef"`

	// pipelineYaml is the raw pipeline configuration in DataPrepper YAML format.
	// +kubebuilder:validation:MinLength=1
	PipelineYaml string `json:"pipelineYaml"`
}

// DataPrepperPipelinePhase represents the high-level state of a DataPrepperPipeline.
type DataPrepperPipelinePhase string

const (
	DataPrepperPipelinePhasePending DataPrepperPipelinePhase = "Pending"
	DataPrepperPipelinePhaseApplied DataPrepperPipelinePhase = "Applied"
)

// DataPrepperPipelineStatus defines the observed state of DataPrepperPipeline.
type DataPrepperPipelineStatus struct {
	// phase is the high-level state of the pipeline: Pending or Applied.
	// +optional
	Phase DataPrepperPipelinePhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed for this DataPrepperPipeline.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the DataPrepperPipeline resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataPrepperPipeline is the Schema for the dataprepperpipelines API
type DataPrepperPipeline struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of DataPrepperPipeline
	// +required
	Spec DataPrepperPipelineSpec `json:"spec"`

	// status defines the observed state of DataPrepperPipeline
	// +optional
	Status DataPrepperPipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DataPrepperPipelineList contains a list of DataPrepperPipeline
type DataPrepperPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DataPrepperPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPrepperPipeline{}, &DataPrepperPipelineList{})
}
