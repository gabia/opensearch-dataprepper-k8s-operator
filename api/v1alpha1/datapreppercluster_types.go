package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataPrepperClusterSpec defines the desired state of DataPrepperCluster
type DataPrepperClusterSpec struct {
	// classRef is the name of a DataPrepperClass to use as a template.
	// When set, the class provides the default image and resource requirements.
	// Spec fields on DataPrepperCluster override values from the class.
	// +optional
	ClassRef string `json:"classRef,omitempty"`

	// image is the DataPrepper container image name (e.g. opensearchproject/data-prepper:2.13.0).
	// Required if classRef is not set; otherwise overrides the class image.
	// +optional
	Image string `json:"image,omitempty"`

	// replicas is the desired number of DataPrepper instances.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`

	// serverConfigMap is the name of a ConfigMap (in the same namespace) holding
	// a single key 'data-prepper-config.yaml'. When set, the operator mounts that
	// key over /usr/share/data-prepper/config/data-prepper-config.yaml via subPath.
	// +optional
	ServerConfigMap string `json:"serverConfigMap,omitempty"`

	// assetsConfigMap is the name of a ConfigMap (in the same namespace) whose
	// entries are mounted into /usr/share/data-prepper/assets/. Use this for
	// index templates and ISM policy JSON files referenced from pipeline YAML.
	// +optional
	AssetsConfigMap string `json:"assetsConfigMap,omitempty"`

	// extraVolumes are additional pod-level volumes (ConfigMap, Secret, etc.)
	// merged with the operator-managed pipelines volume.
	// +optional
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// extraVolumeMounts are additional volume mounts for the data-prepper container,
	// merged with the operator-managed pipelines mount.
	// +optional
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// extraPorts are additional container ports to expose on the data-prepper container,
	// merged with the operator-managed http (4900) port.
	// +optional
	ExtraPorts []corev1.ContainerPort `json:"extraPorts,omitempty"`
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
