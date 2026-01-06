package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-project-arbiter-io-v1alpha1-project,mutating=true,failurePolicy=ignore,sideEffects=None,groups=project-arbiter.io,resources=projects,verbs=create;update,versions=v1alpha1,name=mproject.project-arbiter.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &ProjectDefaulter{}

type ProjectDefaulter struct{}

// Default implements webhook.CustomDefaulter to apply defaults on create/update.
func (d *ProjectDefaulter) Default(_ context.Context, obj runtime.Object) error {
	tn, ok := obj.(*Project)
	if !ok {
		return fmt.Errorf("expected *Project but got %T", obj)
	}
	if tn.Spec.ProjectID == "" {
		return nil
	}

	if tn.Spec.TargetNamespace == "" && len(tn.Spec.TargetNamespaces) == 0 {
		tn.Spec.TargetNamespaces = []string{fmt.Sprintf("project-%s", tn.Spec.ProjectID)}
	}
	return nil
}

// SetupWebhookWithManager registers the webhook with the manager.
func (r *Project) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&ProjectDefaulter{}).
		Complete()
}
