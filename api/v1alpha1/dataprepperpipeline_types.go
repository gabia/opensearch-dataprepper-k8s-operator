package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DataPrepperPipelineSpec defines the desired state of DataPrepperPipeline
type DataPrepperPipelineSpec struct {
	// clusterRef is the name of the DataPrepperCluster (in the same namespace) this pipeline belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterRef string `json:"clusterRef"`

	// yamlKey is the top-level key used when rendering this pipeline in DataPrepper YAML.
	// +kubebuilder:validation:MinLength=1
	YAMLKey string `json:"yamlKey"`

	// pipeline is the structured definition of one DataPrepper pipeline.
	// +kubebuilder:pruning:PreserveUnknownFields
	Pipeline runtime.RawExtension `json:"pipeline"`
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
