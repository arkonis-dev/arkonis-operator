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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

const testAgentImage = "ghcr.io/arkonis-dev/ark-runtime:latest"

var _ = Describe("ArkAgent Controller", func() {
	const (
		resourceName = "test-agent"
		namespace    = "default"
	)

	ctx := context.Background()

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
	backingDeploymentName := types.NamespacedName{Name: resourceName + "-agent", Namespace: namespace}

	AfterEach(func() {
		ad := &arkonisv1alpha1.ArkAgent{}
		if err := k8sClient.Get(ctx, namespacedName, ad); err == nil {
			Expect(k8sClient.Delete(ctx, ad)).To(Succeed())
		}
		dep := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, backingDeploymentName, dep); err == nil {
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		}
	})

	Context("When reconciling a valid ArkAgent", func() {
		BeforeEach(func() {
			By("creating an ArkAgent with required fields")
			replicas := int32(2)
			resource := &arkonisv1alpha1.ArkAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkAgentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are a helpful assistant.",
				},
			}
			err := k8sClient.Get(ctx, namespacedName, &arkonisv1alpha1.ArkAgent{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		It("should create a backing Deployment", func() {
			By("running the reconciler")
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the backing Deployment was created")
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, backingDeploymentName, dep)).To(Succeed())

			By("verifying the Deployment has the correct replica count")
			Expect(dep.Spec.Replicas).NotTo(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(2)))

			By("verifying the Deployment has the agent selector label")
			Expect(dep.Spec.Selector.MatchLabels).To(HaveKey("arkonis.dev/deployment"))
			Expect(dep.Spec.Selector.MatchLabels["arkonis.dev/deployment"]).To(Equal(resourceName))

			By("verifying the container uses the agent runtime image")
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("agent"))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(testAgentImage))

			By("verifying AGENT_MODEL env var is set correctly")
			envVars := dep.Spec.Template.Spec.Containers[0].Env
			var modelEnv string
			for _, e := range envVars {
				if e.Name == "AGENT_MODEL" {
					modelEnv = e.Value
				}
			}
			Expect(modelEnv).To(Equal("claude-haiku-4-5"))
		})

		It("should set status conditions after reconciliation", func() {
			By("running the reconciler twice (create + sync status)")
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile picks up the Deployment status.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("fetching the updated ArkAgent status")
			ad := &arkonisv1alpha1.ArkAgent{}
			Expect(k8sClient.Get(ctx, namespacedName, ad)).To(Succeed())

			By("verifying the Ready condition is present")
			cond := apimeta.FindStatusCondition(ad.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
		})

		It("should set an owner reference on the backing Deployment", func() {
			By("running the reconciler")
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Deployment has an owner reference pointing to the ArkAgent")
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, backingDeploymentName, dep)).To(Succeed())
			Expect(dep.OwnerReferences).To(HaveLen(1))
			Expect(dep.OwnerReferences[0].Kind).To(Equal("ArkAgent"))
			Expect(dep.OwnerReferences[0].Name).To(Equal(resourceName))
		})
	})

	Context("When an ArkAgent has inline webhook tools", func() {
		const toolsName = "tools-agent"
		toolsKey := types.NamespacedName{Name: toolsName, Namespace: namespace}
		toolsBackingKey := types.NamespacedName{Name: toolsName + "-agent", Namespace: namespace}

		BeforeEach(func() {
			replicas := int32(1)
			Expect(k8sClient.Create(ctx, &arkonisv1alpha1.ArkAgent{
				ObjectMeta: metav1.ObjectMeta{Name: toolsName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkAgentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are helpful.",
					Tools: []arkonisv1alpha1.WebhookToolSpec{
						{
							Name:        "fetch_news",
							Description: "Get news",
							URL:         "http://news.internal/headlines",
							Method:      "POST",
						},
					},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			ad := &arkonisv1alpha1.ArkAgent{}
			if err := k8sClient.Get(ctx, toolsKey, ad); err == nil {
				Expect(k8sClient.Delete(ctx, ad)).To(Succeed())
			}
			dep := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, toolsBackingKey, dep); err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
		})

		It("should inject AGENT_WEBHOOK_TOOLS env var into the backing Deployment", func() {
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: toolsKey})
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, toolsBackingKey, dep)).To(Succeed())

			envMap := make(map[string]string)
			for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
				envMap[e.Name] = e.Value
			}

			Expect(envMap).To(HaveKey("AGENT_WEBHOOK_TOOLS"))
			Expect(envMap["AGENT_WEBHOOK_TOOLS"]).To(ContainSubstring("fetch_news"))
			Expect(envMap["AGENT_WEBHOOK_TOOLS"]).To(ContainSubstring("news.internal"))
		})
	})

	Context("When the ArkAgent is deleted", func() {
		It("should reconcile without error for a missing resource", func() {
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When an ArkAgent references an ArkMemory", func() {
		const (
			agentName = "mem-agent"
			memName   = "test-mem"
		)
		agentKey := types.NamespacedName{Name: agentName, Namespace: namespace}
		memKey := types.NamespacedName{Name: memName, Namespace: namespace}
		backingKey := types.NamespacedName{Name: agentName + "-agent", Namespace: namespace}

		BeforeEach(func() {
			By("creating an ArkMemory with redis backend")
			Expect(k8sClient.Create(ctx, &arkonisv1alpha1.ArkMemory{
				ObjectMeta: metav1.ObjectMeta{Name: memName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkMemorySpec{
					Backend: arkonisv1alpha1.MemoryBackendRedis,
					Redis: &arkonisv1alpha1.RedisMemoryConfig{
						SecretRef:  arkonisv1alpha1.LocalObjectReference{Name: "redis-secret"},
						TTLSeconds: 1800,
					},
				},
			})).To(Succeed())

			By("creating an ArkAgent that references the ArkMemory")
			replicas := int32(1)
			Expect(k8sClient.Create(ctx, &arkonisv1alpha1.ArkAgent{
				ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkAgentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are a helpful assistant.",
					MemoryRef:    &arkonisv1alpha1.LocalObjectReference{Name: memName},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			ad := &arkonisv1alpha1.ArkAgent{}
			if err := k8sClient.Get(ctx, agentKey, ad); err == nil {
				Expect(k8sClient.Delete(ctx, ad)).To(Succeed())
			}
			mem := &arkonisv1alpha1.ArkMemory{}
			if err := k8sClient.Get(ctx, memKey, mem); err == nil {
				Expect(k8sClient.Delete(ctx, mem)).To(Succeed())
			}
			dep := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, backingKey, dep); err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
		})

		It("should inject AGENT_MEMORY_BACKEND env var into the backing Deployment", func() {
			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentKey})
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, backingKey, dep)).To(Succeed())

			envVars := dep.Spec.Template.Spec.Containers[0].Env
			envMap := make(map[string]string)
			for _, e := range envVars {
				envMap[e.Name] = e.Value
			}

			Expect(envMap).To(HaveKeyWithValue("AGENT_MEMORY_BACKEND", string(arkonisv1alpha1.MemoryBackendRedis)))
			Expect(envMap).To(HaveKeyWithValue("AGENT_MEMORY_REDIS_TTL", "1800"))
		})

		It("should reconcile without error when the referenced ArkMemory does not exist", func() {
			By("deleting the ArkMemory before reconciling")
			mem := &arkonisv1alpha1.ArkMemory{}
			Expect(k8sClient.Get(ctx, memKey, mem)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mem)).To(Succeed())

			reconciler := &ArkAgentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentKey})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
