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

	agentopsv1alpha1 "github.com/agentops-io/agentops-operator/api/v1alpha1"
)

const testAgentImage = "ghcr.io/agentops-io/agentops-runtime:latest"

var _ = Describe("AgentDeployment Controller", func() {
	const (
		resourceName = "test-agent"
		namespace    = "default"
	)

	ctx := context.Background()

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
	backingDeploymentName := types.NamespacedName{Name: resourceName + "-agent", Namespace: namespace}

	AfterEach(func() {
		ad := &agentopsv1alpha1.AgentDeployment{}
		if err := k8sClient.Get(ctx, namespacedName, ad); err == nil {
			Expect(k8sClient.Delete(ctx, ad)).To(Succeed())
		}
		dep := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, backingDeploymentName, dep); err == nil {
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		}
	})

	Context("When reconciling a valid AgentDeployment", func() {
		BeforeEach(func() {
			By("creating an AgentDeployment with required fields")
			replicas := int32(2)
			resource := &agentopsv1alpha1.AgentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: agentopsv1alpha1.AgentDeploymentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are a helpful assistant.",
				},
			}
			err := k8sClient.Get(ctx, namespacedName, &agentopsv1alpha1.AgentDeployment{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		It("should create a backing Deployment", func() {
			By("running the reconciler")
			reconciler := &AgentDeploymentReconciler{
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
			Expect(dep.Spec.Selector.MatchLabels).To(HaveKey("agentops.io/deployment"))
			Expect(dep.Spec.Selector.MatchLabels["agentops.io/deployment"]).To(Equal(resourceName))

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
			reconciler := &AgentDeploymentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile picks up the Deployment status.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("fetching the updated AgentDeployment status")
			ad := &agentopsv1alpha1.AgentDeployment{}
			Expect(k8sClient.Get(ctx, namespacedName, ad)).To(Succeed())

			By("verifying the Ready condition is present")
			cond := apimeta.FindStatusCondition(ad.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
		})

		It("should set an owner reference on the backing Deployment", func() {
			By("running the reconciler")
			reconciler := &AgentDeploymentReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: testAgentImage,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Deployment has an owner reference pointing to the AgentDeployment")
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, backingDeploymentName, dep)).To(Succeed())
			Expect(dep.OwnerReferences).To(HaveLen(1))
			Expect(dep.OwnerReferences[0].Kind).To(Equal("AgentDeployment"))
			Expect(dep.OwnerReferences[0].Name).To(Equal(resourceName))
		})
	})

	Context("When the AgentDeployment is deleted", func() {
		It("should reconcile without error for a missing resource", func() {
			reconciler := &AgentDeploymentReconciler{
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
})
