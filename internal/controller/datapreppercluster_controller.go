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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dataprepperv1alpha1 "github.com/pkeugine/dataprepper-operator/api/v1alpha1"
)

// resolvedConfig is the effective configuration after merging DataPrepperClass defaults
// with DataPrepperCluster overrides.
type resolvedConfig struct {
	Image     string
	Resources corev1.ResourceRequirements
}

// DataPrepperClusterReconciler reconciles a DataPrepperCluster object
type DataPrepperClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps;services,verbs=get;list;watch;create;update;patch

func (r *DataPrepperClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. DataPrepperCluster CR 가져오기
	cluster := &dataprepperv1alpha1.DataPrepperCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Resolve image/resources from optional DataPrepperClass
	cfg, err := r.resolveConfig(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciling DataPrepperCluster", "name", cluster.Name, "image", cfg.Image, "classRef", cluster.Spec.ClassRef)

	// 3. ConfigMap (파이프라인 설정 파일)
	if err := r.reconcileConfigMap(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// 4. Deployment (DataPrepper Pod)
	if err := r.reconcileDeployment(ctx, cluster, cfg); err != nil {
		return ctrl.Result{}, err
	}

	// 5. Service (HTTP 포트 노출)
	if err := r.reconcileService(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// 6. Status 업데이트
	if err := r.reconcileStatus(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DataPrepperClusterReconciler) reconcileStatus(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) error {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}, deploy); err != nil {
		return err
	}

	pipelineList := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := r.List(ctx, pipelineList, client.InNamespace(cluster.Namespace)); err != nil {
		return err
	}
	pipelineCount := int32(0)
	for _, p := range pipelineList.Items {
		if p.Spec.ClusterRef == cluster.Name && p.DeletionTimestamp.IsZero() {
			pipelineCount++
		}
	}

	phase := dataprepperv1alpha1.DataPrepperClusterPhasePending
	if deploy.Status.ReadyReplicas >= cluster.Spec.Replicas && cluster.Spec.Replicas > 0 {
		phase = dataprepperv1alpha1.DataPrepperClusterPhaseReady
	} else if deploy.Status.UnavailableReplicas > 0 && deploy.Status.ReadyReplicas == 0 {
		phase = dataprepperv1alpha1.DataPrepperClusterPhaseDegraded
	}

	desired := dataprepperv1alpha1.DataPrepperClusterStatus{
		Phase:              phase,
		ReadyReplicas:      deploy.Status.ReadyReplicas,
		Pipelines:          pipelineCount,
		ObservedGeneration: cluster.Generation,
		Conditions:         cluster.Status.Conditions,
	}
	if apiequality.Semantic.DeepEqual(cluster.Status, desired) {
		return nil
	}
	cluster.Status = desired
	return r.Status().Update(ctx, cluster)
}

// resolveConfig merges DataPrepperClass defaults (if classRef is set) with cluster spec overrides.
func (r *DataPrepperClusterReconciler) resolveConfig(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) (*resolvedConfig, error) {
	cfg := &resolvedConfig{
		Image: cluster.Spec.Image,
	}
	if cluster.Spec.ClassRef != "" {
		class := &dataprepperv1alpha1.DataPrepperClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ClassRef}, class); err != nil {
			return nil, fmt.Errorf("failed to get DataPrepperClass %q: %w", cluster.Spec.ClassRef, err)
		}
		if cfg.Image == "" {
			cfg.Image = class.Spec.Image
		}
		cfg.Resources = class.Spec.Resources
	}
	if cfg.Image == "" {
		return nil, fmt.Errorf("either spec.image or spec.classRef must be set")
	}
	return cfg, nil
}

// defaultPipelineYaml is the placeholder pipeline used until a DataPrepperPipeline CR is created.
// DataPrepper requires at least one valid pipeline to start.
const defaultPipelineYaml = `placeholder-pipeline:
  source:
    http:
      port: 2021
  sink:
    - stdout:
`

func (r *DataPrepperClusterReconciler) reconcileConfigMap(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) error {
	cmName := cluster.Name + "-pipelines"
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cmName}, existing)
	if err == nil {
		// ConfigMap exists - DataPrepperPipeline controller manages its data.
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cluster.Namespace,
		},
		Data: map[string]string{
			"pipelines.yaml": defaultPipelineYaml,
		},
	}
	if err := ctrl.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, desired)
}

func (r *DataPrepperClusterReconciler) reconcileDeployment(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster, cfg *resolvedConfig) error {
	replicas := cluster.Spec.Replicas

	ports := append([]corev1.ContainerPort{
		{Name: "http", ContainerPort: 4900},
	}, cluster.Spec.ExtraPorts...)

	volumes := []corev1.Volume{
		{
			Name: "pipelines",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cluster.Name + "-pipelines",
					},
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{Name: "pipelines", MountPath: "/usr/share/data-prepper/pipelines"},
	}

	if cluster.Spec.AssetsConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "assets",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cluster.Spec.AssetsConfigMap},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "assets",
			MountPath: "/usr/share/data-prepper/assets",
		})
	}

	if cluster.Spec.ServerConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "server-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cluster.Spec.ServerConfigMap},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "server-config",
			MountPath: "/usr/share/data-prepper/config/data-prepper-config.yaml",
			SubPath:   "data-prepper-config.yaml",
		})
	}

	volumes = append(volumes, cluster.Spec.ExtraVolumes...)
	volumeMounts = append(volumeMounts, cluster.Spec.ExtraVolumeMounts...)

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": cluster.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": cluster.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:         "data-prepper",
							Image:        cfg.Image,
							Resources:    cfg.Resources,
							Ports:        ports,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
	existing.Spec.Template.Spec.Containers[0].Resources = desired.Spec.Template.Spec.Containers[0].Resources
	existing.Spec.Template.Spec.Containers[0].Ports = desired.Spec.Template.Spec.Containers[0].Ports
	existing.Spec.Template.Spec.Containers[0].VolumeMounts = desired.Spec.Template.Spec.Containers[0].VolumeMounts
	existing.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
	return r.Update(ctx, existing)
}

func (r *DataPrepperClusterReconciler) reconcileService(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": cluster.Name},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 4900, TargetPort: intstr.FromString("http")},
			},
		},
	}
	if err := ctrl.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !apiequality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!apiequality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Selector = desired.Spec.Selector
		return r.Update(ctx, existing)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataPrepperClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dataprepperv1alpha1.DataPrepperCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Named("datapreppercluster").
		Complete(r)
}
