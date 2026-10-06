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

package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/victoriacheng15/k8s-sandbox-controller/internal/metrics"
)

func TestMetricsRegistrationAndCollection(t *testing.T) {
	// 1. Test SandboxesActive GaugeVec
	metrics.SandboxesActive.WithLabelValues("Small", "Ready").Set(3)
	if count := testutil.ToFloat64(metrics.SandboxesActive.WithLabelValues("Small", "Ready")); count != 3 {
		t.Fatalf("expected SandboxesActive to be 3, got %v", count)
	}

	// 2. Test SandboxReconcileTotal CounterVec
	metrics.SandboxReconcileTotal.WithLabelValues("Ready", "success").Inc()
	if count := testutil.ToFloat64(metrics.SandboxReconcileTotal.WithLabelValues("Ready", "success")); count < 1 {
		t.Fatalf("expected SandboxReconcileTotal >= 1, got %v", count)
	}

	// 3. Test SandboxTTLExpiredTotal Counter
	metrics.SandboxTTLExpiredTotal.Inc()
	if count := testutil.ToFloat64(metrics.SandboxTTLExpiredTotal); count < 1 {
		t.Fatalf("expected SandboxTTLExpiredTotal >= 1, got %v", count)
	}

	// 4. Test SandboxDriftHealedTotal CounterVec
	metrics.SandboxDriftHealedTotal.WithLabelValues("ResourceQuota").Inc()
	if count := testutil.ToFloat64(metrics.SandboxDriftHealedTotal.WithLabelValues("ResourceQuota")); count < 1 {
		t.Fatalf("expected SandboxDriftHealedTotal >= 1, got %v", count)
	}

	// 5. Test SandboxReconcileDuration Histogram
	metrics.SandboxReconcileDuration.WithLabelValues("Ready").Observe(0.125)
}
