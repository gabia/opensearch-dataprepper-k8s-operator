package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dataprepperv1alpha1 "github.com/gabia/dataprepper-operator/api/v1alpha1"
)

// SetupDataPrepperClusterWebhookWithManager registers the webhook for DataPrepperCluster in the manager.
func SetupDataPrepperClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &dataprepperv1alpha1.DataPrepperCluster{}).
		WithValidator(&DataPrepperClusterCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-dataprepper-gabia-com-v1alpha1-datapreppercluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=dataprepper.gabia.com,resources=dataprepperclusters,verbs=create;update,versions=v1alpha1,name=vdatapreppercluster-v1alpha1.kb.io,admissionReviewVersions=v1

// DataPrepperClusterCustomValidator validates DataPrepperCluster on create and update.
// It enforces invariants the spec validators cannot express:
// at least one of spec.image / spec.classRef is set, the referenced class exists,
// and spec.autoscaling.maxReplicas can satisfy spec.replicas.
type DataPrepperClusterCustomValidator struct {
	Client client.Client
}

func (v *DataPrepperClusterCustomValidator) ValidateCreate(ctx context.Context, obj *dataprepperv1alpha1.DataPrepperCluster) (admission.Warnings, error) {
	return nil, v.validate(ctx, obj)
}

func (v *DataPrepperClusterCustomValidator) ValidateUpdate(ctx context.Context, _, newObj *dataprepperv1alpha1.DataPrepperCluster) (admission.Warnings, error) {
	return nil, v.validate(ctx, newObj)
}

func (v *DataPrepperClusterCustomValidator) ValidateDelete(_ context.Context, _ *dataprepperv1alpha1.DataPrepperCluster) (admission.Warnings, error) {
	return nil, nil
}

func (v *DataPrepperClusterCustomValidator) validate(ctx context.Context, c *dataprepperv1alpha1.DataPrepperCluster) error {
	if c.Spec.Image == "" && c.Spec.ClassRef == "" {
		return fmt.Errorf("spec.image or spec.classRef must be set")
	}
	if c.Spec.ClassRef != "" {
		class := &dataprepperv1alpha1.DataPrepperClass{}
		if err := v.Client.Get(ctx, types.NamespacedName{Name: c.Spec.ClassRef}, class); err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("DataPrepperClass %q does not exist", c.Spec.ClassRef)
			}
			return fmt.Errorf("looking up DataPrepperClass %q: %w", c.Spec.ClassRef, err)
		}
	}
	if c.Spec.Autoscaling != nil && c.Spec.Autoscaling.MaxReplicas < c.Spec.Replicas {
		return fmt.Errorf("spec.autoscaling.maxReplicas (%d) must be >= spec.replicas (%d)",
			c.Spec.Autoscaling.MaxReplicas, c.Spec.Replicas)
	}
	return nil
}
