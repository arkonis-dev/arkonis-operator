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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// IsTruthy returns false for blank, "false", "0", or "no" (case-insensitive).
func IsTruthy(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s != "" && s != "false" && s != "0" && s != "no"
}

// ResolveTemplate executes a Go template string against the provided data.
func ResolveTemplate(tmplStr string, data map[string]any) (string, error) {
	t, err := template.New("").Option("missingkey=zero").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ResolvePrompt resolves all step input templates and concatenates them into a prompt string.
// When the step has an OutputSchema, the schema is appended as an instruction so the
// agent knows to respond with JSON matching that shape.
func ResolvePrompt(step arkonisv1alpha1.ArkFlowStep, data map[string]any) (string, error) {
	var buf bytes.Buffer
	for key, tmplStr := range step.Inputs {
		resolved, err := ResolveTemplate(tmplStr, data)
		if err != nil {
			return "", fmt.Errorf("input %q: %w", key, err)
		}
		fmt.Fprintf(&buf, "%s: %s\n", key, resolved)
	}
	if step.OutputSchema != "" {
		fmt.Fprintf(&buf, "\nRespond with valid JSON matching this schema:\n%s\n", step.OutputSchema)
	}
	return buf.String(), nil
}

// BuildTemplateData assembles the Go template context from flow inputs and completed step outputs.
// Each step entry exposes:
//   - .steps.<name>.output  — raw text response
//   - .steps.<name>.data    — parsed JSON map (only when OutputJSON is populated)
func BuildTemplateData(
	f *arkonisv1alpha1.ArkFlow,
	statusByName map[string]*arkonisv1alpha1.ArkFlowStepStatus,
) map[string]any {
	stepsData := make(map[string]any, len(f.Status.Steps))
	for name, st := range statusByName {
		entry := map[string]any{"output": st.Output}
		if st.OutputJSON != "" {
			var parsed any
			if json.Unmarshal([]byte(st.OutputJSON), &parsed) == nil {
				entry["data"] = parsed
			}
		}
		stepsData[name] = entry
	}
	return map[string]any{
		"pipeline": map[string]any{"input": f.Spec.Input},
		"steps":    stepsData,
	}
}
