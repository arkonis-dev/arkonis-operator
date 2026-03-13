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

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentopsv1alpha1 "github.com/agentops-io/agentops-operator/api/v1alpha1"
)

var _ = Describe("AgentService Controller", func() {
	const (
		resourceName = "test-service"
		namespace    = "default"
	)

	ctx := context.Background()
	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	AfterEach(func() {
		svc := &agentopsv1alpha1.AgentService{}
		if err := k8sClient.Get(ctx, namespacedName, svc); err == nil {
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
		}
	})

	Context("When reconciling a valid AgentService", func() {
		BeforeEach(func() {
			By("creating an AgentService with a selector")
			resource := &agentopsv1alpha1.AgentService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: agentopsv1alpha1.AgentServiceSpec{
					Selector: agentopsv1alpha1.AgentServiceSelector{
						AgentDeployment: "research-agent",
					},
					Routing: agentopsv1alpha1.AgentServiceRouting{
						Strategy: agentopsv1alpha1.RoutingStrategyLeastBusy,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should reconcile without error", func() {
			By("running the reconciler")
			reconciler := &AgentServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set DeploymentNotFound condition when AgentDeployment is missing", func() {
			By("running the reconciler")
			reconciler := &AgentServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("fetching the updated AgentService status")
			svc := &agentopsv1alpha1.AgentService{}
			Expect(k8sClient.Get(ctx, namespacedName, svc)).To(Succeed())

			By("verifying the Ready condition is False with DeploymentNotFound reason")
			cond := apimeta.FindStatusCondition(svc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("DeploymentNotFound"))
		})
	})

	Context("When the referenced AgentDeployment exists", func() {
		const depName = "backing-agent"
		depKey := types.NamespacedName{Name: depName, Namespace: namespace}

		BeforeEach(func() {
			By("creating the backing AgentDeployment")
			replicas := int32(2)
			dep := &agentopsv1alpha1.AgentDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: namespace},
				Spec: agentopsv1alpha1.AgentDeploymentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are helpful.",
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			By("creating the AgentService referencing it")
			svc := &agentopsv1alpha1.AgentService{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentopsv1alpha1.AgentServiceSpec{
					Selector: agentopsv1alpha1.AgentServiceSelector{AgentDeployment: depName},
					Ports: []agentopsv1alpha1.AgentServicePort{
						{Protocol: agentopsv1alpha1.AgentProtocolHTTP, Port: 8081},
					},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		})

		AfterEach(func() {
			dep := &agentopsv1alpha1.AgentDeployment{}
			if err := k8sClient.Get(ctx, depKey, dep); err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
			k8sSvc := &corev1.Service{}
			if err := k8sClient.Get(ctx, namespacedName, k8sSvc); err == nil {
				Expect(k8sClient.Delete(ctx, k8sSvc)).To(Succeed())
			}
		})

		It("should create a backing k8s Service with the correct selector and owner ref", func() {
			reconciler := &AgentServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the k8s Service was created")
			k8sSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, namespacedName, k8sSvc)).To(Succeed())

			By("verifying the selector targets the AgentDeployment's pods")
			Expect(k8sSvc.Spec.Selector).To(HaveKeyWithValue("agentops.io/deployment", depName))

			By("verifying the port is set")
			Expect(k8sSvc.Spec.Ports).To(HaveLen(1))
			Expect(k8sSvc.Spec.Ports[0].Port).To(Equal(int32(8081)))

			By("verifying an owner reference points to the AgentService")
			Expect(k8sSvc.OwnerReferences).To(HaveLen(1))
			Expect(k8sSvc.OwnerReferences[0].Kind).To(Equal("AgentService"))
			Expect(k8sSvc.OwnerReferences[0].Name).To(Equal(resourceName))

			By("verifying the Ready condition is True")
			agsvc := &agentopsv1alpha1.AgentService{}
			Expect(k8sClient.Get(ctx, namespacedName, agsvc)).To(Succeed())
			cond := apimeta.FindStatusCondition(agsvc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When reconciling an AgentService with no selector", func() {
		BeforeEach(func() {
			By("creating an AgentService with empty selector")
			resource := &agentopsv1alpha1.AgentService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: agentopsv1alpha1.AgentServiceSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should reconcile without error and set NoSelector condition", func() {
			reconciler := &AgentServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			svc := &agentopsv1alpha1.AgentService{}
			Expect(k8sClient.Get(ctx, namespacedName, svc)).To(Succeed())

			cond := apimeta.FindStatusCondition(svc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("NoSelector"))
		})
	})
})
