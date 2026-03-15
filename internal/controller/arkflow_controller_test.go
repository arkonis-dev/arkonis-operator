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

package controller

import (
	"context"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
	"github.com/arkonis-dev/ark-operator/internal/flow"
)

var _ = Describe("ArkFlow Controller", func() {
	const (
		resourceName = "test-flow"
		namespace    = "default"
	)

	ctx := context.Background()
	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	AfterEach(func() {
		flow := &arkonisv1alpha1.ArkFlow{}
		if err := k8sClient.Get(ctx, namespacedName, flow); err == nil {
			Expect(k8sClient.Delete(ctx, flow)).To(Succeed())
		}
	})

	Context("When a step references an unknown dependency (invalid DAG)", func() {
		BeforeEach(func() {
			By("creating a flow where a step depends on a step that does not exist")
			resource := &arkonisv1alpha1.ArkFlow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkFlowSpec{
					Steps: []arkonisv1alpha1.ArkFlowStep{
						{
							Name:      "summarize",
							ArkAgent:  "summarizer-agent",
							DependsOn: []string{"nonexistent-step"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should set Ready=False with reason InvalidDAG", func() {
			By("running the reconciler")
			r := &ArkFlowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("fetching the updated flow status")
			flow := &arkonisv1alpha1.ArkFlow{}
			Expect(k8sClient.Get(ctx, namespacedName, flow)).To(Succeed())

			cond := apimeta.FindStatusCondition(flow.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InvalidDAG"))
			Expect(cond.Message).To(ContainSubstring("nonexistent-step"))
		})
	})

	Context("When the referenced ArkAgent is missing", func() {
		BeforeEach(func() {
			By("creating an ArkFlow referencing a nonexistent ArkAgent")
			resource := &arkonisv1alpha1.ArkFlow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkFlowSpec{
					Steps: []arkonisv1alpha1.ArkFlowStep{
						{
							Name:     "research",
							ArkAgent: "research-agent",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should set Ready=False with reason DeploymentNotFound", func() {
			By("running the reconciler")
			r := &ArkFlowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			flow := &arkonisv1alpha1.ArkFlow{}
			Expect(k8sClient.Get(ctx, namespacedName, flow)).To(Succeed())

			cond := apimeta.FindStatusCondition(flow.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("DeploymentNotFound"))
		})
	})

	Context("When the flow is valid but no Redis is configured", func() {
		const arkAgentName = "flow-agent"
		arkAgentKey := types.NamespacedName{Name: arkAgentName, Namespace: namespace}

		BeforeEach(func() {
			By("creating the ArkAgent the flow references")
			replicas := int32(1)
			dep := &arkonisv1alpha1.ArkAgent{
				ObjectMeta: metav1.ObjectMeta{Name: arkAgentName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkAgentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are helpful.",
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			By("creating the ArkFlow with a valid step")
			resource := &arkonisv1alpha1.ArkFlow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkFlowSpec{
					Steps: []arkonisv1alpha1.ArkFlowStep{
						{
							Name:     "research",
							ArkAgent: arkAgentName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			dep := &arkonisv1alpha1.ArkAgent{}
			if err := k8sClient.Get(ctx, arkAgentKey, dep); err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
		})

		It("should set Ready=False with reason NoTaskQueue", func() {
			By("running the reconciler with no TaskQueueURL set")
			r := &ArkFlowReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				TaskQueueURL: "", // no Redis
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			flow := &arkonisv1alpha1.ArkFlow{}
			Expect(k8sClient.Get(ctx, namespacedName, flow)).To(Succeed())

			cond := apimeta.FindStatusCondition(flow.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NoTaskQueue"))
		})
	})

	Context("When reconciling a nonexistent ArkFlow", func() {
		It("should return without error", func() {
			r := &ArkFlowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When a flow has a circular dependency", func() {
		BeforeEach(func() {
			By("creating a flow where step A depends on step B which depends on step A")
			resource := &arkonisv1alpha1.ArkFlow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkFlowSpec{
					Steps: []arkonisv1alpha1.ArkFlowStep{
						{
							Name:      "step-a",
							ArkAgent:  "some-agent",
							DependsOn: []string{"step-b"},
						},
						{
							Name:      "step-b",
							ArkAgent:  "some-agent",
							DependsOn: []string{"step-a"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should set Ready=False with reason InvalidDAG mentioning a cycle", func() {
			r := &ArkFlowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			flow := &arkonisv1alpha1.ArkFlow{}
			Expect(k8sClient.Get(ctx, namespacedName, flow)).To(Succeed())

			cond := apimeta.FindStatusCondition(flow.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InvalidDAG"))
			Expect(cond.Message).To(ContainSubstring("cycle"))
		})
	})
})

// Unit tests for pure helper functions — no k8s client or Redis required.

var _ = Describe("isTruthy", func() {
	DescribeTable("evaluates strings correctly",
		func(input string, expected bool) {
			Expect(flow.IsTruthy(input)).To(Equal(expected))
		},
		Entry("empty string is falsy", "", false),
		Entry("false is falsy", "false", false),
		Entry("FALSE is falsy", "FALSE", false),
		Entry("0 is falsy", "0", false),
		Entry("no is falsy", "no", false),
		Entry("true is truthy", "true", true),
		Entry("1 is truthy", "1", true),
		Entry("yes is truthy", "yes", true),
		Entry("non-empty string is truthy", "some output", true),
		Entry("whitespace-only false is falsy", "  false  ", false),
	)
})

var _ = Describe("flow package unit tests", func() {
	Describe("ValidateDAG", func() {
		It("passes a valid linear DAG", func() {
			f := flowWithSteps(
				flowStep("a", nil),
				flowStep("b", []string{"a"}),
				flowStep("c", []string{"b"}),
			)
			Expect(flow.ValidateDAG(f)).To(Succeed())
		})

		It("rejects an unknown dependsOn", func() {
			f := flowWithSteps(flowStep("a", []string{"ghost"}))
			Expect(flow.ValidateDAG(f)).To(MatchError(ContainSubstring("ghost")))
		})

		It("detects a direct cycle", func() {
			f := flowWithSteps(
				flowStep("a", []string{"b"}),
				flowStep("b", []string{"a"}),
			)
			Expect(flow.ValidateDAG(f)).To(MatchError(ContainSubstring("cycle")))
		})

		It("detects a three-node cycle", func() {
			f := flowWithSteps(
				flowStep("a", []string{"c"}),
				flowStep("b", []string{"a"}),
				flowStep("c", []string{"b"}),
			)
			Expect(flow.ValidateDAG(f)).To(MatchError(ContainSubstring("cycle")))
		})
	})

	Describe("DepsSucceeded", func() {
		It("returns true when all deps are Succeeded", func() {
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{
				"a": {Phase: arkonisv1alpha1.ArkFlowStepPhaseSucceeded},
			}
			Expect(flow.DepsSucceeded([]string{"a"}, statusByName)).To(BeTrue())
		})

		It("returns true when a dep is Skipped", func() {
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{
				"a": {Phase: arkonisv1alpha1.ArkFlowStepPhaseSkipped},
			}
			Expect(flow.DepsSucceeded([]string{"a"}, statusByName)).To(BeTrue())
		})

		It("returns false when a dep is still Running", func() {
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{
				"a": {Phase: arkonisv1alpha1.ArkFlowStepPhaseRunning},
			}
			Expect(flow.DepsSucceeded([]string{"a"}, statusByName)).To(BeFalse())
		})

		It("returns false when a dep is missing from status", func() {
			Expect(flow.DepsSucceeded([]string{"missing"}, map[string]*arkonisv1alpha1.ArkFlowStepStatus{})).To(BeFalse())
		})
	})

	Describe("EvaluateLoops", func() {
		It("resets a Succeeded loop step to Pending when condition is truthy", func() {
			f := flowWithSteps(arkonisv1alpha1.ArkFlowStep{
				Name:     "collect",
				ArkAgent: "agent",
				Loop:     &arkonisv1alpha1.LoopSpec{Condition: "true", MaxIterations: 3},
			})
			st := &arkonisv1alpha1.ArkFlowStepStatus{
				Name:  "collect",
				Phase: arkonisv1alpha1.ArkFlowStepPhaseSucceeded,
			}
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{"collect": st}
			flow.EvaluateLoops(f, statusByName, map[string]any{})

			Expect(st.Phase).To(Equal(arkonisv1alpha1.ArkFlowStepPhasePending))
			Expect(st.Iterations).To(Equal(1))
		})

		It("leaves a Succeeded step as-is when condition is falsy", func() {
			f := flowWithSteps(arkonisv1alpha1.ArkFlowStep{
				Name:     "collect",
				ArkAgent: "agent",
				Loop:     &arkonisv1alpha1.LoopSpec{Condition: "false", MaxIterations: 3},
			})
			st := &arkonisv1alpha1.ArkFlowStepStatus{
				Name:  "collect",
				Phase: arkonisv1alpha1.ArkFlowStepPhaseSucceeded,
			}
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{"collect": st}
			flow.EvaluateLoops(f, statusByName, map[string]any{})

			Expect(st.Phase).To(Equal(arkonisv1alpha1.ArkFlowStepPhaseSucceeded))
		})

		It("stops looping after MaxIterations", func() {
			f := flowWithSteps(arkonisv1alpha1.ArkFlowStep{
				Name:     "collect",
				ArkAgent: "agent",
				Loop:     &arkonisv1alpha1.LoopSpec{Condition: "true", MaxIterations: 2},
			})
			st := &arkonisv1alpha1.ArkFlowStepStatus{
				Name:       "collect",
				Phase:      arkonisv1alpha1.ArkFlowStepPhaseSucceeded,
				Iterations: 2, // already at max
			}
			statusByName := map[string]*arkonisv1alpha1.ArkFlowStepStatus{"collect": st}
			flow.EvaluateLoops(f, statusByName, map[string]any{})

			Expect(st.Phase).To(Equal(arkonisv1alpha1.ArkFlowStepPhaseSucceeded))
		})
	})

	Describe("UpdateFlowPhase — timeout", func() {
		It("fails the flow when TimeoutSeconds has elapsed", func() {
			past := metav1.NewTime(metav1.Now().Add(-600 * 1e9)) // 600s ago
			f := &arkonisv1alpha1.ArkFlow{
				Spec: arkonisv1alpha1.ArkFlowSpec{
					TimeoutSeconds: 300,
				},
				Status: arkonisv1alpha1.ArkFlowStatus{
					Phase:     arkonisv1alpha1.ArkFlowPhaseRunning,
					StartTime: &past,
					Steps: []arkonisv1alpha1.ArkFlowStepStatus{
						{Name: "s", Phase: arkonisv1alpha1.ArkFlowStepPhaseRunning},
					},
				},
			}
			flow.UpdateFlowPhase(f, map[string]any{})

			Expect(f.Status.Phase).To(Equal(arkonisv1alpha1.ArkFlowPhaseFailed))
			cond := apimeta.FindStatusCondition(f.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("TimedOut"))
		})

		It("does not fail the flow when within timeout", func() {
			now := metav1.Now()
			f := &arkonisv1alpha1.ArkFlow{
				Spec: arkonisv1alpha1.ArkFlowSpec{
					TimeoutSeconds: 3600,
				},
				Status: arkonisv1alpha1.ArkFlowStatus{
					Phase:     arkonisv1alpha1.ArkFlowPhaseRunning,
					StartTime: &now,
					Steps: []arkonisv1alpha1.ArkFlowStepStatus{
						{Name: "s", Phase: arkonisv1alpha1.ArkFlowStepPhaseRunning},
					},
				},
			}
			flow.UpdateFlowPhase(f, map[string]any{})

			Expect(f.Status.Phase).To(Equal(arkonisv1alpha1.ArkFlowPhaseRunning))
		})
	})
})

// helpers for building test fixtures.

func flowStep(name string, deps []string) arkonisv1alpha1.ArkFlowStep {
	return arkonisv1alpha1.ArkFlowStep{
		Name:      name,
		ArkAgent:  "test-agent",
		DependsOn: deps,
	}
}

func flowWithSteps(steps ...arkonisv1alpha1.ArkFlowStep) *arkonisv1alpha1.ArkFlow {
	return &arkonisv1alpha1.ArkFlow{
		Spec: arkonisv1alpha1.ArkFlowSpec{Steps: steps},
	}
}
