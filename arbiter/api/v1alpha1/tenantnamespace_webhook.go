package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-arbiter-io-v1alpha1-tenantnamespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=arbiter.io,resources=tenantnamespaces,verbs=create;update,versions=v1alpha1,name=mtenantnamespace.arbiter.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &TenantNamespaceDefaulter{}

type TenantNamespaceDefaulter struct{}

// Default implements webhook.CustomDefaulter to apply defaults on create/update.
func (d *TenantNamespaceDefaulter) Default(_ context.Context, obj runtime.Object) error {
	tn, ok := obj.(*TenantNamespace)
	if !ok {
		return fmt.Errorf("expected *TenantNamespace but got %T", obj)
	}
	if tn.Spec.TenantID == "" {
		return nil
	}

	if tn.Spec.TargetNamespace == "" {
		tn.Spec.TargetNamespace = fmt.Sprintf("tenant-%s", tn.Spec.TenantID)
	}

	applyBaselineDefaults(&tn.Spec.BaselinePolicy)
	return nil
}

func applyBaselineDefaults(bp *BaselinePolicy) {

	if bp.NetworkIsolation == nil {
		bp.NetworkIsolation = boolPtr(true)
	}
	if bp.ResourceQuota == nil {
		bp.ResourceQuota = boolPtr(true)
	}
	if bp.LimitRange == nil {
		bp.LimitRange = boolPtr(true)
	}

	networkIsolation := bp.NetworkIsolation != nil && *bp.NetworkIsolation
	resourceQuota := bp.ResourceQuota != nil && *bp.ResourceQuota
	limitRange := bp.LimitRange != nil && *bp.LimitRange

	if networkIsolation && len(bp.AllowedIngressPorts) == 0 {
		bp.AllowedIngressPorts = []int32{443}
	}

	if resourceQuota && bp.ResourceQuotaSpec == nil {
		bp.ResourceQuotaSpec = defaultResourceQuotaSpec()
	}
	if limitRange && bp.LimitRangeSpec == nil {
		bp.LimitRangeSpec = defaultLimitRangeSpec()
	}
}

func defaultResourceQuotaSpec() *corev1.ResourceQuotaSpec {
	return &corev1.ResourceQuotaSpec{
		Hard: corev1.ResourceList{
			"requests.cpu":    resource.MustParse("2"),
			"requests.memory": resource.MustParse("4Gi"),
			"limits.cpu":      resource.MustParse("4"),
			"limits.memory":   resource.MustParse("8Gi"),
			"pods":            resource.MustParse("50"),
		},
	}
}

func defaultLimitRangeSpec() *corev1.LimitRangeSpec {
	return &corev1.LimitRangeSpec{
		Limits: []corev1.LimitRangeItem{
			{
				Type: corev1.LimitTypeContainer,
				DefaultRequest: corev1.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("128Mi"),
				},
				Default: corev1.ResourceList{
					"cpu":    resource.MustParse("500m"),
					"memory": resource.MustParse("512Mi"),
				},
			},
		},
	}
}

func boolPtr(v bool) *bool {
	return &v
}

// SetupWebhookWithManager registers the webhook with the manager.
func (r *TenantNamespace) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&TenantNamespaceDefaulter{}).
		Complete()
}
