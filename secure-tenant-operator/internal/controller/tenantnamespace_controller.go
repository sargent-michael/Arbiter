package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/sargent-michael/Kubernetes-Operator/api/v1alpha1"
)

type TenantNamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.upsidedown.io,resources=tenantnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.upsidedown.io,resources=tenantnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.upsidedown.io,resources=tenantnamespaces/finalizers,verbs=update
// Core resources we manage
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles;rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;list;watch;create;update;patch

func (r *TenantNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tn platformv1alpha1.TenantNamespace
	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Align to your current API: spec.tenantID is the tenant identifier.
	tenantID := tn.Spec.TenantID
	if tenantID == "" {
		log.Info("spec.tenantID is empty; waiting for a valid spec")

		apimeta.SetStatusCondition(&tn.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "MissingTenantID",
			Message:            "spec.tenantID must be set",
			LastTransitionTime: metav1.Now(),
		})
		_ = r.Status().Update(ctx, &tn) // best-effort
		return ctrl.Result{}, nil
	}

	targetNS := tn.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = fmt.Sprintf("tenant-%s", tenantID)
	}

	// Baseline toggles (default true)
	networkIsolation := true
	resourceQuota := true
	limitRange := true

	if tn.Spec.BaselinePolicy.NetworkIsolation != nil {
		networkIsolation = *tn.Spec.BaselinePolicy.NetworkIsolation
	}
	if tn.Spec.BaselinePolicy.ResourceQuota != nil {
		resourceQuota = *tn.Spec.BaselinePolicy.ResourceQuota
	}
	if tn.Spec.BaselinePolicy.LimitRange != nil {
		limitRange = *tn.Spec.BaselinePolicy.LimitRange
	}

	// 1) Namespace + labels
	if err := r.ensureNamespace(ctx, &tn, targetNS, tenantID); err != nil {
		return ctrl.Result{}, err
	}

	// 2) Tenant admin RBAC inside namespace
	if err := r.ensureAdminRBAC(ctx, &tn, targetNS); err != nil {
		return ctrl.Result{}, err
	}

	// 3) Quotas/limits
	if resourceQuota {
		if err := r.ensureResourceQuota(ctx, &tn, targetNS); err != nil {
			return ctrl.Result{}, err
		}
	}
	if limitRange {
		if err := r.ensureLimitRange(ctx, &tn, targetNS); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4) Network policies
	if networkIsolation {
		if err := r.ensureNetworkPolicies(ctx, &tn, targetNS); err != nil {
			return ctrl.Result{}, err
		}
	}

	apimeta.SetStatusCondition(&tn.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Tenant namespace and baseline controls are in place",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &tn); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TenantNamespaceReconciler) ensureNamespace(ctx context.Context, tn *platformv1alpha1.TenantNamespace, nsName, tenantID string) error {
	var ns corev1.Namespace
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	if apierrors.IsNotFound(err) {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"platform.upsidedown.io/tenant":      tenantID,
					"pod-security.kubernetes.io/enforce": "baseline",
					"pod-security.kubernetes.io/audit":   "baseline",
					"pod-security.kubernetes.io/warn":    "baseline",
				},
			},
		}
		return r.Create(ctx, &ns)
	}
	if err != nil {
		return err
	}

	desired := map[string]string{
		"platform.upsidedown.io/tenant":      tenantID,
		"pod-security.kubernetes.io/enforce": "baseline",
		"pod-security.kubernetes.io/audit":   "baseline",
		"pod-security.kubernetes.io/warn":    "baseline",
	}

	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	changed := false
	for k, v := range desired {
		if ns.Labels[k] != v {
			ns.Labels[k] = v
			changed = true
		}
	}

	if changed {
		return r.Update(ctx, &ns)
	}
	return nil
}

func (r *TenantNamespaceReconciler) ensureAdminRBAC(ctx context.Context, tn *platformv1alpha1.TenantNamespace, ns string) error {
	// Role: admin within the tenant namespace
	// IMPORTANT: Roles (namespaced) cannot include NonResourceURLs rules.
	roleName := "tenant-admin"

	var role rbacv1.Role
	err := r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: ns}, &role)
	if apierrors.IsNotFound(err) {
		role = rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: ns,
			},
			Rules: []rbacv1.PolicyRule{
				// Namespaced admin (MVP). Tighten later if desired.
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
			},
		}
		if err := ctrl.SetControllerReference(tn, &role, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, &role); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// RoleBinding: binds the subjects provided in spec.adminSubjects
	rbName := "tenant-admin-binding"

	desiredSubjects := make([]rbacv1.Subject, 0, len(tn.Spec.AdminSubjects))
	for _, s := range tn.Spec.AdminSubjects {
		sub := rbacv1.Subject{
			Kind: s.Kind,
			Name: s.Name,
		}

		// APIGroup is required for User/Group; must be empty for ServiceAccount
		if s.Kind == "User" || s.Kind == "Group" {
			sub.APIGroup = "rbac.authorization.k8s.io"
		}

		if s.Kind == "ServiceAccount" {
			sub.Namespace = s.Namespace
			if sub.Namespace == "" {
				sub.Namespace = ns
			}
		}

		desiredSubjects = append(desiredSubjects, sub)
	}

	var rb rbacv1.RoleBinding
	err = r.Get(ctx, types.NamespacedName{Name: rbName, Namespace: ns}, &rb)
	if apierrors.IsNotFound(err) {
		rb = rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: ns,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
			Subjects: desiredSubjects,
		}
		if err := ctrl.SetControllerReference(tn, &rb, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &rb)
	} else if err != nil {
		return err
	}

	// Drift correction (MVP): overwrite subjects
	rb.Subjects = desiredSubjects
	return r.Update(ctx, &rb)
}

func (r *TenantNamespaceReconciler) ensureResourceQuota(ctx context.Context, tn *platformv1alpha1.TenantNamespace, ns string) error {
	name := "tenant-quota"

	var rq corev1.ResourceQuota
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &rq)
	if apierrors.IsNotFound(err) {
		rq = corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					"requests.cpu":    resourceMustParse("2"),
					"requests.memory": resourceMustParse("4Gi"),
					"limits.cpu":      resourceMustParse("4"),
					"limits.memory":   resourceMustParse("8Gi"),
					"pods":            resourceMustParse("50"),
				},
			},
		}
		if err := ctrl.SetControllerReference(tn, &rq, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &rq)
	}
	return err
}

func (r *TenantNamespaceReconciler) ensureLimitRange(ctx context.Context, tn *platformv1alpha1.TenantNamespace, ns string) error {
	name := "tenant-limits"

	var lr corev1.LimitRange
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &lr)
	if apierrors.IsNotFound(err) {
		lr = corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						DefaultRequest: corev1.ResourceList{
							"cpu":    resourceMustParse("100m"),
							"memory": resourceMustParse("128Mi"),
						},
						Default: corev1.ResourceList{
							"cpu":    resourceMustParse("500m"),
							"memory": resourceMustParse("512Mi"),
						},
					},
				},
			},
		}
		if err := ctrl.SetControllerReference(tn, &lr, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &lr)
	}
	return err
}

func (r *TenantNamespaceReconciler) ensureNetworkPolicies(ctx context.Context, tn *platformv1alpha1.TenantNamespace, ns string) error {
	// Default deny all ingress + egress
	deny := &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-deny-all", Namespace: ns},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}
	if err := r.createIfNotExists(ctx, tn, deny); err != nil {
		return err
	}

	// Allow DNS egress to kube-dns in kube-system
	allowDNS := &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-dns-egress", Namespace: ns},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeEgress},
			Egress: []netv1.NetworkPolicyEgressRule{
				{
					To: []netv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"k8s-app": "kube-dns"},
							},
						},
					},
					Ports: []netv1.NetworkPolicyPort{
						{Protocol: protoPtr("UDP"), Port: intstrPtr(53)},
						{Protocol: protoPtr("TCP"), Port: intstrPtr(53)},
					},
				},
			},
		},
	}
	return r.createIfNotExists(ctx, tn, allowDNS)
}

func (r *TenantNamespaceReconciler) createIfNotExists(ctx context.Context, owner client.Object, obj client.Object) error {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	err := r.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) {
		if err := ctrl.SetControllerReference(owner, obj, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, obj)
	}
	return err
}

func (r *TenantNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.TenantNamespace{}).
		Named("tenantnamespace").
		Complete(r)
}
