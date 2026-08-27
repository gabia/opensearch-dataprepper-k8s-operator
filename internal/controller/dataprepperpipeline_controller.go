package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	dataprepperv1alpha1 "github.com/gabia/opensearch-dataprepper-k8s-operator/api/v1alpha1"
)

const (
	pipelineFinalizer        = "dataprepper.gabia.com/pipeline-finalizer"
	pipelinesHashAnnotation  = "dataprepper.gabia.com/pipelines-hash"
	pipelinesConfigMapKey    = "pipelines.yaml"
	pipelinesConfigMapSuffix = "-pipelines"
)

type DataPrepperPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperpipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=dataprepper.gabia.com,resources=dataprepperclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

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

	if !pipeline.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
			log.Info("Pipeline being deleted, rebuilding cluster ConfigMap", "pipeline", pipeline.Name, "cluster", clusterRef)
			if err := r.rebuildClusterConfig(ctx, pipeline.Namespace, clusterRef, pipeline.Name); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pipeline, pipelineFinalizer)
			return ctrl.Result{}, r.Update(ctx, pipeline)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
		controllerutil.AddFinalizer(pipeline, pipelineFinalizer)
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	cluster := &dataprepperv1alpha1.DataPrepperCluster{}
	err := r.Get(ctx, types.NamespacedName{Namespace: pipeline.Namespace, Name: clusterRef}, cluster)
	if errors.IsNotFound(err) {
		log.Info("Pipeline references missing cluster, marking Pending", "pipeline", pipeline.Name, "cluster", clusterRef)
		return ctrl.Result{}, r.setStatus(ctx, pipeline, dataprepperv1alpha1.DataPrepperPipelinePhasePending, podHealth{})
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciling DataPrepperPipeline", "pipeline", pipeline.Name, "cluster", clusterRef)
	if err := r.rebuildClusterConfig(ctx, pipeline.Namespace, clusterRef, ""); err != nil {
		return ctrl.Result{}, err
	}
	health := r.inspectClusterHealth(ctx, pipeline.Namespace, clusterRef)
	return ctrl.Result{}, r.setStatus(ctx, pipeline, dataprepperv1alpha1.DataPrepperPipelinePhaseApplied, health)
}

type podHealth struct {
	known   bool
	healthy bool
	reason  string
	message string
}

func (r *DataPrepperPipelineReconciler) inspectClusterHealth(ctx context.Context, namespace, clusterName string) podHealth {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(namespace),
		client.MatchingLabels{"app": clusterName},
	); err != nil {
		return podHealth{}
	}
	if len(pods.Items) == 0 {
		return podHealth{known: true, reason: "NoPods", message: "no DataPrepper pods are scheduled"}
	}
	for _, pod := range pods.Items {
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name != "data-prepper" {
				continue
			}
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				return podHealth{
					known:   true,
					reason:  cs.State.Waiting.Reason,
					message: fmt.Sprintf("pod %s waiting: %s", pod.Name, cs.State.Waiting.Message),
				}
			}
			if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
				return podHealth{
					known:   true,
					reason:  cs.State.Terminated.Reason,
					message: fmt.Sprintf("pod %s terminated: %s", pod.Name, cs.State.Terminated.Message),
				}
			}
		}
	}
	return podHealth{known: true, healthy: true, reason: "DataPrepperRunning", message: "all data-prepper containers running"}
}

func (r *DataPrepperPipelineReconciler) setStatus(ctx context.Context, pipeline *dataprepperv1alpha1.DataPrepperPipeline, phase dataprepperv1alpha1.DataPrepperPipelinePhase, health podHealth) error {
	conditions := append([]metav1.Condition{}, pipeline.Status.Conditions...)
	ready := phase == dataprepperv1alpha1.DataPrepperPipelinePhaseApplied

	readyCond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: pipeline.Generation,
	}
	if ready {
		readyCond.Status = metav1.ConditionTrue
		readyCond.Reason = "PipelineApplied"
		readyCond.Message = "pipeline content merged into cluster ConfigMap"
	} else {
		readyCond.Reason = "ClusterNotFound"
		readyCond.Message = fmt.Sprintf("cluster %q not found in namespace", pipeline.Spec.ClusterRef)
	}
	meta.SetStatusCondition(&conditions, readyCond)

	if health.known {
		healthyCond := metav1.Condition{
			Type:               "Healthy",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: pipeline.Generation,
			Reason:             health.reason,
			Message:            health.message,
		}
		if health.healthy {
			healthyCond.Status = metav1.ConditionTrue
		}
		meta.SetStatusCondition(&conditions, healthyCond)
	}

	if pipeline.Status.Phase == phase &&
		pipeline.Status.ObservedGeneration == pipeline.Generation &&
		apiequality.Semantic.DeepEqual(pipeline.Status.Conditions, conditions) {
		return nil
	}
	pipeline.Status.Phase = phase
	pipeline.Status.ObservedGeneration = pipeline.Generation
	pipeline.Status.Conditions = conditions
	return r.Status().Update(ctx, pipeline)
}

// rebuildClusterConfig collects all pipelines targeting the given cluster (excluding excludeName),
// renders them into a ConfigMap, and triggers a rolling restart of the cluster's Deployment.
// Updates are skipped when the resulting content is identical, which prevents reconcile loops
// when this controller watches the same ConfigMap and Deployment.
func (r *DataPrepperPipelineReconciler) rebuildClusterConfig(ctx context.Context, namespace, clusterRef, excludeName string) error {
	log := logf.FromContext(ctx)

	pipelineList := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := r.List(ctx, pipelineList, client.InNamespace(namespace)); err != nil {
		return err
	}

	definitions := make(map[string]json.RawMessage)
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
		if strings.TrimSpace(p.Spec.YAMLKey) == "" {
			return fmt.Errorf("DataPrepperPipeline %q has an empty spec.yamlKey", p.Name)
		}
		if _, exists := definitions[p.Spec.YAMLKey]; exists {
			return fmt.Errorf("duplicate spec.yamlKey %q for DataPrepperCluster %q", p.Spec.YAMLKey, clusterRef)
		}

		var definition map[string]json.RawMessage
		if err := json.Unmarshal(p.Spec.Pipeline.Raw, &definition); err != nil {
			return fmt.Errorf("DataPrepperPipeline %q has an invalid spec.pipeline object: %w", p.Name, err)
		}
		if len(definition) == 0 {
			return fmt.Errorf("DataPrepperPipeline %q has an empty spec.pipeline object", p.Name)
		}
		definitions[p.Spec.YAMLKey] = append(json.RawMessage(nil), p.Spec.Pipeline.Raw...)
	}

	content := defaultPipelineYaml
	if len(definitions) > 0 {
		rendered, err := yaml.Marshal(definitions)
		if err != nil {
			return fmt.Errorf("could not render pipeline definitions: %w", err)
		}
		content = string(rendered)
	}

	cmName := clusterRef + pipelinesConfigMapSuffix
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cmName}, cm); err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	if cm.Data[pipelinesConfigMapKey] != content {
		cm.Data[pipelinesConfigMapKey] = content
		if err := r.Update(ctx, cm); err != nil {
			return err
		}
		log.Info("Updated cluster ConfigMap", "configmap", cmName)
	}

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

// findPipelinesForConfigMap maps a ConfigMap event to all DataPrepperPipelines targeting that
// cluster, so that drift in the ConfigMap data triggers the controller to re-apply.
func (r *DataPrepperPipelineReconciler) findPipelinesForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}
	if !strings.HasSuffix(cm.Name, pipelinesConfigMapSuffix) {
		return nil
	}
	clusterName := strings.TrimSuffix(cm.Name, pipelinesConfigMapSuffix)

	pipelineList := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := r.List(ctx, pipelineList, client.InNamespace(cm.Namespace)); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(pipelineList.Items))
	for _, p := range pipelineList.Items {
		if p.Spec.ClusterRef == clusterName {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: p.Namespace, Name: p.Name},
			})
		}
	}
	return requests
}

// findPipelinesForCluster maps a DataPrepperCluster event to all pipelines that reference it,
// so cluster creation/deletion is reflected in pipeline status promptly.
func (r *DataPrepperPipelineReconciler) findPipelinesForCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	cluster, ok := obj.(*dataprepperv1alpha1.DataPrepperCluster)
	if !ok {
		return nil
	}
	pipelineList := &dataprepperv1alpha1.DataPrepperPipelineList{}
	if err := r.List(ctx, pipelineList, client.InNamespace(cluster.Namespace)); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(pipelineList.Items))
	for _, p := range pipelineList.Items {
		if p.Spec.ClusterRef == cluster.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: p.Namespace, Name: p.Name},
			})
		}
	}
	return requests
}

func (r *DataPrepperPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dataprepperv1alpha1.DataPrepperPipeline{}).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.findPipelinesForConfigMap)).
		Watches(&dataprepperv1alpha1.DataPrepperCluster{}, handler.EnqueueRequestsFromMapFunc(r.findPipelinesForCluster)).
		Named("dataprepperpipeline").
		Complete(r)
}
