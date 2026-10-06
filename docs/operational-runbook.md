# Operational Runbook

## Overview

This runbook documents operational procedures, triage workflows, telemetry checks, and recovery recipes for maintaining `k8s-sandbox-controller`.

---

## 1. Starting the Controller & Health Diagnostics

### Starting the Operator
Before performing health checks or applying sandboxes, initialize prerequisites and start the controller:

```bash
# 1. Install CRDs and CEL admission policies
make install
kubectl apply -k config/admission

# 2. Run the controller locally
make run

# 3. In another terminal, apply a sample Sandbox
kubectl apply -f config/samples/platform_v1alpha1_sandbox.yaml
```

### Health Checks & Probes
Once the controller is running, verify the liveness and readiness probes in a separate terminal:

- **Liveness probe:** `http://localhost:8081/healthz`
- **Readiness probe:** `http://localhost:8081/readyz`

```bash
curl -s http://localhost:8081/healthz
curl -s http://localhost:8081/readyz
```

Both endpoints return `ok` when the manager is operational.

### Scraping Prometheus Metrics
When the controller is running, verify exported Prometheus metrics in a separate terminal:

```bash
curl -s http://localhost:8080/metrics | grep sandbox_
```

Key alerts and indicators to monitor:
- `sandbox_reconcile_total{status="error"}` rate spiking above 0.
- `sandbox_reconcile_duration_seconds` p99 exceeding 500ms.
- `sandbox_ttl_expired_total` tracking teardowns.

---

## 2. Common Operational Failure Modes

### Failure Mode 1: Stuck Sandbox Deletion (Finalizer Deadlock)
**Symptoms:** A Sandbox CR remains in `Terminating` phase indefinitely.
**Cause:** The child namespace `sbx-<name>` has pending background resources (such as terminating pods or lingering finalizers) blocking namespace deletion.
**Remediation:**
1. List active sandboxes to identify the stuck instance name and child namespace:
   ```bash
   kubectl get sandboxes
   ```
2. Inspect the child namespace and any lingering workloads or managed policies:
   ```bash
   kubectl get ns sbx-<name>
   kubectl get all,resourcequota,limitrange,networkpolicy -n sbx-<name>
   ```
3. If resources are stuck terminating in the namespace, resolve the underlying resource blockage.
4. In emergency recovery scenarios where the child namespace is removed but the finalizer remains:
   ```bash
   kubectl patch sandbox <name> -p '{"metadata":{"finalizers":[]}}' --type=merge
   ```

### Failure Mode 2: Resource Drift or Manual Mutated Quotas
**Symptoms:** Quota exhaustion in a sandbox despite spec tiers.
**Cause:** An operator or script manually modified `sbx-quota` or `sbx-limits`.
**Remediation:**
The controller automatically corrects drift via secondary watches. If an immediate sync is desired:
```bash
kubectl annotate sandbox <name> sandbox.dev/sync="$(date +%s)" --overwrite
```

### Failure Mode 3: Pod Rejection via CEL ValidatingAdmissionPolicy
**Symptoms:** Developer pods in `sbx-<name>` fail admission with `ValidatingAdmissionPolicy 'sandbox-workload-security' denied request`.
**Cause:** The workload violates sandbox security baselines (uses `:latest` tag, unapproved image registry, or runs as root).
**Remediation:**
Inspect policy violations on the workload:
1. Ensure image uses an immutable tag (e.g., `nginx:1.27-alpine` instead of `nginx:latest`).
2. Ensure image registry is one of `docker.io`, `quay.io`, `ghcr.io`, or `gcr.io`.
3. Set `securityContext.runAsNonRoot: true` in the pod spec.

---

## 3. Teardown & Reset Procedures

### Teardown Active Sandboxes
```bash
kubectl delete sandbox --all
```

### Remove CRDs and Policies
```bash
make uninstall
kubectl delete -k config/admission
```
