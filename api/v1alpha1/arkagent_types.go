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

// MCPServerSpec defines one MCP tool server.
type MCPServerSpec struct {
	// Name is a human-readable identifier for this MCP server.
	Name string `json:"name"`
	// URL is the SSE endpoint of the MCP server.
	URL string `json:"url"`
}

// WebhookToolSpec defines an inline HTTP tool available to agent pods without a full MCP server.
// The agent runtime calls the URL directly when the LLM invokes the tool.
type WebhookToolSpec struct {
	// Name is the tool identifier exposed to the LLM. Must be unique within the deployment.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Description explains the tool's purpose to the LLM.
	Description string `json:"description,omitempty"`
	// URL is the HTTP endpoint the agent calls when the LLM invokes this tool.
	// +kubebuilder:validation:Required
	URL string `json:"url"`
	// Method is the HTTP method used when calling the endpoint.
	// +kubebuilder:default=POST
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH
	Method string `json:"method,omitempty"`
	// InputSchema is a JSON Schema object (as a raw JSON string) describing the tool's
	// input parameters. The LLM uses this to construct valid inputs.
	InputSchema string `json:"inputSchema,omitempty"`
}

// SystemPromptSource selects a system prompt from a ConfigMap or Secret key.
// Exactly one of ConfigMapKeyRef and SecretKeyRef must be non-nil.
type SystemPromptSource struct {
	// ConfigMapKeyRef selects a key of a ConfigMap in the same namespace.
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// SecretKeyRef selects a key of a Secret in the same namespace.
	// Use this when the prompt contains sensitive instructions.
	// +optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// ArkonisLimits constrains per-agent resource usage.
type ArkonisLimits struct {
	// MaxTokensPerCall is the maximum number of tokens per Anthropic API call.
	// +kubebuilder:default=8000
	MaxTokensPerCall int `json:"maxTokensPerCall,omitempty"`
	// MaxConcurrentTasks is the maximum number of tasks processed in parallel per replica.
	// +kubebuilder:default=5
	MaxConcurrentTasks int `json:"maxConcurrentTasks,omitempty"`
	// TimeoutSeconds is the per-task deadline.
	// +kubebuilder:default=120
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// MaxDailyTokens is the rolling 24-hour token budget (input + output combined) for this
	// deployment. When the budget is reached, the operator scales replicas to 0 and sets a
	// BudgetExceeded condition. The deployment resumes automatically once the 24-hour window
	// rotates and usage drops below the limit. Zero means no daily limit.
	// +kubebuilder:validation:Minimum=1
	MaxDailyTokens int64 `json:"maxDailyTokens,omitempty"`
}

// ProbeType defines the kind of liveness check.
// +kubebuilder:validation:Enum=semantic;ping
type ProbeType string

const (
	ProbeTypeSemantic ProbeType = "semantic"
	ProbeTypePing     ProbeType = "ping"
)

// ArkonisProbe defines how to evaluate agent health.
type ArkonisProbe struct {
	// Type is the probe strategy: "semantic" (LLM-based) or "ping" (HTTP).
	Type ProbeType `json:"type"`
	// IntervalSeconds is how often the probe runs.
	// +kubebuilder:default=30
	IntervalSeconds int `json:"intervalSeconds,omitempty"`
	// ValidatorPrompt is the prompt sent when Type is "semantic".
	ValidatorPrompt string `json:"validatorPrompt,omitempty"`
}

// ArkAgentSpec defines the desired state of ArkAgent.
// +kubebuilder:validation:XValidation:rule="has(self.systemPrompt) || has(self.systemPromptRef)",message="at least one of systemPrompt or systemPromptRef must be set"
type ArkAgentSpec struct {
	// Replicas is the number of agent instances to run.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=50
	Replicas *int32 `json:"replicas,omitempty"`

	// Model is the LLM model ID (e.g. "claude-sonnet-4-20250514").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// SystemPrompt is the inline system message injected into every agent call.
	// For long or frequently-iterated prompts, prefer systemPromptRef instead.
	// +optional
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// SystemPromptRef references a ConfigMap or Secret key whose content is used
	// as the system prompt. Takes precedence over systemPrompt when both are set.
	// Updating the referenced ConfigMap or Secret triggers an automatic rolling
	// restart of agent pods.
	// +optional
	SystemPromptRef *SystemPromptSource `json:"systemPromptRef,omitempty"`

	// MCPServers lists the MCP tool servers available to this agent.
	MCPServers []MCPServerSpec `json:"mcpServers,omitempty"`

	// Limits constrains agent resource usage.
	Limits *ArkonisLimits `json:"limits,omitempty"`

	// LivenessProbe defines how agent health is evaluated.
	LivenessProbe *ArkonisProbe `json:"livenessProbe,omitempty"`

	// ConfigRef optionally references an ArkSettings for shared settings.
	ConfigRef *LocalObjectReference `json:"configRef,omitempty"`

	// MemoryRef optionally references an ArkMemory that defines the persistent
	// memory backend for agent instances. When set, the operator injects memory
	// connection details as environment variables into agent pods.
	MemoryRef *LocalObjectReference `json:"memoryRef,omitempty"`

	// Tools lists inline HTTP webhook tools available to agent pods without an external MCP server.
	// The operator serialises these as JSON into the AGENT_WEBHOOK_TOOLS environment variable.
	Tools []WebhookToolSpec `json:"tools,omitempty"`
}

// LocalObjectReference identifies an object in the same namespace.
type LocalObjectReference struct {
	// Name of the referenced object.
	Name string `json:"name"`
}

// ArkAgentStatus defines the observed state of ArkAgent.
type ArkAgentStatus struct {
	// ReadyReplicas is the number of agent pods ready to accept tasks.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Replicas is the total number of agent pods (ready or not).
	Replicas int32 `json:"replicas,omitempty"`
	// ObservedGeneration is the .metadata.generation this status reflects.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// DailyTokenUsage is the sum of tokens consumed by this deployment in the rolling 24-hour
	// window. Populated only when spec.limits.maxDailyTokens is set.
	DailyTokenUsage *TokenUsage `json:"dailyTokenUsage,omitempty"`
	// Conditions reflect the current state of the ArkAgent.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Tokens(24h)",type=integer,JSONPath=`.status.dailyTokenUsage.totalTokens`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced

// ArkAgent manages a pool of LLM agent instances.
type ArkAgent struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec ArkAgentSpec `json:"spec"`

	// +optional
	Status ArkAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArkAgentList contains a list of ArkAgent.
type ArkAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArkAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArkAgent{}, &ArkAgentList{})
}
