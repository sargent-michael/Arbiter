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
}

// +kubebuilder:resource:scope=Cluster,shortName=bl
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
