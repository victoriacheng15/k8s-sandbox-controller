package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

const (
	// SandboxCleanupFinalizer ensures child namespace and resources are fully deleted before Sandbox CR removal.
	SandboxCleanupFinalizer = "finalizers.sandbox.dev/cleanup"
)

// ensureFinalizer registers the cleanup finalizer on the Sandbox if not already present.
// Returns true if the finalizer was added and the object updated.
func (r *SandboxReconciler) ensureFinalizer(ctx context.Context, sandbox *platformv1alpha1.Sandbox) (bool, error) {
	if controllerutil.ContainsFinalizer(sandbox, SandboxCleanupFinalizer) {
		return false, nil
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &platformv1alpha1.Sandbox{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(sandbox), latest); err != nil {
			return err
		}
		if controllerutil.ContainsFinalizer(latest, SandboxCleanupFinalizer) {
			return nil
		}
		controllerutil.AddFinalizer(latest, SandboxCleanupFinalizer)
		return r.Update(ctx, latest)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// handleDeletion manages teardown when Sandbox DeletionTimestamp is set.
// It verifies child namespace deletion completes (apierrors.IsNotFound) before releasing the finalizer.
func (r *SandboxReconciler) handleDeletion(ctx context.Context, sandbox *platformv1alpha1.Sandbox) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(sandbox, SandboxCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Update phase to Terminating if not already set
	if sandbox.Status.Phase != platformv1alpha1.SandboxPhaseTerminating {
		sandbox.Status.Phase = platformv1alpha1.SandboxPhaseTerminating
		if err := r.updateStatus(ctx, sandbox); err != nil {
			log.Error(err, "Failed to update status to Terminating")
		}
	}

	nsName := NamespaceName(sandbox)
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: nsName}, ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Child namespace is deleted, safe to release finalizer
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				latest := &platformv1alpha1.Sandbox{}
				if err := r.Get(ctx, client.ObjectKeyFromObject(sandbox), latest); err != nil {
					return client.IgnoreNotFound(err)
				}
				if !controllerutil.ContainsFinalizer(latest, SandboxCleanupFinalizer) {
					return nil
				}
				controllerutil.RemoveFinalizer(latest, SandboxCleanupFinalizer)
				return r.Update(ctx, latest)
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
			}
			log.Info("Child namespace removed, released finalizer", "sandbox", sandbox.Name, "namespace", nsName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting child namespace %s: %w", nsName, err)
	}

	// Namespace still exists; initiate deletion if not already terminating
	if ns.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("deleting child namespace %s: %w", nsName, err)
		}
		log.Info("Initiated child namespace deletion", "sandbox", sandbox.Name, "namespace", nsName)
	}

	// Poll until child namespace is fully removed (apierrors.IsNotFound)
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}
