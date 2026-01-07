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

type OccupantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const occupantFinalizer = "project-arbiter.io/occupant-cleanup"
const occupantLabelKey = "project-arbiter.io/occupant"
const namespaceModeLabelKey = "project-arbiter.io/namespace-mode"
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
	occupantReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "arbiter_occupant_reconcile_total",
			Help: "Total number of occupant reconciliations.",
		},
		[]string{"occupant", "result"},
	)
	occupantReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "arbiter_occupant_reconcile_duration_seconds",
			Help:    "Occupant reconciliation duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"occupant", "result"},
	)
	occupantNamespaces = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "arbiter_occupant_namespaces",
			Help: "Count of namespaces by occupant and mode.",
		},
		[]string{"occupant", "mode"},
	)
	occupantDriftCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "arbiter_occupant_drift_count",
			Help: "Drift corrections detected for the occupant.",
		},
		[]string{"occupant"},
	)
	occupantInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "arbiter_occupant_info",
			Help: "Occupant metadata for dashboards (value is always 1).",
		},
		[]string{"occupant", "enforcement", "capabilities", "health", "outcome"},
	)
	occupantEnforcementMode = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "arbiter_occupant_enforcement_mode",
			Help: "Enforcement mode for the occupant (1 for active mode).",
		},
		[]string{"occupant", "mode"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileErrors,
		reconcileDuration,
		occupantReconcileTotal,
		occupantReconcileDuration,
		occupantNamespaces,
		occupantDriftCount,
		occupantInfo,
		occupantEnforcementMode,
	)
}

// +kubebuilder:rbac:groups=project-arbiter.io,resources=occupants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=project-arbiter.io,resources=occupants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=project-arbiter.io,resources=occupants/finalizers,verbs=update

// Core resources we manage
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// RBAC resources we manage
// We only create RoleBindings (NOT Roles)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch

// Needed to reference ClusterRole "admin" in RoleBindings (RBAC bind check)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,resourceNames=admin,verbs=bind

func (r *OccupantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx)
	start := time.Now()
	occupantName := req.Name
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "error"
			reconcileErrors.WithLabelValues("occupant").Inc()
		}
		reconcileTotal.WithLabelValues("occupant", outcome).Inc()
		reconcileDuration.WithLabelValues("occupant", outcome).Observe(time.Since(start).Seconds())
		if occupantName != "" {
			occupantReconcileTotal.WithLabelValues(occupantName, outcome).Inc()
			occupantReconcileDuration.WithLabelValues(occupantName, outcome).Observe(time.Since(start).Seconds())
		}
	}()

	var occ platformv1alpha1.Occupant
	if err := r.Get(ctx, req.NamespacedName, &occ); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	occupantName = occ.Name

	if !occ.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&occ, occupantFinalizer) {
			done, err := r.finalizeOccupant(ctx, &occ)
			if err != nil {
				return ctrl.Result{}, err
			}
			if !done {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			controllerutil.RemoveFinalizer(&occ, occupantFinalizer)
			if err := r.Update(ctx, &occ); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	occupantID := occ.Spec.OccupantID
	if occupantID == "" {
		log.Info("spec.occupantID is empty; waiting for a valid spec")

		apimeta.SetStatusCondition(&occ.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "MissingOccupantID",
			Message:            "spec.occupantID must be set",
			LastTransitionTime: metav1.Now(),
		})
		occ.Status.Health = "WARN"
		occ.Status.LastReconciliationOutcome = "blocked"
		now := metav1.Now()
		occ.Status.LastReconciliationTime = &now
		_ = r.Status().Update(ctx, &occ) // best-effort
		return ctrl.Result{}, nil
	}

	managedNamespaces := collectManagedNamespaces(&occ)
	adoptedNamespaces := excludeNamespaces(collectAdoptedNamespaces(&occ), managedNamespaces)
	if len(managedNamespaces) == 0 && len(adoptedNamespaces) == 0 {
		log.Info("no target namespaces resolved; waiting for a valid spec")
		apimeta.SetStatusCondition(&occ.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "MissingNamespaces",
			Message:            "spec.targetNamespaces, spec.targetNamespace, or spec.adoptedNamespaces must be set",
			LastTransitionTime: metav1.Now(),
		})
		occ.Status.Health = "WARN"
		occ.Status.LastReconciliationOutcome = "blocked"
		now := metav1.Now()
		occ.Status.LastReconciliationTime = &now
		_ = r.Status().Update(ctx, &occ) // best-effort
		return ctrl.Result{}, nil
	}

	policy, err := r.resolveBaselinePolicy(ctx, &occ)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(&occ, occupantFinalizer) {
		controllerutil.AddFinalizer(&occ, occupantFinalizer)
		if err := r.Update(ctx, &occ); err != nil {
			return ctrl.Result{}, err
		}
	}

	enforcementMode := normalizeEnforcementMode(occ.Spec.EnforcementMode)
	adjustedPolicy := policy
	if enforcementMode == "Permissive" {
		adjustedPolicy = disableBaselinePolicy(policy)
	}

	driftCount := int32(0)

	for _, targetNS := range managedNamespaces {
		namespaceDrift, err := r.reconcileNamespace(ctx, &occ, targetNS, occupantID, enforcementMode, false, adjustedPolicy)
		if err != nil {
			return ctrl.Result{}, err
		}
		driftCount += namespaceDrift
	}

	for _, adoptedNS := range adoptedNamespaces {
		namespaceDrift, err := r.reconcileNamespace(ctx, &occ, adoptedNS, occupantID, enforcementMode, true, adjustedPolicy)
		if err != nil {
			return ctrl.Result{}, err
		}
		driftCount += namespaceDrift
	}

	original := occ.DeepCopy()

	apimeta.SetStatusCondition(&occ.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Occupant namespaces and baseline controls are in place",
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: occ.Generation,
	})

	allNamespaces := append([]string{}, managedNamespaces...)
	allNamespaces = append(allNamespaces, adoptedNamespaces...)
	occ.Status.Namespaces = append([]string{}, allNamespaces...)
	sort.Strings(occ.Status.Namespaces)
	occ.Status.NamespacesSummary = summarizeNamespaces(managedNamespaces, adoptedNamespaces)
	occ.Status.ResourceQuotaSummary = summarizeResourceQuota(adjustedPolicy)
	occ.Status.LimitRangeSummary = summarizeLimitRange(adjustedPolicy)
	occ.Status.NetworkPolicySummary = summarizeNetworkPolicy(adjustedPolicy)
	occ.Status.EnabledCapabilities = resolveCapabilities(&occ, adjustedPolicy)
	occ.Status.CapabilitiesSummary = summarizeCapabilities(occ.Status.EnabledCapabilities)
	occ.Status.IdentityBindings = summarizeIdentityBindings(occ.Spec.AdminSubjects)
	occ.Status.EnforcementStatus = enforcementMode
	occ.Status.ExternalIntegrations = summarizeExternalIntegrations(occ.Spec.ExternalIntegrations)
	occ.Status.DriftCount = driftCount
	occ.Status.DriftSummary = fmt.Sprintf("%d", driftCount)
	if driftCount > 0 {
		occ.Status.Health = "WARN"
	} else {
		occ.Status.Health = "OK"
	}
	occ.Status.LastReconciliationOutcome = "success"
	now := metav1.Now()
	occ.Status.LastReconciliationTime = &now

	if err := r.Status().Patch(ctx, &occ, client.MergeFrom(original)); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	updateOccupantMetrics(&occ, enforcementMode, driftCount, managedNamespaces, adoptedNamespaces)
	return ctrl.Result{}, nil
}

func updateOccupantMetrics(
	occ *platformv1alpha1.Occupant,
	enforcementMode string,
	driftCount int32,
	managedNamespaces []string,
	adoptedNamespaces []string,
) {
	occupantName := occ.Name
	occupantNamespaces.WithLabelValues(occupantName, "managed").Set(float64(len(managedNamespaces)))
	occupantNamespaces.WithLabelValues(occupantName, "adopted").Set(float64(len(adoptedNamespaces)))
	occupantDriftCount.WithLabelValues(occupantName).Set(float64(driftCount))

	occupantEnforcementMode.WithLabelValues(occupantName, "enforcing").Set(boolToGauge(enforcementMode == "Enforcing"))
	occupantEnforcementMode.WithLabelValues(occupantName, "permissive").Set(boolToGauge(enforcementMode == "Permissive"))

	health := occ.Status.Health
	if health == "" {
		health = "unknown"
	}
	outcome := occ.Status.LastReconciliationOutcome
	if outcome == "" {
		outcome = "unknown"
	}
	capabilities := occ.Status.CapabilitiesSummary
	if capabilities == "" {
		capabilities = "none"
	}

	occupantInfo.WithLabelValues(occupantName, enforcementMode, capabilities, health, outcome).Set(1)
}

func boolToGauge(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func (r *OccupantReconciler) finalizeOccupant(ctx context.Context, occ *platformv1alpha1.Occupant) (bool, error) {
	occupantID := occ.Spec.OccupantID
	targetNamespaces := collectManagedNamespaces(occ)
	if len(targetNamespaces) == 0 && occupantID != "" {
		var nsList corev1.NamespaceList
		if err := r.List(ctx, &nsList, client.MatchingLabels{occupantLabelKey: occupantID}); err != nil {
			return false, err
		}
		for _, ns := range nsList.Items {
			if ns.Labels[namespaceModeLabelKey] == "managed" {
				targetNamespaces = append(targetNamespaces, ns.Name)
			}
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

func (r *OccupantReconciler) reconcileNamespace(
	ctx context.Context,
	occ *platformv1alpha1.Occupant,
	nsName, occupantID, enforcementMode string,
	adopted bool,
	policy platformv1alpha1.BaselinePolicy,
) (int32, error) {
	drift := int32(0)

	changed, err := r.ensureNamespace(ctx, occ, nsName, occupantID, enforcementMode, adopted)
	if err != nil {
		return drift, err
	}
	if changed {
		drift++
	}

	changed, err = r.ensureAdminRBAC(ctx, occ, nsName)
	if err != nil {
		return drift, err
	}
	if changed {
		drift++
	}

	if policy.ResourceQuota != nil && *policy.ResourceQuota {
		changed, err = r.ensureResourceQuota(ctx, occ, nsName, policy)
		if err != nil {
			return drift, err
		}
		if changed {
			drift++
		}
	}
	if policy.LimitRange != nil && *policy.LimitRange {
		changed, err = r.ensureLimitRange(ctx, occ, nsName, policy)
		if err != nil {
			return drift, err
		}
		if changed {
			drift++
		}
	}
	if policy.NetworkIsolation != nil && *policy.NetworkIsolation {
		networkDrift, err := r.ensureNetworkPolicies(ctx, occ, nsName, policy)
		if err != nil {
			return drift, err
		}
		drift += int32(networkDrift)
	}
	if enforcementMode == "Permissive" && !adopted {
		cleanupDrift, err := r.cleanupBaselineResources(ctx, nsName)
		if err != nil {
			return drift, err
		}
		drift += cleanupDrift
	}
	return drift, nil
}

func (r *OccupantReconciler) ensureNamespace(ctx context.Context, occ *platformv1alpha1.Occupant, nsName, occupantID, enforcementMode string, adopted bool) (bool, error) {
	var ns corev1.Namespace
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	if apierrors.IsNotFound(err) {
		if adopted {
			return false, fmt.Errorf("adopted namespace %q does not exist", nsName)
		}
		labels := baseNamespaceLabels(occupantID, enforcementMode, adopted)
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nsName,
				Labels: labels,
			},
		}
		if err := ctrl.SetControllerReference(occ, &ns, r.Scheme); err != nil {
			return false, err
		}
		return true, r.Create(ctx, &ns)
	}
	if err != nil {
		return false, err
	}

	desired := baseNamespaceLabels(occupantID, enforcementMode, adopted)
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

	if !adopted && !metav1.IsControlledBy(&ns, occ) {
		if err := ctrl.SetControllerReference(occ, &ns, r.Scheme); err != nil {
			return false, err
		}
		changed = true
	}

	if changed {
		return true, r.Update(ctx, &ns)
	}
	return false, nil
}

func (r *OccupantReconciler) ensureAdminRBAC(ctx context.Context, occ *platformv1alpha1.Occupant, ns string) (bool, error) {
	// FIX: Do not create a wildcard Role (that triggers RBAC escalation protection).
	// Instead, bind tenant admin subjects to the built-in ClusterRole "admin" within this namespace.
	// This yields namespace-admin power without requiring cluster-admin.

	if len(occ.Spec.AdminSubjects) == 0 {
		// Nothing to bind; no-op.
		return false, nil
	}

	rbName := "occupant-admins"
	legacyName := "tenant-admins"

	changed := false
	deletedLegacy, err := r.deleteLegacyRoleBinding(ctx, legacyName, ns)
	if err != nil {
		return false, err
	}
	if deletedLegacy {
		changed = true
	}

	desiredSubjects := make([]rbacv1.Subject, 0, len(occ.Spec.AdminSubjects))
	for _, s := range occ.Spec.AdminSubjects {
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
	err = r.Get(ctx, types.NamespacedName{Name: rbName, Namespace: ns}, &rb)
	if apierrors.IsNotFound(err) {
		rb = rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "arbiter",
					occupantLabelKey:               occ.Spec.OccupantID,
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
		if err := ctrl.SetControllerReference(occ, &rb, r.Scheme); err != nil {
			return false, err
		}
		return true, r.Create(ctx, &rb)
	} else if err != nil {
		return false, err
	}

	// Drift correction (MVP): overwrite subjects + ensure roleRef stays correct.
	desiredRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "admin",
	}
	if rb.Labels == nil {
		rb.Labels = map[string]string{}
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/managed-by": "arbiter",
		occupantLabelKey:               occ.Spec.OccupantID,
	}

	if reflect.DeepEqual(rb.Subjects, desiredSubjects) &&
		reflect.DeepEqual(rb.RoleRef, desiredRoleRef) &&
		labelsContain(rb.Labels, desiredLabels) {
		return changed, nil
	}

	rb.Subjects = desiredSubjects
	rb.RoleRef = desiredRoleRef
	for k, v := range desiredLabels {
		rb.Labels[k] = v
	}

	return true, r.Update(ctx, &rb)
}

func (r *OccupantReconciler) deleteLegacyRoleBinding(ctx context.Context, name, namespace string) (bool, error) {
	var rb rbacv1.RoleBinding
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &rb)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, r.Delete(ctx, &rb)
}

func (r *OccupantReconciler) ensureResourceQuota(ctx context.Context, occ *platformv1alpha1.Occupant, ns string, policy platformv1alpha1.BaselinePolicy) (bool, error) {
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
		if err := ctrl.SetControllerReference(occ, &rq, r.Scheme); err != nil {
			return false, err
		}
		return true, r.Create(ctx, &rq)
	}
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(rq.Spec, desiredSpec) {
		rq.Spec = desiredSpec
		return true, r.Update(ctx, &rq)
	}
	return false, nil
}

func (r *OccupantReconciler) ensureLimitRange(ctx context.Context, occ *platformv1alpha1.Occupant, ns string, policy platformv1alpha1.BaselinePolicy) (bool, error) {
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
		if err := ctrl.SetControllerReference(occ, &lr, r.Scheme); err != nil {
			return false, err
		}
		return true, r.Create(ctx, &lr)
	}
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(lr.Spec, desiredSpec) {
		lr.Spec = desiredSpec
		return true, r.Update(ctx, &lr)
	}
	return false, nil
}

func (r *OccupantReconciler) ensureNetworkPolicies(ctx context.Context, occ *platformv1alpha1.Occupant, ns string, policy platformv1alpha1.BaselinePolicy) (int, error) {
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
	changed, err := r.ensureNetworkPolicy(ctx, occ, deny)
	if err != nil {
		return 0, err
	}
	drift := 0
	if changed {
		drift++
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
	changed, err = r.ensureNetworkPolicy(ctx, occ, allowHTTPSIngress)
	if err != nil {
		return drift, err
	}
	if changed {
		drift++
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
	changed, err = r.ensureNetworkPolicy(ctx, occ, allowDNS)
	if changed {
		drift++
	}
	return drift, err
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

func (r *OccupantReconciler) ensureNetworkPolicy(ctx context.Context, owner client.Object, desired *netv1.NetworkPolicy) (bool, error) {
	var current netv1.NetworkPolicy
	key := types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}
	if err := r.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			if err := ctrl.SetControllerReference(owner, desired, r.Scheme); err != nil {
				return false, err
			}
			return true, r.Create(ctx, desired)
		}
		return false, err
	}

	if reflect.DeepEqual(current.Spec, desired.Spec) {
		return false, nil
	}

	current.Spec = desired.Spec
	return true, r.Update(ctx, &current)
}

func (r *OccupantReconciler) cleanupBaselineResources(ctx context.Context, ns string) (int32, error) {
	var drift int32
	if deleted, err := r.deleteResourceQuota(ctx, "tenant-quota", ns); err != nil {
		return drift, err
	} else if deleted {
		drift++
	}
	if deleted, err := r.deleteLimitRange(ctx, "tenant-limits", ns); err != nil {
		return drift, err
	} else if deleted {
		drift++
	}
	networkNames := []string{"default-deny-all", "allow-https-ingress", "allow-dns-egress"}
	for _, name := range networkNames {
		if deleted, err := r.deleteNetworkPolicy(ctx, name, ns); err != nil {
			return drift, err
		} else if deleted {
			drift++
		}
	}
	return drift, nil
}

func (r *OccupantReconciler) deleteResourceQuota(ctx context.Context, name, ns string) (bool, error) {
	var rq corev1.ResourceQuota
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &rq)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, r.Delete(ctx, &rq)
}

func (r *OccupantReconciler) deleteLimitRange(ctx context.Context, name, ns string) (bool, error) {
	var lr corev1.LimitRange
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &lr)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, r.Delete(ctx, &lr)
}

func (r *OccupantReconciler) deleteNetworkPolicy(ctx context.Context, name, ns string) (bool, error) {
	var np netv1.NetworkPolicy
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &np)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, r.Delete(ctx, &np)
}

func (r *OccupantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Occupant{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&netv1.NetworkPolicy{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&platformv1alpha1.Baseline{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
			var list platformv1alpha1.OccupantList
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
		Named("occupant").
		Complete(r)
}

func (r *OccupantReconciler) resolveBaselinePolicy(ctx context.Context, occ *platformv1alpha1.Occupant) (platformv1alpha1.BaselinePolicy, error) {
	policy := defaultBaselinePolicy()

	baseline, err := r.getBaseline(ctx)
	if err != nil {
		return policy, err
	}
	if baseline != nil {
		policy = mergeBaselinePolicy(policy, baseline.Spec.BaselinePolicy)
	}

	policy = mergeBaselinePolicy(policy, occ.Spec.BaselinePolicy)
	policy = normalizeBaselinePolicy(policy)
	return policy, nil
}

func (r *OccupantReconciler) getBaseline(ctx context.Context) (*platformv1alpha1.Baseline, error) {
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

func collectManagedNamespaces(occ *platformv1alpha1.Occupant) []string {
	var targets []string
	if len(occ.Spec.TargetNamespaces) > 0 {
		targets = append(targets, occ.Spec.TargetNamespaces...)
	} else if occ.Spec.TargetNamespace != "" {
		targets = append(targets, occ.Spec.TargetNamespace)
	} else if occ.Spec.OccupantID != "" && len(occ.Spec.AdoptedNamespaces) == 0 {
		targets = append(targets, fmt.Sprintf("occupant-%s", occ.Spec.OccupantID))
	}
	return uniqueNamespaces(targets)
}

func collectAdoptedNamespaces(occ *platformv1alpha1.Occupant) []string {
	return uniqueNamespaces(occ.Spec.AdoptedNamespaces)
}

func boolPtr(v bool) *bool {
	return &v
}

func uniqueNamespaces(names []string) []string {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(names))
	for _, ns := range names {
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

func excludeNamespaces(source, exclude []string) []string {
	if len(source) == 0 || len(exclude) == 0 {
		return source
	}
	excludeSet := map[string]struct{}{}
	for _, ns := range exclude {
		excludeSet[ns] = struct{}{}
	}
	kept := make([]string, 0, len(source))
	for _, ns := range source {
		if _, ok := excludeSet[ns]; ok {
			continue
		}
		kept = append(kept, ns)
	}
	return kept
}

func normalizeEnforcementMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "permissive":
		return "Permissive"
	default:
		return "Enforcing"
	}
}

func disableBaselinePolicy(policy platformv1alpha1.BaselinePolicy) platformv1alpha1.BaselinePolicy {
	disabled := policy
	disabled.NetworkIsolation = boolPtr(false)
	disabled.ResourceQuota = boolPtr(false)
	disabled.LimitRange = boolPtr(false)
	return disabled
}

func baseNamespaceLabels(occupantID, enforcementMode string, adopted bool) map[string]string {
	enforceLevel := "baseline"
	if enforcementMode == "Permissive" {
		enforceLevel = "privileged"
	}
	modeLabel := "managed"
	if adopted {
		modeLabel = "adopted"
	}
	return map[string]string{
		occupantLabelKey:                     occupantID,
		namespaceModeLabelKey:                modeLabel,
		"pod-security.kubernetes.io/enforce": enforceLevel,
		"pod-security.kubernetes.io/audit":   enforceLevel,
		"pod-security.kubernetes.io/warn":    enforceLevel,
	}
}

func labelsContain(existing, expected map[string]string) bool {
	for k, v := range expected {
		if existing[k] != v {
			return false
		}
	}
	return true
}

func summarizeNamespaces(managed, adopted []string) string {
	managedCount := len(managed)
	adoptedCount := len(adopted)
	parts := []string{}
	if managedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d (managed)", managedCount))
	}
	if adoptedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d (adopted)", adoptedCount))
	}
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, ", ")
}

func resolveCapabilities(occ *platformv1alpha1.Occupant, policy platformv1alpha1.BaselinePolicy) []string {
	capSet := map[string]struct{}{}
	add := func(value string) {
		if value == "" {
			return
		}
		capSet[value] = struct{}{}
	}

	add("rbac")
	for _, capName := range occ.Spec.Capabilities {
		capName = strings.TrimSpace(capName)
		add(capName)
	}
	if policy.NetworkIsolation != nil && *policy.NetworkIsolation {
		add("net")
	}
	if occ.Spec.ExternalIntegrations.Observability != "" {
		add("obs")
	}
	if occ.Spec.ExternalIntegrations.CI != "" {
		add("ci")
	}
	if occ.Spec.ExternalIntegrations.Cost != "" {
		add("cost")
	}

	ordered := []string{}
	preferred := []string{"rbac", "net", "obs", "ci", "cost"}
	for _, capName := range preferred {
		if _, ok := capSet[capName]; ok {
			ordered = append(ordered, capName)
			delete(capSet, capName)
		}
	}
	remaining := make([]string, 0, len(capSet))
	for capName := range capSet {
		remaining = append(remaining, capName)
	}
	sort.Strings(remaining)
	return append(ordered, remaining...)
}

func summarizeCapabilities(caps []string) string {
	if len(caps) == 0 {
		return "none"
	}
	return strings.Join(caps, ",")
}

func summarizeIdentityBindings(subjects []platformv1alpha1.RbacSubject) string {
	if len(subjects) == 0 {
		return "none"
	}
	groups := []string{}
	users := []string{}
	serviceAccounts := []string{}
	for _, subject := range subjects {
		switch subject.Kind {
		case "Group":
			groups = append(groups, subject.Name)
		case "User":
			users = append(users, subject.Name)
		case "ServiceAccount":
			if subject.Namespace != "" {
				serviceAccounts = append(serviceAccounts, fmt.Sprintf("%s/%s", subject.Namespace, subject.Name))
			} else {
				serviceAccounts = append(serviceAccounts, subject.Name)
			}
		}
	}
	sort.Strings(groups)
	sort.Strings(users)
	sort.Strings(serviceAccounts)

	parts := []string{}
	if len(groups) > 0 {
		parts = append(parts, fmt.Sprintf("groups:%s", strings.Join(groups, ",")))
	}
	if len(users) > 0 {
		parts = append(parts, fmt.Sprintf("users:%s", strings.Join(users, ",")))
	}
	if len(serviceAccounts) > 0 {
		parts = append(parts, fmt.Sprintf("sa:%s", strings.Join(serviceAccounts, ",")))
	}
	return strings.Join(parts, " ")
}

func summarizeExternalIntegrations(integrations platformv1alpha1.ExternalIntegrations) string {
	parts := []string{}
	if integrations.CI != "" {
		parts = append(parts, fmt.Sprintf("ci:%s", integrations.CI))
	}
	if integrations.Observability != "" {
		parts = append(parts, fmt.Sprintf("obs:%s", integrations.Observability))
	}
	if integrations.Cost != "" {
		parts = append(parts, fmt.Sprintf("cost:%s", integrations.Cost))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func summarizeResourceQuota(policy platformv1alpha1.BaselinePolicy) string {
	if policy.ResourceQuota == nil || !*policy.ResourceQuota {
		return "disabled"
	}
	if policy.ResourceQuotaSpec == nil {
		return "default"
	}
	spec := policy.ResourceQuotaSpec
	return fmt.Sprintf("cpu:%s/%s mem:%s/%s pods:%s",
		resourceString(spec.Hard, corev1.ResourceRequestsCPU),
		resourceString(spec.Hard, corev1.ResourceLimitsCPU),
		resourceString(spec.Hard, corev1.ResourceRequestsMemory),
		resourceString(spec.Hard, corev1.ResourceLimitsMemory),
		resourceString(spec.Hard, corev1.ResourcePods),
	)
}

func summarizeLimitRange(policy platformv1alpha1.BaselinePolicy) string {
	if policy.LimitRange == nil || !*policy.LimitRange {
		return "disabled"
	}
	if policy.LimitRangeSpec == nil || len(policy.LimitRangeSpec.Limits) == 0 {
		return "default"
	}
	item := policy.LimitRangeSpec.Limits[0]
	defCPU := resourceString(item.Default, corev1.ResourceCPU)
	defMem := resourceString(item.Default, corev1.ResourceMemory)
	reqCPU := resourceString(item.DefaultRequest, corev1.ResourceCPU)
	reqMem := resourceString(item.DefaultRequest, corev1.ResourceMemory)
	return fmt.Sprintf("req:%s/%s lim:%s/%s", reqCPU, reqMem, defCPU, defMem)
}

func summarizeNetworkPolicy(policy platformv1alpha1.BaselinePolicy) string {
	if policy.NetworkIsolation == nil || !*policy.NetworkIsolation {
		return "disabled"
	}
	if len(policy.AllowedIngressPorts) == 0 {
		return "ingress:443"
	}
	ports := make([]string, 0, len(policy.AllowedIngressPorts))
	for _, port := range policy.AllowedIngressPorts {
		ports = append(ports, fmt.Sprintf("%d", port))
	}
	sort.Strings(ports)
	return fmt.Sprintf("ingress:%s", strings.Join(ports, ","))
}

func resourceString(list corev1.ResourceList, name corev1.ResourceName) string {
	if list == nil {
		return "-"
	}
	quantity, ok := list[name]
	if !ok {
		return "-"
	}
	return quantity.String()
}
