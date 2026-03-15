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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// SetCondition updates the "Ready" condition on the flow status.
func SetCondition(f *arkonisv1alpha1.ArkFlow, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&f.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: f.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// InitializeSteps sets up step statuses and marks the flow Running on the first reconcile.
func InitializeSteps(f *arkonisv1alpha1.ArkFlow) {
	if f.Status.Phase != "" {
		return
	}
	now := metav1.Now()
	f.Status.Phase = arkonisv1alpha1.ArkFlowPhaseRunning
	f.Status.StartTime = &now
	f.Status.Steps = make([]arkonisv1alpha1.ArkFlowStepStatus, len(f.Spec.Steps))
	for i, step := range f.Spec.Steps {
		f.Status.Steps[i] = arkonisv1alpha1.ArkFlowStepStatus{
			Name:  step.Name,
			Phase: arkonisv1alpha1.ArkFlowStepPhasePending,
		}
	}
	SetCondition(f, metav1.ConditionTrue, "Validated", "Flow DAG is valid; execution started")
}

// ParseOutputJSON tries to parse completed step outputs as JSON when the step declared an OutputSchema.
// Parsed results are stored in OutputJSON so downstream templates can reference individual fields.
func ParseOutputJSON(
	f *arkonisv1alpha1.ArkFlow,
	statusByName map[string]*arkonisv1alpha1.ArkFlowStepStatus,
) {
	schemaByName := make(map[string]string, len(f.Spec.Steps))
	for _, step := range f.Spec.Steps {
		if step.OutputSchema != "" {
			schemaByName[step.Name] = step.OutputSchema
		}
	}
	for name, st := range statusByName {
		if _, hasSchema := schemaByName[name]; !hasSchema {
			continue
		}
		if st.Phase != arkonisv1alpha1.ArkFlowStepPhaseSucceeded || st.Output == "" || st.OutputJSON != "" {
			continue
		}
		if raw := ExtractJSON(st.Output); raw != "" {
			var check any
			if json.Unmarshal([]byte(raw), &check) == nil {
				st.OutputJSON = raw
			}
		}
	}
}

// EvaluateLoops checks every Succeeded step that has a Loop spec. If the loop condition
// is still truthy and max iterations haven't been reached, the step is reset to Pending
// so it will be re-submitted on the next pass.
func EvaluateLoops(
	f *arkonisv1alpha1.ArkFlow,
	statusByName map[string]*arkonisv1alpha1.ArkFlowStepStatus,
	templateData map[string]any,
) {
	for _, step := range f.Spec.Steps {
		if step.Loop == nil {
			continue
		}
		st := statusByName[step.Name]
		if st == nil || st.Phase != arkonisv1alpha1.ArkFlowStepPhaseSucceeded {
			continue
		}
		maxIter := step.Loop.MaxIterations
		if maxIter <= 0 {
			maxIter = 10
		}
		if st.Iterations >= maxIter {
			continue
		}
		condResult, err := ResolveTemplate(step.Loop.Condition, templateData)
		if err != nil || !IsTruthy(condResult) {
			continue
		}
		st.Iterations++
		st.Phase = arkonisv1alpha1.ArkFlowStepPhasePending
		st.TaskID = ""
		st.Output = ""
		st.OutputJSON = ""
		st.StartTime = nil
		st.CompletionTime = nil
		st.Message = fmt.Sprintf("loop iteration %d/%d", st.Iterations, maxIter)
	}
}

// ExtractJSON returns the first JSON object or array found in s.
// It handles two cases:
//  1. The whole string is valid JSON — returned as-is.
//  2. JSON is wrapped in a markdown code fence (```json ... ``` or ``` ... ```) —
//     the fenced block is extracted and returned.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var check any
	if json.Unmarshal([]byte(s), &check) == nil {
		return s
	}
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			continue
		}
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			start := strings.Index(s, line)
			if start < 0 {
				break
			}
			end := strings.Index(s[start:], "\n```")
			if end < 0 {
				candidate := strings.TrimSpace(s[start:])
				if json.Unmarshal([]byte(candidate), &check) == nil {
					return candidate
				}
				break
			}
			candidate := strings.TrimSpace(s[start : start+end])
			if json.Unmarshal([]byte(candidate), &check) == nil {
				return candidate
			}
			break
		}
	}
	return ""
}

// ToInt64 coerces a Redis value (string or int64) to int64.
func ToInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}
