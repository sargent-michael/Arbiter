package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-project-arbiter-io-v1alpha1-occupant,mutating=true,failurePolicy=ignore,sideEffects=None,groups=project-arbiter.io,resources=occupants,verbs=create;update,versions=v1alpha1,name=moccupant.project-arbiter.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &OccupantDefaulter{}

type OccupantDefaulter struct{}

// Default implements webhook.CustomDefaulter to apply defaults on create/update.
func (d *OccupantDefaulter) Default(_ context.Context, obj runtime.Object) error {
	tn, ok := obj.(*Occupant)
	if !ok {
		return fmt.Errorf("expected *Occupant but got %T", obj)
	}
	if tn.Spec.OccupantID == "" {
		return nil
	}

	if tn.Spec.EnforcementMode == "" {
		tn.Spec.EnforcementMode = "Enforcing"
	}

	if tn.Spec.TargetNamespace == "" && len(tn.Spec.TargetNamespaces) == 0 && len(tn.Spec.AdoptedNamespaces) == 0 {
		tn.Spec.TargetNamespaces = []string{fmt.Sprintf("occupant-%s", tn.Spec.OccupantID)}
	}
	return nil
}

// SetupWebhookWithManager registers the webhook with the manager.
func (r *Occupant) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&OccupantDefaulter{}).
		Complete()
}
