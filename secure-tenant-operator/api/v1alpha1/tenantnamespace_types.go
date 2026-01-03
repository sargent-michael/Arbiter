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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TenantNamespaceSpec defines the desired state of TenantNamespace
type TenantNamespaceSpec struct {
	// tenantID is the stable identifier for the tenant (used for labels, naming, etc.)
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	TenantID string `json:"tenantID"`

	// targetNamespace is the namespace name to create/manage. If empty, defaults to "tenant-<tenantID>".
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// adminSubjects grants admin access inside the tenant namespace.
	// These are RBAC subjects (users, groups, serviceaccounts).
	// +optional
	AdminSubjects []RbacSubject `json:"adminSubjects,omitempty"`

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
	// enable default deny ingress/egress with DNS allowed and intra-namespace allowed
	// +optional
	NetworkIsolation *bool `json:"networkIsolation,omitempty"`

	// apply default ResourceQuota
	// +optional
	ResourceQuota *bool `json:"resourceQuota,omitempty"`

	// apply default LimitRange
	// +optional
	LimitRange *bool `json:"limitRange,omitempty"`
}

// TenantNamespaceStatus defines the observed state of TenantNamespace.
type TenantNamespaceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:scope=Cluster,shortName=tns
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type TenantNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantNamespaceSpec   `json:"spec"`
	Status TenantNamespaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// +kubebuilder:object:root=true
type TenantNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TenantNamespace{}, &TenantNamespaceList{})
}
