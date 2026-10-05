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
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceTier defines the preset resource quotas for a Sandbox.
// +kubebuilder:validation:Enum=Small;Medium;Large
type ResourceTier string

const (
	ResourceTierSmall  ResourceTier = "Small"
	ResourceTierMedium ResourceTier = "Medium"
	ResourceTierLarge  ResourceTier = "Large"
)

// SandboxPhase represents the high-level lifecycle state of a Sandbox.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Terminating;Expired
type SandboxPhase string

const (
	SandboxPhasePending      SandboxPhase = "Pending"
	SandboxPhaseProvisioning SandboxPhase = "Provisioning"
	SandboxPhaseReady        SandboxPhase = "Ready"
	SandboxPhaseTerminating  SandboxPhase = "Terminating"
	SandboxPhaseExpired      SandboxPhase = "Expired"
)

// Condition types for Sandbox status.
const (
	ConditionTypeNamespaceReady = "NamespaceReady"
	ConditionTypeResourcesReady = "ResourcesReady"
	ConditionTypeNetworkReady   = "NetworkReady"
	ConditionTypeReady          = "Ready"
	ConditionTypeExpired        = "Expired"
)

// SandboxSpec defines the desired state of Sandbox.
type SandboxSpec struct {
	// ttlDuration defines the lifetime of the Sandbox before expiration (e.g., "4h", "24h", "30m").
	// +required
	TtlDuration metav1.Duration `json:"ttlDuration"`

	// resourceTier defines the quota allocation size for the Sandbox.
	// +kubebuilder:default=Small
	// +optional
	ResourceTier ResourceTier `json:"resourceTier,omitempty"`

	// networkIsolation controls whether default-deny ingress and egress policies are enforced.
	// When true, only DNS and intra-namespace traffic are allowed.
	// +kubebuilder:default=true
	// +optional
	NetworkIsolation bool `json:"networkIsolation,omitempty"`
}

// SandboxStatus defines the observed state of Sandbox.
type SandboxStatus struct {
	// conditions represent the granular observations of the Sandbox state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase indicates the high-level lifecycle stage of the Sandbox.
	// +optional
	Phase SandboxPhase `json:"phase,omitempty"`

	// allocatedNamespace is the name of the dedicated namespace provisioned for this Sandbox.
	// +optional
	AllocatedNamespace string `json:"allocatedNamespace,omitempty"`

	// expiresAt is the timestamp at which this Sandbox expires and becomes eligible for teardown.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current lifecycle phase"
// +kubebuilder:printcolumn:name="Namespace",type="string",JSONPath=".status.allocatedNamespace",description="Allocated sandbox namespace"
// +kubebuilder:printcolumn:name="Tier",type="string",JSONPath=".spec.resourceTier",description="Resource quota tier"
// +kubebuilder:printcolumn:name="Expires At",type="date",JSONPath=".status.expiresAt",description="Expiration timestamp"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Sandbox is the Schema for the sandboxes API
type Sandbox struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Sandbox
	// +required
	Spec SandboxSpec `json:"spec"`

	// status defines the observed state of Sandbox
	// +optional
	Status SandboxStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SandboxList contains a list of Sandbox
type SandboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Sandbox `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Sandbox{}, &SandboxList{})
		return nil
	})
}
