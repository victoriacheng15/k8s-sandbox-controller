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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const labelPhase = "phase"

var (
	// SandboxesActive tracks the current count of sandboxes by tier and phase.
	SandboxesActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sandbox_active_total",
			Help: "Current count of developer sandboxes partitioned by tier and status phase.",
		},
		[]string{"tier", labelPhase},
	)

	// SandboxReconcileTotal tracks total reconciliation loops partitioned by phase and result status.
	SandboxReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sandbox_reconcile_total",
			Help: "Total count of sandbox reconciliation attempts by phase and result status.",
		},
		[]string{labelPhase, "status"},
	)

	// SandboxReconcileDuration tracks reconciliation latency in seconds.
	SandboxReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sandbox_reconcile_duration_seconds",
			Help:    "Latency of sandbox reconciliation operations in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelPhase},
	)

	// SandboxTTLExpiredTotal tracks the total number of sandboxes expired by the TTL controller.
	SandboxTTLExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sandbox_ttl_expired_total",
			Help: "Total count of ephemeral developer sandboxes torn down due to TTL expiration.",
		},
	)

	// SandboxDriftHealedTotal tracks the total number of child resource drift events corrected.
	SandboxDriftHealedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sandbox_drift_healed_total",
			Help: "Total count of mutated or deleted child resources reconciled back to desired state.",
		},
		[]string{"resource_kind"},
	)
)

func init() {
	// Register metrics with controller-runtime's global registry
	metrics.Registry.MustRegister(
		SandboxesActive,
		SandboxReconcileTotal,
		SandboxReconcileDuration,
		SandboxTTLExpiredTotal,
		SandboxDriftHealedTotal,
	)
}
