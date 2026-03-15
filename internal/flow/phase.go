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

package flow

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// SumStepTokens accumulates input/output token counts across all completed steps
// and updates f.Status.TotalTokenUsage. Returns the grand total.
func SumStepTokens(f *arkonisv1alpha1.ArkFlow) int64 {
	var totalIn, totalOut int64
	for _, st := range f.Status.Steps {
		if st.TokenUsage != nil {
			totalIn += st.TokenUsage.InputTokens
			totalOut += st.TokenUsage.OutputTokens
		}
	}
	if totalIn > 0 || totalOut > 0 {
		f.Status.TotalTokenUsage = &arkonisv1alpha1.TokenUsage{
			InputTokens:  totalIn,
			OutputTokens: totalOut,
			TotalTokens:  totalIn + totalOut,
		}
	}
	return totalIn + totalOut
}

// EnforceTimeout sets the flow to Failed if the timeout has elapsed. Returns true if timed out.
func EnforceTimeout(f *arkonisv1alpha1.ArkFlow, now metav1.Time) bool {
	if f.Spec.TimeoutSeconds <= 0 || f.Status.StartTime == nil {
		return false
	}
	deadline := f.Status.StartTime.Add(time.Duration(f.Spec.TimeoutSeconds) * time.Second)
	if !now.After(deadline) {
		return false
	}
	f.Status.Phase = arkonisv1alpha1.ArkFlowPhaseFailed
	f.Status.CompletionTime = &now
	SetCondition(f, metav1.ConditionFalse, "TimedOut",
		fmt.Sprintf("flow exceeded timeout of %ds", f.Spec.TimeoutSeconds))
	return true
}

// UpdateFlowPhase inspects step statuses and transitions the flow to Succeeded or Failed.
func UpdateFlowPhase(f *arkonisv1alpha1.ArkFlow, templateData map[string]any) {
	now := metav1.Now()

	if EnforceTimeout(f, now) {
		return
	}

	totalTokens := SumStepTokens(f)

	if f.Spec.MaxTokens > 0 && totalTokens > f.Spec.MaxTokens {
		f.Status.Phase = arkonisv1alpha1.ArkFlowPhaseFailed
		f.Status.CompletionTime = &now
		SetCondition(f, metav1.ConditionFalse, "BudgetExceeded",
			fmt.Sprintf("token budget of %d exceeded: used %d", f.Spec.MaxTokens, totalTokens))
		return
	}

	failed, allDone := false, true
	for _, st := range f.Status.Steps {
		switch st.Phase {
		case arkonisv1alpha1.ArkFlowStepPhaseFailed:
			failed = true
		case arkonisv1alpha1.ArkFlowStepPhaseSucceeded, arkonisv1alpha1.ArkFlowStepPhaseSkipped:
			// ok — both count as done
		default:
			allDone = false
		}
	}

	switch {
	case failed:
		f.Status.Phase = arkonisv1alpha1.ArkFlowPhaseFailed
		f.Status.CompletionTime = &now
		SetCondition(f, metav1.ConditionFalse, "StepFailed", "one or more steps failed")
	case allDone:
		f.Status.Phase = arkonisv1alpha1.ArkFlowPhaseSucceeded
		f.Status.CompletionTime = &now
		if f.Spec.Output != "" {
			out, _ := ResolveTemplate(f.Spec.Output, templateData)
			f.Status.Output = out
		}
		SetCondition(f, metav1.ConditionTrue, "Succeeded", "all steps completed successfully")
	}
}
