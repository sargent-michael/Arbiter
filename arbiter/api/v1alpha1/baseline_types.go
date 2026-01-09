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

// BaselineSpec defines default baseline policy settings for tenants.
type BaselineSpec struct {
	// baselinePolicy provides default settings for all tenants.
	// +optional
	BaselinePolicy BaselinePolicy `json:"baselinePolicy,omitempty"`
}

// BaselineStatus defines the observed state of Baseline.
type BaselineStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// defaultsSummary is a human-friendly summary of baseline defaults.
	// +optional
	DefaultsSummary string `json:"defaultsSummary,omitempty"`
}

// +kubebuilder:resource:scope=Cluster,shortName=bl
// +kubebuilder:printcolumn:name="Defaults",type=string,JSONPath=".status.defaultsSummary"
// +kubebuilder:printcolumn:name="Ingress",type=string,JSONPath=".spec.baselinePolicy.allowedIngressPorts",priority=1
// +kubebuilder:printcolumn:name="ReqCPU",type=string,JSONPath=".spec.baselinePolicy.resourceQuotaSpec.hard[\"requests.cpu\"]",priority=1
// +kubebuilder:printcolumn:name="ReqMem",type=string,JSONPath=".spec.baselinePolicy.resourceQuotaSpec.hard[\"requests.memory\"]",priority=1
// +kubebuilder:printcolumn:name="LimCPU",type=string,JSONPath=".spec.baselinePolicy.resourceQuotaSpec.hard[\"limits.cpu\"]",priority=1
// +kubebuilder:printcolumn:name="LimMem",type=string,JSONPath=".spec.baselinePolicy.resourceQuotaSpec.hard[\"limits.memory\"]",priority=1
// +kubebuilder:printcolumn:name="Pods",type=string,JSONPath=".spec.baselinePolicy.resourceQuotaSpec.hard.pods",priority=1
// +kubebuilder:printcolumn:name="PVCs",type=string,JSONPath=".spec.baselinePolicy.storageQuotaSpec.hard.persistentvolumeclaims",priority=1
// +kubebuilder:printcolumn:name="Storage",type=string,JSONPath=".spec.baselinePolicy.storageQuotaSpec.hard[\"requests.storage\"]",priority=1
// +kubebuilder:printcolumn:name="LRReqCPU",type=string,JSONPath=".spec.baselinePolicy.limitRangeSpec.limits[0].defaultRequest.cpu",priority=1
// +kubebuilder:printcolumn:name="LRReqMem",type=string,JSONPath=".spec.baselinePolicy.limitRangeSpec.limits[0].defaultRequest.memory",priority=1
// +kubebuilder:printcolumn:name="LRLimCPU",type=string,JSONPath=".spec.baselinePolicy.limitRangeSpec.limits[0].default.cpu",priority=1
// +kubebuilder:printcolumn:name="LRLimMem",type=string,JSONPath=".spec.baselinePolicy.limitRangeSpec.limits[0].default.memory",priority=1
// +kubebuilder:printcolumn:name="Net",type=boolean,JSONPath=".spec.baselinePolicy.networkIsolation",priority=1
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Baseline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BaselineSpec   `json:"spec,omitempty"`
	Status BaselineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BaselineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Baseline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Baseline{}, &BaselineList{})
}
