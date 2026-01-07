package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-project-arbiter-io-v1alpha1-settler,mutating=true,failurePolicy=ignore,sideEffects=None,groups=project-arbiter.io,resources=settlers,verbs=create;update,versions=v1alpha1,name=msettler.project-arbiter.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &SettlerDefaulter{}

type SettlerDefaulter struct{}

// Default implements webhook.CustomDefaulter to apply defaults on create/update.
func (d *SettlerDefaulter) Default(_ context.Context, obj runtime.Object) error {
	tn, ok := obj.(*Settler)
	if !ok {
		return fmt.Errorf("expected *Settler but got %T", obj)
	}
	if tn.Spec.SettlerID == "" {
		return nil
	}

	if tn.Spec.TargetNamespace == "" && len(tn.Spec.TargetNamespaces) == 0 {
		tn.Spec.TargetNamespaces = []string{fmt.Sprintf("settler-%s", tn.Spec.SettlerID)}
	}
	return nil
}

// SetupWebhookWithManager registers the webhook with the manager.
func (r *Settler) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&SettlerDefaulter{}).
		Complete()
}
