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

// DataPrepperPipelineStatus defines the observed state of DataPrepperPipeline.
type DataPrepperPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the DataPrepperPipeline resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
