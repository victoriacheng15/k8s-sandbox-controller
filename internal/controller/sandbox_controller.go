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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

// SandboxReconciler reconciles a Sandbox object
type SandboxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.sandbox.dev,resources=sandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.sandbox.dev,resources=sandboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.sandbox.dev,resources=sandboxes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces;resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	sandbox := &platformv1alpha1.Sandbox{}
	if err := r.Get(ctx, req.NamespacedName, sandbox); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch Sandbox")
		return ctrl.Result{}, err
	}

	// Defer deletion handling to finalizer pipeline in Phase 4
	if !sandbox.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	nsName := NamespaceName(sandbox)

	// 1. Reconcile Namespace
	if err := r.reconcileNamespace(ctx, sandbox); err != nil {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeNamespaceReady, metav1.ConditionFalse, "NamespaceReconcileFailed", err.Error())
		_ = r.updateStatus(ctx, sandbox)
		return ctrl.Result{}, fmt.Errorf("reconciling namespace: %w", err)
	}
	r.setCondition(sandbox, platformv1alpha1.ConditionTypeNamespaceReady, metav1.ConditionTrue, "NamespaceProvisioned", "Dedicated namespace exists and is labeled")

	// 2. Reconcile ResourceQuota
	if err := r.reconcileResourceQuota(ctx, sandbox); err != nil {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeResourcesReady, metav1.ConditionFalse, "QuotaReconcileFailed", err.Error())
		_ = r.updateStatus(ctx, sandbox)
		return ctrl.Result{}, fmt.Errorf("reconciling resource quota: %w", err)
	}

	// 3. Reconcile LimitRange
	if err := r.reconcileLimitRange(ctx, sandbox); err != nil {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeResourcesReady, metav1.ConditionFalse, "LimitRangeReconcileFailed", err.Error())
		_ = r.updateStatus(ctx, sandbox)
		return ctrl.Result{}, fmt.Errorf("reconciling limit range: %w", err)
	}
	r.setCondition(sandbox, platformv1alpha1.ConditionTypeResourcesReady, metav1.ConditionTrue, "ResourcesEnforced", "ResourceQuota and LimitRange configured")

	// 4. Reconcile NetworkPolicy
	if err := r.reconcileNetworkPolicy(ctx, sandbox); err != nil {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeNetworkReady, metav1.ConditionFalse, "NetworkPolicyReconcileFailed", err.Error())
		_ = r.updateStatus(ctx, sandbox)
		return ctrl.Result{}, fmt.Errorf("reconciling network policy: %w", err)
	}
	if sandbox.Spec.NetworkIsolation {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeNetworkReady, metav1.ConditionTrue, "NetworkIsolationEnforced", "Default-deny policy applied with DNS allowances")
	} else {
		r.setCondition(sandbox, platformv1alpha1.ConditionTypeNetworkReady, metav1.ConditionTrue, "NetworkIsolationDisabled", "Network isolation is disabled")
	}

	// 5. Aggregate Status & Phase
	r.setCondition(sandbox, platformv1alpha1.ConditionTypeReady, metav1.ConditionTrue, "AllResourcesReady", "Sandbox environment is fully provisioned")
	sandbox.Status.Phase = platformv1alpha1.SandboxPhaseReady
	sandbox.Status.AllocatedNamespace = nsName

	if err := r.updateStatus(ctx, sandbox); err != nil {
		log.Error(err, "Failed to update Sandbox status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SandboxReconciler) reconcileNamespace(ctx context.Context, sandbox *platformv1alpha1.Sandbox) error {
	desired := DesiredNamespace(sandbox)
	existing := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Ensure required managed labels exist
	needsUpdate := false
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range desired.Labels {
		if existing.Labels[k] != v {
			existing.Labels[k] = v
			needsUpdate = true
		}
	}
	if needsUpdate {
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *SandboxReconciler) reconcileResourceQuota(ctx context.Context, sandbox *platformv1alpha1.Sandbox) error {
	desired := DesiredResourceQuota(sandbox)
	existing := &corev1.ResourceQuota{}
	err := r.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec.Hard = desired.Spec.Hard
	return r.Update(ctx, existing)
}

func (r *SandboxReconciler) reconcileLimitRange(ctx context.Context, sandbox *platformv1alpha1.Sandbox) error {
	desired := DesiredLimitRange(sandbox)
	existing := &corev1.LimitRange{}
	err := r.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec.Limits = desired.Spec.Limits
	return r.Update(ctx, existing)
}

func (r *SandboxReconciler) reconcileNetworkPolicy(ctx context.Context, sandbox *platformv1alpha1.Sandbox) error {
	desired := DesiredNetworkPolicy(sandbox)
	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, existing)

	if !sandbox.Spec.NetworkIsolation {
		if err == nil {
			return r.Delete(ctx, existing)
		}
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec = desired.Spec
	return r.Update(ctx, existing)
}

func (r *SandboxReconciler) setCondition(sandbox *platformv1alpha1.Sandbox, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&sandbox.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sandbox.Generation,
		Reason:             reason,
		Message:            message,
	})
}

func (r *SandboxReconciler) updateStatus(ctx context.Context, sandbox *platformv1alpha1.Sandbox) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &platformv1alpha1.Sandbox{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(sandbox), latest); err != nil {
			return err
		}
		latest.Status = sandbox.Status
		return r.Status().Update(ctx, latest)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapChildToSandbox := func(ctx context.Context, obj client.Object) []ctrl.Request {
		labels := obj.GetLabels()
		if name, ok := labels[LabelSandboxName]; ok && name != "" {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Name: name}}}
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Sandbox{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(mapChildToSandbox)).
		Watches(&corev1.ResourceQuota{}, handler.EnqueueRequestsFromMapFunc(mapChildToSandbox)).
		Watches(&corev1.LimitRange{}, handler.EnqueueRequestsFromMapFunc(mapChildToSandbox)).
		Watches(&networkingv1.NetworkPolicy{}, handler.EnqueueRequestsFromMapFunc(mapChildToSandbox)).
		Named("sandbox").
		Complete(r)
}
