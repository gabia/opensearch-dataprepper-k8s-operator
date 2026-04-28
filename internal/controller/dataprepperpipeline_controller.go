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
	"crypto/sha256"
	"encoding/hex"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dataprepperv1alpha1 "github.com/pkeugine/dataprepper-operator/api/v1alpha1"
)

const pipelineFinalizer = "dataprepper.gabia.com/pipeline-finalizer"
const pipelinesHashAnnotation = "dataprepper.gabia.com/pipelines-hash"

type DataPrepperPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines/finalizers,verbs=update

func (r *DataPrepperPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pipeline := &dataprepperv1alpha1.DataPrepperPipeline{}
	if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	clusterRef := pipeline.Spec.ClusterRef

	// Handle deletion via finalizer
	if !pipeline.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
			log.Info("Pipeline being deleted, rebuilding cluster ConfigMap", "pipeline", pipeline.Name, "cluster", clusterRef)
			if err := r.rebuildClusterConfig(ctx, pipeline.Namespace, clusterRef, pipeline.Name); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pipeline, pipelineFinalizer)
			return ctrl.Result{}, r.Update(ctx, pipeline)
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
		controllerutil.AddFinalizer(pipeline, pipelineFinalizer)
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling DataPrepperPipeline", "pipeline", pipeline.Name, "cluster", clusterRef)
	if err := r.rebuildClusterConfig(ctx, pipeline.Namespace, clusterRef, ""); err != nil {
		return ctrl.Result{}, err
	}

	if pipeline.Status.Phase != dataprepperv1alpha1.DataPrepperPipelinePhaseApplied ||
		pipeline.Status.ObservedGeneration != pipeline.Generation {
		pipeline.Status.Phase = dataprepperv1alpha1.DataPrepperPipelinePhaseApplied
		pipeline.Status.ObservedGeneration = pipeline.Generation
		if err := r.Status().Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// rebuildClusterConfig collects all pipelines targeting the given cluster (excluding excludeName),
// updates the cluster's ConfigMap, and triggers a rolling restart of the cluster's Deployment.
func (r *DataPrepperPipelineReconciler) rebuildClusterConfig(ctx context.Context, namespace, clusterRef, excludeName string) error {
	log := logf.FromContext(ctx)

	pipelineList := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := r.List(ctx, pipelineList, client.InNamespace(namespace)); err != nil {
		return err
	}

	var combined strings.Builder
	for _, p := range pipelineList.Items {
		if p.Name == excludeName {
			continue
		}
		if p.Spec.ClusterRef != clusterRef {
			continue
		}
		if !p.DeletionTimestamp.IsZero() {
			continue
		}
		combined.WriteString(p.Spec.PipelineYaml)
		combined.WriteString("\n")
	}
	content := combined.String()
	if content == "" {
		content = defaultPipelineYaml
	}

	cmName := clusterRef + "-pipelines"
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cmName}, cm); err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["pipelines.yaml"] = content
	if err := r.Update(ctx, cm); err != nil {
		return err
	}
	log.Info("Updated cluster ConfigMap", "configmap", cmName)

	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])[:16]

	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterRef}, deploy); err != nil {
		return err
	}
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = map[string]string{}
	}
	if deploy.Spec.Template.Annotations[pipelinesHashAnnotation] == hashStr {
		return nil
	}
	deploy.Spec.Template.Annotations[pipelinesHashAnnotation] = hashStr
	if err := r.Update(ctx, deploy); err != nil {
		return err
	}
	log.Info("Triggered rolling restart", "deployment", clusterRef, "hash", hashStr)
	return nil
}

func (r *DataPrepperPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dataprepperv1alpha1.DataPrepperPipeline{}).
		Named("dataprepperpipeline").
		Complete(r)
}
