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

// RoutingStrategy defines how tasks are distributed across agent instances.
// +kubebuilder:validation:Enum=round-robin;least-busy;random
type RoutingStrategy string

const (
	RoutingStrategyRoundRobin RoutingStrategy = "round-robin"
	RoutingStrategyLeastBusy  RoutingStrategy = "least-busy"
	RoutingStrategyRandom     RoutingStrategy = "random"
)

// AgentProtocol defines the protocol for an ArkService port.
// +kubebuilder:validation:Enum=A2A;HTTP
type AgentProtocol string

const (
	AgentProtocolA2A  AgentProtocol = "A2A"
	AgentProtocolHTTP AgentProtocol = "HTTP"
)

// ArkServicePort defines a port exposed by the ArkService.
type ArkServicePort struct {
	// Protocol is the communication protocol: "A2A" (agent-to-agent) or "HTTP" (external).
	Protocol AgentProtocol `json:"protocol"`
	// Port is the network port number.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// ArkServiceRouting defines the task routing configuration.
type ArkServiceRouting struct {
	// Strategy controls how tasks are distributed across agent replicas.
	// +kubebuilder:default=round-robin
	Strategy RoutingStrategy `json:"strategy,omitempty"`
}

// ArkServiceSpec defines the desired state of ArkService.
type ArkServiceSpec struct {
	// Selector identifies the ArkAgent this service routes tasks to.
	// +kubebuilder:validation:Required
	Selector ArkServiceSelector `json:"selector"`

	// Routing configures how incoming tasks are distributed.
	Routing ArkServiceRouting `json:"routing,omitempty"`

	// Ports lists the network ports exposed by this service.
	Ports []ArkServicePort `json:"ports,omitempty"`
}

// ArkServiceSelector identifies the target ArkAgent.
type ArkServiceSelector struct {
	// ArkAgent is the name of the ArkAgent to route tasks to.
	// +kubebuilder:validation:Required
	ArkAgent string `json:"arkAgent"`
}

// ArkServiceStatus defines the observed state of ArkService.
type ArkServiceStatus struct {
	// ReadyReplicas is the number of ready agent replicas currently backing this service.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// ObservedGeneration is the .metadata.generation this status reflects.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions reflect the current state of the ArkService.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.selector.arkAgent`
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.routing.strategy`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=aosvc,scope=Namespaced

// ArkService routes incoming tasks to a pool of ArkAgent replicas.
type ArkService struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec ArkServiceSpec `json:"spec"`

	// +optional
	Status ArkServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArkServiceList contains a list of ArkService.
type ArkServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArkService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArkService{}, &ArkServiceList{})
}
