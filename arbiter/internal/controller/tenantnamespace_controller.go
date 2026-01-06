package controller

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/prometheus/client_golang/prometheus"

	platformv1alpha1 "github.com/sargent-michael/Kubernetes-Operator/api/v1alpha1"
)

type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const projectFinalizer = "project-arbiter.io/project-cleanup"
const projectLabelKey = "project-arbiter.io/project"
const defaultBaselineName = "default"

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "arbiter_reconcile_total",
			Help: "Total number of reconciliations.",
		},
		[]string{"controller", "result"},
	)
	reconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "arbiter_reconcile_errors_total",
			Help: "Total number of reconciliation errors.",
		},
		[]string{"controller"},
	)
	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "arbiter_reconcile_duration_seconds",
			Help:    "Reconciliation duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller", "result"},
	)
)

func init() {
	metrics.Registry.MustRegister(reconcileTotal, reconcileErrors, reconcileDuration)
}

// +kubebuilder:rbac:groups=project-arbiter.io,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=project-arbiter.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=project-arbiter.io,resources=projects/finalizers,verbs=update

// Core resources we manage
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch

// RBAC resources we manage
// We only create RoleBindings (NOT Roles)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch

// Needed to reference ClusterRole "admin" in RoleBindings (RBAC bind check)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,resourceNames=admin,verbs=bind

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx)
	start := time.Now()
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
			reconcileErrors.WithLabelValues("project").Inc()
		}
		reconcileTotal.WithLabelValues("project", outcome).Inc()
		reconcileDuration.WithLabelValues("project", outcome).Observe(time.Since(start).Seconds())
	}()

	var tn platformv1alpha1.Project
	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tn.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&tn, projectFinalizer) {
			done, err := r.finalizeProject(ctx, &tn)
			if err != nil {
				return ctrl.Result{}, err
			}
			if !done {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			controllerutil.RemoveFinalizer(&tn, projectFinalizer)
			if err := r.Update(ctx, &tn); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
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

	targetNamespaces := collectTargetNamespaces(&tn)
	if len(targetNamespaces) == 0 {
		log.Info("no target namespaces resolved; waiting for a valid spec")
		apimeta.SetStatusCondition(&tn.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "MissingTargetNamespaces",
			Message:            "spec.targetNamespaces or spec.targetNamespace must be set",
			LastTransitionTime: metav1.Now(),
		})
		_ = r.Status().Update(ctx, &tn) // best-effort
		return ctrl.Result{}, nil
	}

	policy, err := r.resolveBaselinePolicy(ctx, &tn)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(&tn, projectFinalizer) {
		controllerutil.AddFinalizer(&tn, projectFinalizer)
		if err := r.Update(ctx, &tn); err != nil {
			return ctrl.Result{}, err
		}
	}

	for _, targetNS := range targetNamespaces {
		// 1) Namespace + labels
		if err := r.ensureNamespace(ctx, &tn, targetNS, tenantID); err != nil {
			return ctrl.Result{}, err
		}

		// 2) Tenant admin RBAC inside namespace (RoleBinding -> ClusterRole/admin)
		if err := r.ensureAdminRBAC(ctx, &tn, targetNS); err != nil {
			return ctrl.Result{}, err
		}

		// 3) Quotas/limits
		if policy.ResourceQuota != nil && *policy.ResourceQuota {
			if err := r.ensureResourceQuota(ctx, &tn, targetNS, policy); err != nil {
				return ctrl.Result{}, err
			}
		}
		if policy.LimitRange != nil && *policy.LimitRange {
			if err := r.ensureLimitRange(ctx, &tn, targetNS, policy); err != nil {
				return ctrl.Result{}, err
			}
		}

		// 4) Network policies
		if policy.NetworkIsolation != nil && *policy.NetworkIsolation {
			if err := r.ensureNetworkPolicies(ctx, &tn, targetNS, policy); err != nil {
				return ctrl.Result{}, err
			}
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

func (r *ProjectReconciler) finalizeProject(ctx context.Context, tn *platformv1alpha1.Project) (bool, error) {
	tenantID := tn.Spec.TenantID
	targetNamespaces := collectTargetNamespaces(tn)
	if len(targetNamespaces) == 0 && tenantID != "" {
		var nsList corev1.NamespaceList
		if err := r.List(ctx, &nsList, client.MatchingLabels{projectLabelKey: tenantID}); err != nil {
			return false, err
		}
		for _, ns := range nsList.Items {
			targetNamespaces = append(targetNamespaces, ns.Name)
		}
	}

	if len(targetNamespaces) == 0 {
		return true, nil
	}

	done := true
	for _, targetNS := range targetNamespaces {
		var ns corev1.Namespace
		err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if ns.DeletionTimestamp != nil {
			done = false
			continue
		}
		if err := r.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
		done = false
	}
	return done, nil
}

func (r *ProjectReconciler) ensureNamespace(ctx context.Context, tn *platformv1alpha1.Project, nsName, tenantID string) error {
	var ns corev1.Namespace
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	if apierrors.IsNotFound(err) {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					projectLabelKey:                             tenantID,
					"pod-security.kubernetes.io/enforce": "baseline",
					"pod-security.kubernetes.io/audit":   "baseline",
					"pod-security.kubernetes.io/warn":    "baseline",
				},
			},
		}
		if err := ctrl.SetControllerReference(tn, &ns, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &ns)
	}
	if err != nil {
		return err
	}

	desired := map[string]string{
		projectLabelKey:                             tenantID,
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

	if !metav1.IsControlledBy(&ns, tn) {
		if err := ctrl.SetControllerReference(tn, &ns, r.Scheme); err != nil {
			return err
		}
		changed = true
	}

	if changed {
		return r.Update(ctx, &ns)
	}
	return nil
}

func (r *ProjectReconciler) ensureAdminRBAC(ctx context.Context, tn *platformv1alpha1.Project, ns string) error {
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
					"app.kubernetes.io/managed-by": "arbiter",
					projectLabelKey:                      tn.Spec.TenantID,
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
	rb.Labels["app.kubernetes.io/managed-by"] = "arbiter"
	rb.Labels[projectLabelKey] = tn.Spec.TenantID

	return r.Update(ctx, &rb)
}

func (r *ProjectReconciler) ensureResourceQuota(ctx context.Context, tn *platformv1alpha1.Project, ns string, policy platformv1alpha1.BaselinePolicy) error {
	name := "tenant-quota"
	desiredSpec := defaultResourceQuotaSpec()
	if policy.ResourceQuotaSpec != nil {
		desiredSpec = *policy.ResourceQuotaSpec
	}

	var rq corev1.ResourceQuota
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &rq)
	if apierrors.IsNotFound(err) {
		rq = corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       desiredSpec,
		}
		if err := ctrl.SetControllerReference(tn, &rq, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &rq)
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rq.Spec, desiredSpec) {
		rq.Spec = desiredSpec
		return r.Update(ctx, &rq)
	}
	return nil
}

func (r *ProjectReconciler) ensureLimitRange(ctx context.Context, tn *platformv1alpha1.Project, ns string, policy platformv1alpha1.BaselinePolicy) error {
	name := "tenant-limits"
	desiredSpec := defaultLimitRangeSpec()
	if policy.LimitRangeSpec != nil {
		desiredSpec = *policy.LimitRangeSpec
	}

	var lr corev1.LimitRange
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &lr)
	if apierrors.IsNotFound(err) {
		lr = corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       desiredSpec,
		}
		if err := ctrl.SetControllerReference(tn, &lr, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, &lr)
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(lr.Spec, desiredSpec) {
		lr.Spec = desiredSpec
		return r.Update(ctx, &lr)
	}
	return nil
}

func (r *ProjectReconciler) ensureNetworkPolicies(ctx context.Context, tn *platformv1alpha1.Project, ns string, policy platformv1alpha1.BaselinePolicy) error {
	allowedIngressPorts := policy.AllowedIngressPorts
	if len(allowedIngressPorts) == 0 {
		allowedIngressPorts = []int32{443}
	}
	ingressPorts := make([]netv1.NetworkPolicyPort, 0, len(allowedIngressPorts))
	for _, port := range allowedIngressPorts {
		ingressPorts = append(ingressPorts, netv1.NetworkPolicyPort{
			Protocol: protoPtr("TCP"),
			Port:     intstrPtr(int(port)),
		})
	}

	// Default deny all ingress + egress
	deny := &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-deny-all", Namespace: ns},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}
	if err := r.ensureNetworkPolicy(ctx, tn, deny); err != nil {
		return err
	}

	// Allow ingress from anywhere on allowed TCP ports
	allowHTTPSIngress := &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-https-ingress", Namespace: ns},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			Ingress: []netv1.NetworkPolicyIngressRule{
				{
					Ports: ingressPorts,
				},
			},
		},
	}
	if err := r.ensureNetworkPolicy(ctx, tn, allowHTTPSIngress); err != nil {
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
	return r.ensureNetworkPolicy(ctx, tn, allowDNS)
}

func defaultResourceQuotaSpec() corev1.ResourceQuotaSpec {
	return corev1.ResourceQuotaSpec{
		Hard: corev1.ResourceList{
			"requests.cpu":    resourceMustParse("2"),
			"requests.memory": resourceMustParse("4Gi"),
			"limits.cpu":      resourceMustParse("4"),
			"limits.memory":   resourceMustParse("8Gi"),
			"pods":            resourceMustParse("50"),
		},
	}
}

func defaultLimitRangeSpec() corev1.LimitRangeSpec {
	return corev1.LimitRangeSpec{
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
	}
}

func (r *ProjectReconciler) ensureNetworkPolicy(ctx context.Context, owner client.Object, desired *netv1.NetworkPolicy) error {
	var current netv1.NetworkPolicy
	key := types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}
	if err := r.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			if err := ctrl.SetControllerReference(owner, desired, r.Scheme); err != nil {
				return err
			}
			return r.Create(ctx, desired)
		}
		return err
	}

	if reflect.DeepEqual(current.Spec, desired.Spec) {
		return nil
	}

	current.Spec = desired.Spec
	return r.Update(ctx, &current)
}

func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Project{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&netv1.NetworkPolicy{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&platformv1alpha1.Baseline{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
			var list platformv1alpha1.ProjectList
			if err := r.List(ctx, &list); err != nil {
				return nil
			}
			requests := make([]reconcile.Request, 0, len(list.Items))
			for _, item := range list.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: item.Name},
				})
			}
			return requests
		})).
		Named("project").
		Complete(r)
}

func (r *ProjectReconciler) resolveBaselinePolicy(ctx context.Context, tn *platformv1alpha1.Project) (platformv1alpha1.BaselinePolicy, error) {
	policy := defaultBaselinePolicy()

	baseline, err := r.getBaseline(ctx)
	if err != nil {
		return policy, err
	}
	if baseline != nil {
		policy = mergeBaselinePolicy(policy, baseline.Spec.BaselinePolicy)
	}

	policy = mergeBaselinePolicy(policy, tn.Spec.BaselinePolicy)
	policy = normalizeBaselinePolicy(policy)
	return policy, nil
}

func (r *ProjectReconciler) getBaseline(ctx context.Context) (*platformv1alpha1.Baseline, error) {
	var list platformv1alpha1.BaselineList
	if err := r.List(ctx, &list); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	for i := range list.Items {
		if list.Items[i].Name == defaultBaselineName {
			return &list.Items[i], nil
		}
	}
	sort.SliceStable(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})
	return &list.Items[0], nil
}

func mergeBaselinePolicy(base platformv1alpha1.BaselinePolicy, override platformv1alpha1.BaselinePolicy) platformv1alpha1.BaselinePolicy {
	if override.NetworkIsolation != nil {
		base.NetworkIsolation = override.NetworkIsolation
	}
	if override.ResourceQuota != nil {
		base.ResourceQuota = override.ResourceQuota
	}
	if override.LimitRange != nil {
		base.LimitRange = override.LimitRange
	}
	if override.ResourceQuotaSpec != nil {
		base.ResourceQuotaSpec = override.ResourceQuotaSpec
	}
	if override.LimitRangeSpec != nil {
		base.LimitRangeSpec = override.LimitRangeSpec
	}
	if len(override.AllowedIngressPorts) > 0 {
		base.AllowedIngressPorts = append([]int32{}, override.AllowedIngressPorts...)
	}
	return base
}

func normalizeBaselinePolicy(policy platformv1alpha1.BaselinePolicy) platformv1alpha1.BaselinePolicy {
	if policy.NetworkIsolation == nil {
		policy.NetworkIsolation = boolPtr(true)
	}
	if policy.ResourceQuota == nil {
		policy.ResourceQuota = boolPtr(true)
	}
	if policy.LimitRange == nil {
		policy.LimitRange = boolPtr(true)
	}

	if policy.NetworkIsolation != nil && *policy.NetworkIsolation && len(policy.AllowedIngressPorts) == 0 {
		policy.AllowedIngressPorts = []int32{443}
	}
	if policy.ResourceQuota != nil && *policy.ResourceQuota && policy.ResourceQuotaSpec == nil {
		spec := defaultResourceQuotaSpec()
		policy.ResourceQuotaSpec = &spec
	}
	if policy.LimitRange != nil && *policy.LimitRange && policy.LimitRangeSpec == nil {
		spec := defaultLimitRangeSpec()
		policy.LimitRangeSpec = &spec
	}
	return policy
}

func defaultBaselinePolicy() platformv1alpha1.BaselinePolicy {
	return normalizeBaselinePolicy(platformv1alpha1.BaselinePolicy{})
}

func collectTargetNamespaces(tn *platformv1alpha1.Project) []string {
	var targets []string
	if len(tn.Spec.TargetNamespaces) > 0 {
		targets = append(targets, tn.Spec.TargetNamespaces...)
	} else if tn.Spec.TargetNamespace != "" {
		targets = append(targets, tn.Spec.TargetNamespace)
	} else if tn.Spec.TenantID != "" {
		targets = append(targets, fmt.Sprintf("tenant-%s", tn.Spec.TenantID))
	}

	seen := map[string]struct{}{}
	unique := make([]string, 0, len(targets))
	for _, ns := range targets {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			continue
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		unique = append(unique, ns)
	}
	return unique
}

func boolPtr(v bool) *bool {
	return &v
}
