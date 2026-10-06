# Architecture Design

## Overview

`k8s-sandbox-controller` provides ephemeral, multi-tenant developer sandboxes within a Kubernetes cluster. It reconciles `Sandbox` custom resources (`platform.sandbox.dev/v1alpha1`) and guarantees hermetic isolation, deterministic resource quotas, automatic drift correction, and self-cleaning lifecycle management.

---

## High-Level Architecture

![k8s-sandbox-controller Architecture](assets/architecture.png)

---

## Architectural Invariants

### 1. Level-Triggered Convergence
The controller follows a strictly level-triggered reconciliation model:
- All child resources (`Namespace`, `ResourceQuota`, `LimitRange`, `NetworkPolicy`) are derived purely from the `Sandbox` spec.
- Reconciling is idempotent: if child resources already exist, specs are checked and repaired if drift is detected.
- If a child resource is deleted or tampered with by external users, secondary watches on `Namespace`, `ResourceQuota`, `LimitRange`, and `NetworkPolicy` immediately enqueue the parent `Sandbox` for healing.

### 2. Isolation & Resource Boundary
Every `Sandbox` instance maps 1:1 to an isolated child namespace named `sbx-<name>`:
- **Labels:** Labeled with `sandbox.dev/managed: "true"` and `sandbox.dev/sandbox-name: "<name>"`.
- **Resource Quotas:** Standardized tier-based bounds (`Small`, `Medium`, `Large`) restricting total compute, memory, and pod counts.
- **Limit Ranges:** Sets default and maximum CPU and memory limits per container to prevent cluster resource starvation.
- **Network Isolation:** Applies a default-deny ingress/egress `NetworkPolicy` allowing only intra-namespace pod traffic and DNS resolution (UDP/TCP port 53). Can be toggled dynamically via `spec.networkIsolation`.

### 3. Admission & In-Tree CEL Security
Rather than introducing external webhook overhead and TLS certificate rot:
- Implements in-tree Kubernetes `ValidatingAdmissionPolicy` and `ValidatingAdmissionPolicyBinding` (GA in Kubernetes 1.30+).
- Intercepts Pod creation requests strictly in namespaces labeled `sandbox.dev/managed: "true"`.
- Denies pods using the `:latest` image tag.
- Enforces container images sourced only from vetted registries (`docker.io`, `quay.io`, `ghcr.io`, `gcr.io`).
- Requires `securityContext.runAsNonRoot: true` and capability drops (`drop: ["ALL"]`).

### 4. Lifecycle & Finalizer Safety
- **TTL Expiration:** `spec.ttlDuration` sets an ephemeral lifetime (e.g., `2h`, `24h`). `status.expiresAt` is calculated and anchored upon first reconciliation.
- **Requeue Scheduling:** The controller schedules precise requeue loops using `RequeueAfter: RemainingTTL(sandbox, now)` without active polling loops.
- **Ordered Teardown:** Upon expiration or deletion:
  1. Deletion of the child namespace `sbx-<name>` is initiated.
  2. The parent `Sandbox` retains `finalizers.sandbox.dev/cleanup`.
  3. Once the child namespace is completely removed from the etcd registry, the finalizer is stripped, allowing Kubernetes garbage collection to complete.

---

## Observability & Metrics

Prometheus telemetry is natively exported on the manager metrics endpoint:

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `sandbox_active_total` | Gauge | `tier`, `phase` | Total active sandboxes by tier and phase |
| `sandbox_reconcile_total` | Counter | `phase`, `status` | Total reconciliation attempts and results |
| `sandbox_reconcile_duration_seconds` | Histogram | `phase` | Latency distribution of reconciliation loops |
| `sandbox_ttl_expired_total` | Counter | - | Count of sandboxes expired and torn down |
| `sandbox_drift_healed_total` | Counter | `resource_kind` | Count of drifted child resources corrected |
