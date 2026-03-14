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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TriggerSourceType identifies what fires the trigger.
// +kubebuilder:validation:Enum=cron;webhook;pipeline-output
type TriggerSourceType string

const (
	// TriggerSourceCron fires on a cron schedule.
	TriggerSourceCron TriggerSourceType = "cron"
	// TriggerSourceWebhook fires when an HTTP POST is received at the trigger's webhook URL.
	TriggerSourceWebhook TriggerSourceType = "webhook"
	// TriggerSourcePipelineOutput fires when a named ArkFlow reaches a given phase.
	TriggerSourcePipelineOutput TriggerSourceType = "pipeline-output"
)

// ConcurrencyPolicy controls what happens when a trigger fires while a
// previously dispatched pipeline is still running.
// +kubebuilder:validation:Enum=Allow;Forbid
type ConcurrencyPolicy string

const (
	// ConcurrencyAllow always dispatches a new pipeline run, even if a previous one is still running.
	ConcurrencyAllow ConcurrencyPolicy = "Allow"
	// ConcurrencyForbid skips the fire if any pipeline owned by this trigger is still Running.
	ConcurrencyForbid ConcurrencyPolicy = "Forbid"
)

// ArkEventSource defines what fires the trigger.
type ArkEventSource struct {
	// Type is the source type: cron | webhook | pipeline-output.
	// +kubebuilder:validation:Required
	Type TriggerSourceType `json:"type"`

	// Cron is a standard 5-field cron expression (minute hour dom month dow).
	// Only used when type=cron.
	// Example: "0 9 * * 1-5" (9am on weekdays)
	Cron string `json:"cron,omitempty"`

	// PipelineOutput triggers when the named ArkFlow reaches a phase.
	// Only used when type=pipeline-output.
	PipelineOutput *PipelineOutputSource `json:"pipelineOutput,omitempty"`
}

// PipelineOutputSource references an ArkFlow whose completion fires the trigger.
type PipelineOutputSource struct {
	// Name is the ArkFlow to watch.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// OnPhase is the pipeline phase that fires the trigger. Defaults to Succeeded.
	// +kubebuilder:default=Succeeded
	OnPhase ArkFlowPhase `json:"onPhase,omitempty"`
}

// ArkEventTarget describes a pipeline to dispatch when the trigger fires.
type ArkEventTarget struct {
	// Pipeline is the name of the ArkFlow spec to use as a template for the dispatched run.
	// +kubebuilder:validation:Required
	Pipeline string `json:"pipeline"`

	// Input values to set on the dispatched pipeline, overriding the template pipeline's inputs.
	// Values are Go template strings evaluated with the trigger fire context:
	//   {{ .trigger.name }}    — trigger name
	//   {{ .trigger.firedAt }} — RFC3339 fire timestamp
	//   {{ .trigger.body.* }} — JSON fields from the webhook request body (webhook type only)
	//   {{ .trigger.output }} — upstream pipeline output (pipeline-output type only)
	Input map[string]string `json:"input,omitempty"`
}

// ArkEventSpec defines the desired state of ArkEvent.
type ArkEventSpec struct {
	// Source defines what fires this trigger.
	// +kubebuilder:validation:Required
	Source ArkEventSource `json:"source"`

	// Targets is the list of pipelines to dispatch when the trigger fires.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Targets []ArkEventTarget `json:"targets"`

	// ConcurrencyPolicy controls what happens when the trigger fires while a previous
	// run is still in progress. Defaults to Allow.
	// +kubebuilder:default=Allow
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// Suspended pauses the trigger without deleting it.
	// +kubebuilder:default=false
	Suspended bool `json:"suspended,omitempty"`
}

// ArkEventStatus defines the observed state of ArkEvent.
type ArkEventStatus struct {
	// LastFiredAt is when the trigger last dispatched pipelines.
	LastFiredAt *metav1.Time `json:"lastFiredAt,omitempty"`
	// NextFireAt is the next scheduled fire time (cron type only).
	NextFireAt *metav1.Time `json:"nextFireAt,omitempty"`
	// FiredCount is the total number of times this trigger has fired.
	FiredCount int64 `json:"firedCount,omitempty"`
	// WebhookURL is the URL to POST to in order to fire this trigger (webhook type only).
	// Requires --trigger-webhook-url to be configured on the operator.
	WebhookURL string `json:"webhookURL,omitempty"`
	// ObservedGeneration is the .metadata.generation this status reflects.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions reflect the current state of the trigger.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.source.type`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspended`
// +kubebuilder:printcolumn:name="Fired",type=integer,JSONPath=`.status.firedCount`
// +kubebuilder:printcolumn:name="Last Fired",type=date,JSONPath=`.status.lastFiredAt`
// +kubebuilder:printcolumn:name="Next Fire",type=date,JSONPath=`.status.nextFireAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced

// ArkEvent fires ArkFlow runs in response to external events:
// a cron schedule, an inbound HTTP webhook, or another pipeline completing.
type ArkEvent struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec ArkEventSpec `json:"spec"`

	// +optional
	Status ArkEventStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArkEventList contains a list of ArkEvent.
type ArkEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArkEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArkEvent{}, &ArkEventList{})
}
