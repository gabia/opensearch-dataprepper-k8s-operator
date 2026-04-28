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

// DataPrepperClusterSpec defines the desired state of DataPrepperCluster
type DataPrepperClusterSpec struct {
	// image is the DataPrepper container image name (e.g. opensearchproject/data-prepper:2.13.0).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// replicas is the desired number of DataPrepper instances.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`
}

// DataPrepperClusterPhase represents the high-level state of a DataPrepperCluster.
type DataPrepperClusterPhase string

const (
	DataPrepperClusterPhasePending  DataPrepperClusterPhase = "Pending"
	DataPrepperClusterPhaseReady    DataPrepperClusterPhase = "Ready"
	DataPrepperClusterPhaseDegraded DataPrepperClusterPhase = "Degraded"
)

// DataPrepperClusterStatus defines the observed state of DataPrepperCluster.
type DataPrepperClusterStatus struct {
	// phase is the high-level state of the cluster: Pending, Ready, or Degraded.
	// +optional
	Phase DataPrepperClusterPhase `json:"phase,omitempty"`

	// readyReplicas is the number of DataPrepper pods that are ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// pipelines is the number of DataPrepperPipeline resources targeting this cluster.
	// +optional
	Pipelines int32 `json:"pipelines,omitempty"`

	// observedGeneration is the most recent generation observed for this DataPrepperCluster.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the DataPrepperCluster resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Pipelines",type=integer,JSONPath=`.status.pipelines`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataPrepperCluster is the Schema for the dataprepperclusters API
type DataPrepperCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of DataPrepperCluster
	// +required
	Spec DataPrepperClusterSpec `json:"spec"`

	// status defines the observed state of DataPrepperCluster
	// +optional
	Status DataPrepperClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DataPrepperClusterList contains a list of DataPrepperCluster
type DataPrepperClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DataPrepperCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPrepperCluster{}, &DataPrepperClusterList{})
}
