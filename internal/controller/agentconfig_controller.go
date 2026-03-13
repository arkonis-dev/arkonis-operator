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

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/agentops-io/agentops-operator/api/v1alpha1"
)

// AgentConfigReconciler reconciles a AgentConfig object
type AgentConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agentops.agentops.io,resources=agentconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentops.agentops.io,resources=agentconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentops.agentops.io,resources=agentconfigs/finalizers,verbs=update

// AgentConfig is a storage-only resource (analogous to ConfigMap).
// The reconciler just acknowledges the resource and sets a Ready condition.
// AgentDeployments reference it by name; the operator reads it during pod construction.
func (r *AgentConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	agentCfg := &agentopsv1alpha1.AgentConfig{}
	if err := r.Get(ctx, req.NamespacedName, agentCfg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !agentCfg.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	agentCfg.Status.ObservedGeneration = agentCfg.Generation
	apimeta.SetStatusCondition(&agentCfg.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: agentCfg.Generation,
		Reason:             "Accepted",
		Message:            "AgentConfig is valid and available",
	})

	return ctrl.Result{}, r.Status().Update(ctx, agentCfg)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.AgentConfig{}).
		Named("agentconfig").
		Complete(r)
}
