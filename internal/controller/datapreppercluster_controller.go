package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dataprepperv1alpha1 "github.com/gabia/dataprepper-operator/api/v1alpha1"
)

// resolvedConfig is the effective configuration after merging DataPrepperClass defaults
// with DataPrepperCluster overrides.
type resolvedConfig struct {
	Image          string
	Resources      corev1.ResourceRequirements
	EnvFrom        []corev1.EnvFromSource
	NodeSelector   map[string]string
	Tolerations    []corev1.Toleration
	PodLabels      map[string]string
	PodAnnotations map[string]string
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
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

func (r *DataPrepperClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cluster := &dataprepperv1alpha1.DataPrepperCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cfg, err := r.resolveConfig(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciling DataPrepperCluster", "name", cluster.Name, "image", cfg.Image, "classRef", cluster.Spec.ClassRef, "replicas", cluster.Spec.Replicas)

	if err := r.reconcileConfigMap(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	peerHash, err := r.reconcilePeerConfig(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileDeployment(ctx, cluster, cfg, peerHash); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileService(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileHeadlessService(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileServiceMonitor(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
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

	ready := cluster.Spec.Replicas > 0 && deploy.Status.ReadyReplicas >= cluster.Spec.Replicas
	degraded := deploy.Status.UnavailableReplicas > 0 && deploy.Status.ReadyReplicas == 0
	progressing := deploy.Generation != deploy.Status.ObservedGeneration ||
		(deploy.Spec.Replicas != nil && deploy.Status.UpdatedReplicas < *deploy.Spec.Replicas)

	phase := dataprepperv1alpha1.DataPrepperClusterPhasePending
	switch {
	case ready:
		phase = dataprepperv1alpha1.DataPrepperClusterPhaseReady
	case degraded:
		phase = dataprepperv1alpha1.DataPrepperClusterPhaseDegraded
	}

	conditions := append([]metav1.Condition{}, cluster.Status.Conditions...)
	setCond(&conditions, cluster.Generation, "Ready", ready,
		map[bool]string{true: "DeploymentAvailable", false: "DeploymentUnavailable"},
		map[bool]string{
			true:  fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, cluster.Spec.Replicas),
			false: fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, cluster.Spec.Replicas),
		})
	setCond(&conditions, cluster.Generation, "Progressing", progressing,
		map[bool]string{true: "RolloutInProgress", false: "RolloutComplete"},
		map[bool]string{
			true:  fmt.Sprintf("rollout updating: %d/%d replicas updated", deploy.Status.UpdatedReplicas, cluster.Spec.Replicas),
			false: "deployment matches desired generation",
		})
	setCond(&conditions, cluster.Generation, "Degraded", degraded,
		map[bool]string{true: "AllReplicasUnavailable", false: "ReplicasAvailable"},
		map[bool]string{
			true:  fmt.Sprintf("%d unavailable replicas, none ready", deploy.Status.UnavailableReplicas),
			false: "at least one replica ready",
		})

	desired := dataprepperv1alpha1.DataPrepperClusterStatus{
		Phase:              phase,
		ReadyReplicas:      deploy.Status.ReadyReplicas,
		Pipelines:          pipelineCount,
		ObservedGeneration: cluster.Generation,
		Conditions:         conditions,
	}
	if apiequality.Semantic.DeepEqual(cluster.Status, desired) {
		return nil
	}
	cluster.Status = desired
	return r.Status().Update(ctx, cluster)
}

func setCond(conds *[]metav1.Condition, generation int64, condType string, on bool, reasons, messages map[bool]string) {
	status := metav1.ConditionFalse
	if on {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(conds, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reasons[on],
		Message:            messages[on],
		ObservedGeneration: generation,
	})
}

// resolveConfig merges DataPrepperClass defaults (if classRef is set) with cluster spec overrides.
func (r *DataPrepperClusterReconciler) resolveConfig(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) (*resolvedConfig, error) {
	cfg := &resolvedConfig{
		Image:     cluster.Spec.Image,
		Resources: cluster.Spec.Resources,
		EnvFrom:   cluster.Spec.EnvFrom,
	}
	if cluster.Spec.ClassRef != "" {
		class := &dataprepperv1alpha1.DataPrepperClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ClassRef}, class); err != nil {
			return nil, fmt.Errorf("failed to get DataPrepperClass %q: %w", cluster.Spec.ClassRef, err)
		}
		if cfg.Image == "" {
			cfg.Image = class.Spec.Image
		}
		if isEmptyResources(cfg.Resources) {
			cfg.Resources = class.Spec.Resources
		}
		if len(cfg.EnvFrom) == 0 {
			cfg.EnvFrom = class.Spec.EnvFrom
		}
		cfg.NodeSelector = class.Spec.NodeSelector
		cfg.Tolerations = class.Spec.Tolerations
		cfg.PodLabels = class.Spec.PodLabels
		cfg.PodAnnotations = class.Spec.PodAnnotations
	}
	if cfg.Image == "" {
		return nil, fmt.Errorf("either spec.image or spec.classRef must be set")
	}
	return cfg, nil
}

func isEmptyResources(r corev1.ResourceRequirements) bool {
	return len(r.Requests) == 0 && len(r.Limits) == 0 && len(r.Claims) == 0
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

func (r *DataPrepperClusterReconciler) reconcileDeployment(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster, cfg *resolvedConfig, peerHash string) error {
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

	serverConfigMapName := cluster.Spec.ServerConfigMap
	if serverConfigMapName == "" {
		serverConfigMapName = peerConfigMapName(cluster)
	}
	volumes = append(volumes, corev1.Volume{
		Name: "server-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: serverConfigMapName},
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "server-config",
		MountPath: "/usr/share/data-prepper/config/data-prepper-config.yaml",
		SubPath:   "data-prepper-config.yaml",
	})

	volumes = append(volumes, cluster.Spec.ExtraVolumes...)
	volumeMounts = append(volumeMounts, cluster.Spec.ExtraVolumeMounts...)

	podLabels := mergeStringMap(cfg.PodLabels, map[string]string{"app": cluster.Name})
	podAnnotations := mergeStringMap(cfg.PodAnnotations, nil)
	if peerHash != "" {
		podAnnotations[peerConfigHashAnnotation] = peerHash
	}

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
					Labels:      podLabels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					NodeSelector: cfg.NodeSelector,
					Tolerations:  cfg.Tolerations,
					Containers: []corev1.Container{
						{
							Name:           "data-prepper",
							Image:          cfg.Image,
							Resources:      cfg.Resources,
							Ports:          ports,
							VolumeMounts:   volumeMounts,
							EnvFrom:        cfg.EnvFrom,
							StartupProbe:   tcpProbe(0, 10, 30),
							LivenessProbe:  tcpProbe(0, 20, 3),
							ReadinessProbe: tcpProbe(5, 10, 3),
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
	existing.Spec.Template.Labels = desired.Spec.Template.Labels
	existing.Spec.Template.Annotations = desired.Spec.Template.Annotations
	existing.Spec.Template.Spec.NodeSelector = desired.Spec.Template.Spec.NodeSelector
	existing.Spec.Template.Spec.Tolerations = desired.Spec.Template.Spec.Tolerations
	existing.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
	existing.Spec.Template.Spec.Containers[0].Resources = desired.Spec.Template.Spec.Containers[0].Resources
	existing.Spec.Template.Spec.Containers[0].Ports = desired.Spec.Template.Spec.Containers[0].Ports
	existing.Spec.Template.Spec.Containers[0].VolumeMounts = desired.Spec.Template.Spec.Containers[0].VolumeMounts
	existing.Spec.Template.Spec.Containers[0].EnvFrom = desired.Spec.Template.Spec.Containers[0].EnvFrom
	existing.Spec.Template.Spec.Containers[0].StartupProbe = desired.Spec.Template.Spec.Containers[0].StartupProbe
	existing.Spec.Template.Spec.Containers[0].LivenessProbe = desired.Spec.Template.Spec.Containers[0].LivenessProbe
	existing.Spec.Template.Spec.Containers[0].ReadinessProbe = desired.Spec.Template.Spec.Containers[0].ReadinessProbe
	existing.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
	return r.Update(ctx, existing)
}

// mergeStringMap returns a new map where keys from base are overlaid by overrides.
// nil inputs are treated as empty.
func mergeStringMap(base, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func tcpProbe(initialDelay, period int32, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("http")},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
		FailureThreshold:    failureThreshold,
	}
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

const peerConfigHashAnnotation = "dataprepper.gabia.com/peer-config-hash"

func peerConfigMapName(cluster *dataprepperv1alpha1.DataPrepperCluster) string {
	return cluster.Name + "-peer-config"
}

func headlessServiceName(cluster *dataprepperv1alpha1.DataPrepperCluster) string {
	return cluster.Name + "-headless"
}

// renderPeerConfig builds the data-prepper-config.yaml content for a cluster.
// Single-replica clusters use local_node to skip peer-forwarder bind. Multi-
// replica clusters use dns discovery against the headless Service so peers
// resolve each other through Kubernetes DNS.
func renderPeerConfig(cluster *dataprepperv1alpha1.DataPrepperCluster) string {
	if cluster.Spec.Replicas <= 1 {
		return "ssl: false\npeer_forwarder:\n  discovery_mode: local_node\n"
	}
	domain := fmt.Sprintf("%s.%s.svc.cluster.local", headlessServiceName(cluster), cluster.Namespace)
	return fmt.Sprintf(
		"ssl: false\npeer_forwarder:\n  discovery_mode: dns\n  domain_name: %s\n  ssl: false\n",
		domain,
	)
}

// reconcilePeerConfig manages an operator-owned ConfigMap holding
// data-prepper-config.yaml when the user has not provided spec.serverConfigMap.
// Returns the content hash so the Deployment can roll on changes.
func (r *DataPrepperClusterReconciler) reconcilePeerConfig(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) (string, error) {
	cmName := peerConfigMapName(cluster)
	existing := &corev1.ConfigMap{}
	getErr := r.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cmName}, existing)

	if cluster.Spec.ServerConfigMap != "" {
		if errors.IsNotFound(getErr) {
			return "", nil
		}
		if getErr != nil {
			return "", getErr
		}
		return "", r.Delete(ctx, existing)
	}

	content := renderPeerConfig(cluster)
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])[:16]

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: cluster.Namespace},
		Data:       map[string]string{"data-prepper-config.yaml": content},
	}
	if err := ctrl.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return "", err
	}
	if errors.IsNotFound(getErr) {
		return hashStr, r.Create(ctx, desired)
	}
	if getErr != nil {
		return "", getErr
	}
	if existing.Data["data-prepper-config.yaml"] == content {
		return hashStr, nil
	}
	existing.Data = desired.Data
	return hashStr, r.Update(ctx, existing)
}

// reconcileHeadlessService keeps a clusterIP=None Service so DataPrepper peers
// can resolve each other via DNS in multi-replica deployments. The Service
// always exists (the cost is negligible) so a 1->N replicas change does not
// require additional plumbing.
func (r *DataPrepperClusterReconciler) reconcileHeadlessService(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessServiceName(cluster),
			Namespace: cluster.Namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
			Selector:                 map[string]string{"app": cluster.Name},
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
	if existing.Spec.ClusterIP == "None" &&
		existing.Spec.PublishNotReadyAddresses == desired.Spec.PublishNotReadyAddresses &&
		apiequality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		return nil
	}
	existing.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
	existing.Spec.Selector = desired.Spec.Selector
	return r.Update(ctx, existing)
}

var serviceMonitorGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitor",
}

// reconcileServiceMonitor creates or removes a ServiceMonitor based on
// spec.metrics.serviceMonitor. The ServiceMonitor CRD is optional; when it
// is not installed the reconcile is a silent no-op.
func (r *DataPrepperClusterReconciler) reconcileServiceMonitor(ctx context.Context, cluster *dataprepperv1alpha1.DataPrepperCluster) error {
	want := cluster.Spec.Metrics != nil && cluster.Spec.Metrics.ServiceMonitor

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(serviceMonitorGVK)
	desired.SetName(cluster.Name)
	desired.SetNamespace(cluster.Namespace)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(serviceMonitorGVK)
	getErr := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if meta.IsNoMatchError(getErr) {
		return nil
	}

	if !want {
		if errors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		return r.Delete(ctx, existing)
	}

	spec := map[string]any{
		"selector": map[string]any{
			"matchLabels": map[string]any{"app": cluster.Name},
		},
		"endpoints": []any{
			map[string]any{
				"port": "http",
				"path": "/metrics/prometheus",
			},
		},
	}
	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(getErr) {
		err := r.Create(ctx, desired)
		if meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}
	if getErr != nil {
		return getErr
	}

	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	if apiequality.Semantic.DeepEqual(existingSpec, spec) {
		return nil
	}
	if err := unstructured.SetNestedMap(existing.Object, spec, "spec"); err != nil {
		return err
	}
	return r.Update(ctx, existing)
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
