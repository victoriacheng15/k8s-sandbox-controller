package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

// CalculateExpiresAt computes the persistent expiration timestamp for a Sandbox.
// If the Sandbox status already contains ExpiresAt, it is returned without modification.
// Otherwise, it computes expiration based on CreationTimestamp (or fallback baseTime) + Spec.TtlDuration.
func CalculateExpiresAt(sandbox *platformv1alpha1.Sandbox, baseTime time.Time) *metav1.Time {
	if sandbox.Status.ExpiresAt != nil {
		return sandbox.Status.ExpiresAt
	}

	start := baseTime
	if !sandbox.CreationTimestamp.IsZero() {
		start = sandbox.CreationTimestamp.Time
	}

	expiresAt := metav1.NewTime(start.Add(sandbox.Spec.TtlDuration.Duration))
	return &expiresAt
}

// IsExpired reports whether the Sandbox has exceeded its calculated lifetime.
func IsExpired(sandbox *platformv1alpha1.Sandbox, now time.Time) bool {
	if sandbox.Status.ExpiresAt == nil {
		return false
	}
	return !now.Before(sandbox.Status.ExpiresAt.Time)
}

// RemainingTTL returns the duration remaining until Sandbox expiration.
// Returns 0 if already expired or if ExpiresAt is not yet calculated.
func RemainingTTL(sandbox *platformv1alpha1.Sandbox, now time.Time) time.Duration {
	if sandbox.Status.ExpiresAt == nil {
		return 0
	}
	remaining := sandbox.Status.ExpiresAt.Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}
