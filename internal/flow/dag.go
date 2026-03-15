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

// Package flow contains pure business logic for ArkFlow DAG execution.
// It has no dependency on Kubernetes controller-runtime or Redis —
// the operator and the ark CLI both import it.
package flow

import (
	"fmt"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// IsTerminalPhase reports whether the flow has reached a final state.
func IsTerminalPhase(phase arkonisv1alpha1.ArkFlowPhase) bool {
	return phase == arkonisv1alpha1.ArkFlowPhaseSucceeded || phase == arkonisv1alpha1.ArkFlowPhaseFailed
}

// BuildStatusByName returns a name→status pointer map for convenient step lookups.
func BuildStatusByName(f *arkonisv1alpha1.ArkFlow) map[string]*arkonisv1alpha1.ArkFlowStepStatus {
	m := make(map[string]*arkonisv1alpha1.ArkFlowStepStatus, len(f.Status.Steps))
	for i := range f.Status.Steps {
		m[f.Status.Steps[i].Name] = &f.Status.Steps[i]
	}
	return m
}

// DepsSucceeded returns true when every name in deps has completed (Succeeded or Skipped).
func DepsSucceeded(deps []string, statusByName map[string]*arkonisv1alpha1.ArkFlowStepStatus) bool {
	for _, dep := range deps {
		st, ok := statusByName[dep]
		if !ok {
			return false
		}
		if st.Phase != arkonisv1alpha1.ArkFlowStepPhaseSucceeded &&
			st.Phase != arkonisv1alpha1.ArkFlowStepPhaseSkipped {
			return false
		}
	}
	return true
}

// ValidateDAG checks that every dependsOn entry names a known step and that there are no cycles.
func ValidateDAG(f *arkonisv1alpha1.ArkFlow) error {
	stepNames := make(map[string]struct{}, len(f.Spec.Steps))
	for _, step := range f.Spec.Steps {
		stepNames[step.Name] = struct{}{}
	}
	for _, step := range f.Spec.Steps {
		for _, dep := range step.DependsOn {
			if _, ok := stepNames[dep]; !ok {
				return fmt.Errorf("step %q depends on unknown step %q", step.Name, dep)
			}
		}
	}
	adj := make(map[string][]string, len(f.Spec.Steps))
	for _, step := range f.Spec.Steps {
		adj[step.Name] = step.DependsOn
	}
	const white, gray, black = 0, 1, 2
	color := make(map[string]int, len(f.Spec.Steps))
	var dfs func(name string) error
	dfs = func(name string) error {
		color[name] = gray
		for _, dep := range adj[name] {
			switch color[dep] {
			case gray:
				return fmt.Errorf("cycle detected: step %q → %q forms a cycle", name, dep)
			case white:
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		color[name] = black
		return nil
	}
	for _, step := range f.Spec.Steps {
		if color[step.Name] == white {
			if err := dfs(step.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
