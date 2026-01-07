/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OccupantSpec defines the desired state of Occupant
type OccupantSpec struct {
	// occupantID is the stable identifier for the occupant (used for labels, naming, etc.)
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	OccupantID string `json:"occupantID"`

	// targetNamespace is the namespace name to create/manage. If empty, defaults to "occupant-<occupantID>".
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// targetNamespaces is a list of namespaces to create/manage for this tenant.
	// If empty, defaults to ["occupant-<occupantID>"].
	// +optional
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// adoptedNamespaces are existing namespaces Arbiter should adopt without owning deletion lifecycle.
	// +optional
	AdoptedNamespaces []string `json:"adoptedNamespaces,omitempty"`

	// adminSubjects grants admin access inside the tenant namespace.
	// These are RBAC subjects (users, groups, serviceaccounts).
	// +optional
	AdminSubjects []RbacSubject `json:"adminSubjects,omitempty"`

	// enforcementMode controls whether Arbiter enforces baseline policies on managed namespaces.
	// +kubebuilder:validation:Enum=Enforcing;Permissive
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`

	// capabilities lists declared capabilities to expose in status summaries.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// externalIntegrations describe optional integrations for this occupant.
	// +optional
	ExternalIntegrations ExternalIntegrations `json:"externalIntegrations,omitempty"`

	// baselinePolicy enables default quota/limits/network policy.
	// +optional
	BaselinePolicy BaselinePolicy `json:"baselinePolicy,omitempty"`
}

type RbacSubject struct {
	// kind: User | Group | ServiceAccount
	// +kubebuilder:validation:Enum=User;Group;ServiceAccount
	Kind string `json:"kind"`

	// name of the subject
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace only for ServiceAccount subjects
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type BaselinePolicy struct {
	// enable default deny ingress/egress with DNS egress and HTTPS ingress allowed
	// +optional
	NetworkIsolation *bool `json:"networkIsolation,omitempty"`

	// apply default ResourceQuota
	// +optional
	ResourceQuota *bool `json:"resourceQuota,omitempty"`

	// apply default LimitRange
	// +optional
	LimitRange *bool `json:"limitRange,omitempty"`

	// resourceQuotaSpec overrides the default ResourceQuota spec when set.
	// +optional
	ResourceQuotaSpec *corev1.ResourceQuotaSpec `json:"resourceQuotaSpec,omitempty"`

	// limitRangeSpec overrides the default LimitRange spec when set.
	// +optional
	LimitRangeSpec *corev1.LimitRangeSpec `json:"limitRangeSpec,omitempty"`

	// allowedIngressPorts overrides the default allowed ingress ports (TCP).
	// If empty, defaults to [443].
	// +optional
	AllowedIngressPorts []int32 `json:"allowedIngressPorts,omitempty"`
}

type ExternalIntegrations struct {
	// ci integration provider or label.
	// +optional
	CI string `json:"ci,omitempty"`

	// observability integration provider or label.
	// +optional
	Observability string `json:"observability,omitempty"`

	// cost integration provider or label.
	// +optional
	Cost string `json:"cost,omitempty"`
}

// OccupantStatus defines the observed state of Occupant.
type OccupantStatus struct {
	// namespaces managed for this occupant.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// namespacesSummary is a summary of managed/adopted namespace counts.
	// +optional
	NamespacesSummary string `json:"namespacesSummary,omitempty"`

	// enabledCapabilities is the computed list of enabled capabilities.
	// +optional
	EnabledCapabilities []string `json:"enabledCapabilities,omitempty"`

	// capabilitiesSummary is a human-friendly summary of capabilities.
	// +optional
	CapabilitiesSummary string `json:"capabilitiesSummary,omitempty"`

	// health is a coarse health summary (OK/WARN/ERROR).
	// +optional
	Health string `json:"health,omitempty"`

	// driftCount is the number of drift corrections detected in the last reconcile.
	// +optional
	DriftCount int32 `json:"driftCount,omitempty"`

	// driftSummary is a human-friendly drift summary.
	// +optional
	DriftSummary string `json:"driftSummary,omitempty"`

	// identityBindings is a human-friendly summary of admin subjects.
	// +optional
	IdentityBindings string `json:"identityBindings,omitempty"`

	// enforcementStatus reflects the current enforcement mode.
	// +optional
	EnforcementStatus string `json:"enforcementStatus,omitempty"`

	// externalIntegrations summarizes external integrations.
	// +optional
	ExternalIntegrations string `json:"externalIntegrations,omitempty"`

	// lastReconciliationOutcome is the last reconcile outcome.
	// +optional
	LastReconciliationOutcome string `json:"lastReconciliationOutcome,omitempty"`

	// lastReconciliationTime is the time of the last reconcile.
	// +optional
	LastReconciliationTime *metav1.Time `json:"lastReconciliationTime,omitempty"`

	// resourceQuotaSummary is a human-friendly summary of enforced quota.
	// +optional
	ResourceQuotaSummary string `json:"resourceQuotaSummary,omitempty"`

	// limitRangeSummary is a human-friendly summary of enforced limits.
	// +optional
	LimitRangeSummary string `json:"limitRangeSummary,omitempty"`

	// networkPolicySummary is a human-friendly summary of enforced network policy.
	// +optional
	NetworkPolicySummary string `json:"networkPolicySummary,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:scope=Cluster,shortName=occ;project
// +kubebuilder:printcolumn:name="Namespaces",type=string,JSONPath=`.status.namespacesSummary`
// +kubebuilder:printcolumn:name="Enforcement",type=string,JSONPath=`.spec.enforcementMode`
// +kubebuilder:printcolumn:name="Capabilities",type=string,JSONPath=`.status.capabilitiesSummary`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health`
// +kubebuilder:printcolumn:name="Drift",type=string,JSONPath=`.status.driftSummary`
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Occupant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OccupantSpec   `json:"spec"`
	Status OccupantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type OccupantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Occupant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Occupant{}, &OccupantList{})
}
