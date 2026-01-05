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
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch

// RBAC resources we manage
// We only create RoleBindings (NOT Roles)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch

// Needed to reference ClusterRole "admin" in RoleBindings (RBAC bind check)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,resourceNames=admin,verbs=bind

func (r *TenantNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tn platformv1alpha1.TenantNamespace
	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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

	// 2) Tenant admin RBAC inside namespace (RoleBinding -> ClusterRole/admin)
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
	// FIX: Do not create a wildcard Role (that triggers RBAC escalation protection).
	// Instead, bind tenant admin subjects to the built-in ClusterRole "admin" within this namespace.
	// This yields namespace-admin power without requiring cluster-admin.

	if len(tn.Spec.AdminSubjects) == 0 {
		// Nothing to bind; no-op.
		return nil
	}

	rbName := "tenant-admins"

	desiredSubjects := make([]rbacv1.Subject, 0, len(tn.Spec.AdminSubjects))
	for _, s := range tn.Spec.AdminSubjects {
		sub := rbacv1.Subject{
			Kind: s.Kind,
			Name: s.Name,
		}

		// APIGroup is required for User/Group; must be empty for ServiceAccount
		if s.Kind == "User" || s.Kind == "Group" {
			sub.APIGroup = rbacv1.GroupName // "rbac.authorization.k8s.io"
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
	err := r.Get(ctx, types.NamespacedName{Name: rbName, Namespace: ns}, &rb)
	if apierrors.IsNotFound(err) {
		rb = rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by":  "secure-tenant-operator",
					"platform.upsidedown.io/tenant": tn.Spec.TenantID,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     "admin",
			},
			Subjects: desiredSubjects,
		}
		// Cluster-scoped owner -> namespaced dependent is allowed.
		if err := ctrl.SetControllerReference(tn, &rb, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &rb)
	} else if err != nil {
		return err
	}

	// Drift correction (MVP): overwrite subjects + ensure roleRef stays correct.
	rb.Subjects = desiredSubjects
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "admin",
	}
	if rb.Labels == nil {
		rb.Labels = map[string]string{}
	}
	rb.Labels["app.kubernetes.io/managed-by"] = "secure-tenant-operator"
	rb.Labels["platform.upsidedown.io/tenant"] = tn.Spec.TenantID

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
	if err != nil {
		return err
	}
	return nil
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
	if err != nil {
		return err
	}
	return nil
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
