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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataPrepperClassSpec defines the desired state of DataPrepperClass
type DataPrepperClassSpec struct {
	// image is the DataPrepper container image name (e.g. opensearchproject/data-prepper:2.13.0).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// resources are the default compute resource requirements for DataPrepper containers
	// using this class. Individual DataPrepperCluster resources may override.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataPrepperClass is the Schema for the dataprepperclasses API.
// It is a cluster-scoped template for DataPrepperCluster, similar to how
// StorageClass is a template for PersistentVolumeClaim.
type DataPrepperClass struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of DataPrepperClass
	// +required
	Spec DataPrepperClassSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// DataPrepperClassList contains a list of DataPrepperClass
type DataPrepperClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DataPrepperClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPrepperClass{}, &DataPrepperClassList{})
}
