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

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

var _ = Describe("ArkService Controller", func() {
	const (
		resourceName = "test-service"
		namespace    = "default"
	)

	ctx := context.Background()
	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	AfterEach(func() {
		svc := &arkonisv1alpha1.ArkService{}
		if err := k8sClient.Get(ctx, namespacedName, svc); err == nil {
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
		}
	})

	Context("When reconciling a valid ArkService", func() {
		BeforeEach(func() {
			By("creating an ArkService with a selector")
			resource := &arkonisv1alpha1.ArkService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkServiceSpec{
					Selector: arkonisv1alpha1.ArkServiceSelector{
						ArkAgent: "research-agent",
					},
					Routing: arkonisv1alpha1.ArkServiceRouting{
						Strategy: arkonisv1alpha1.RoutingStrategyLeastBusy,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should reconcile without error", func() {
			By("running the reconciler")
			reconciler := &ArkServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set DeploymentNotFound condition when ArkAgent is missing", func() {
			By("running the reconciler")
			reconciler := &ArkServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("fetching the updated ArkService status")
			svc := &arkonisv1alpha1.ArkService{}
			Expect(k8sClient.Get(ctx, namespacedName, svc)).To(Succeed())

			By("verifying the Ready condition is False with DeploymentNotFound reason")
			cond := apimeta.FindStatusCondition(svc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("DeploymentNotFound"))
		})
	})

	Context("When the referenced ArkAgent exists", func() {
		const agentName = "backing-agent"
		agentKey := types.NamespacedName{Name: agentName, Namespace: namespace}

		BeforeEach(func() {
			By("creating the backing ArkAgent")
			replicas := int32(2)
			dep := &arkonisv1alpha1.ArkAgent{
				ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkAgentSpec{
					Replicas:     &replicas,
					Model:        "claude-haiku-4-5",
					SystemPrompt: "You are helpful.",
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			By("creating the ArkService referencing it")
			svc := &arkonisv1alpha1.ArkService{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: arkonisv1alpha1.ArkServiceSpec{
					Selector: arkonisv1alpha1.ArkServiceSelector{ArkAgent: agentName},
					Ports: []arkonisv1alpha1.ArkServicePort{
						{Protocol: arkonisv1alpha1.AgentProtocolHTTP, Port: 8081},
					},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		})

		AfterEach(func() {
			dep := &arkonisv1alpha1.ArkAgent{}
			if err := k8sClient.Get(ctx, agentKey, dep); err == nil {
				Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			}
			k8sSvc := &corev1.Service{}
			if err := k8sClient.Get(ctx, namespacedName, k8sSvc); err == nil {
				Expect(k8sClient.Delete(ctx, k8sSvc)).To(Succeed())
			}
		})

		It("should create a backing k8s Service with the correct selector and owner ref", func() {
			reconciler := &ArkServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the k8s Service was created")
			k8sSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, namespacedName, k8sSvc)).To(Succeed())

			By("verifying the selector targets the ArkAgent's pods")
			Expect(k8sSvc.Spec.Selector).To(HaveKeyWithValue("arkonis.dev/deployment", agentName))

			By("verifying the port is set")
			Expect(k8sSvc.Spec.Ports).To(HaveLen(1))
			Expect(k8sSvc.Spec.Ports[0].Port).To(Equal(int32(8081)))

			By("verifying an owner reference points to the ArkService")
			Expect(k8sSvc.OwnerReferences).To(HaveLen(1))
			Expect(k8sSvc.OwnerReferences[0].Kind).To(Equal("ArkService"))
			Expect(k8sSvc.OwnerReferences[0].Name).To(Equal(resourceName))

			By("verifying the Ready condition is True")
			arksvc := &arkonisv1alpha1.ArkService{}
			Expect(k8sClient.Get(ctx, namespacedName, arksvc)).To(Succeed())
			cond := apimeta.FindStatusCondition(arksvc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When reconciling an ArkService with no selector", func() {
		BeforeEach(func() {
			By("creating an ArkService with empty selector")
			resource := &arkonisv1alpha1.ArkService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: arkonisv1alpha1.ArkServiceSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should reconcile without error and set NoSelector condition", func() {
			reconciler := &ArkServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			svc := &arkonisv1alpha1.ArkService{}
			Expect(k8sClient.Get(ctx, namespacedName, svc)).To(Succeed())

			cond := apimeta.FindStatusCondition(svc.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("NoSelector"))
		})
	})
})
