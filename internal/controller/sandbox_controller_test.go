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
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

var _ = Describe("Sandbox Controller", func() {
	var (
		ctx        context.Context
		reconciler *SandboxReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &SandboxReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
	})

	Context("Reconciliation and Resource Convergence", func() {
		It("should provision all child resources, configure status, and schedule TTL requeue", func() {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sbx-converge",
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 2 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())

			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}

			// First reconcile pass: registers finalizer
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			latest := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(latest, SandboxCleanupFinalizer)).To(BeTrue())

			// Second reconcile pass: initializes status and converges child resources
			result, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", 2*time.Hour))

			// Verify child Namespace exists with correct labels
			nsName := NamespaceName(sandbox)
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)).To(Succeed())
			Expect(ns.Labels).To(HaveKeyWithValue(LabelManaged, "true"))
			Expect(ns.Labels).To(HaveKeyWithValue(LabelSandboxName, sandbox.Name))

			// Verify child ResourceQuota
			quota := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: ResourceQuotaName}, quota)).To(Succeed())
			Expect(quota.Spec.Hard[corev1.ResourceRequestsCPU]).To(Equal(quotaForTier(platformv1alpha1.ResourceTierSmall)[corev1.ResourceRequestsCPU]))

			// Verify child LimitRange
			lr := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: LimitRangeName}, lr)).To(Succeed())
			Expect(lr.Spec.Limits).To(HaveLen(1))

			// Verify child NetworkPolicy
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: NetworkPolicyName}, np)).To(Succeed())

			// Verify Sandbox Status and Conditions
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			Expect(latest.Status.Phase).To(Equal(platformv1alpha1.SandboxPhaseReady))
			Expect(latest.Status.AllocatedNamespace).To(Equal(nsName))
			Expect(latest.Status.ExpiresAt).NotTo(BeNil())

			Expect(meta.IsStatusConditionTrue(latest.Status.Conditions, platformv1alpha1.ConditionTypeNamespaceReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(latest.Status.Conditions, platformv1alpha1.ConditionTypeResourcesReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(latest.Status.Conditions, platformv1alpha1.ConditionTypeNetworkReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(latest.Status.Conditions, platformv1alpha1.ConditionTypeReady)).To(BeTrue())
		})

		It("should converge idempotently without duplicate changes on repeated runs", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-sbx-converge"}}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})
	})

	Context("Drift Detection and Auto-Healing", func() {
		setupReadySandbox := func(name string) (*platformv1alpha1.Sandbox, ctrl.Request, string) {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 1 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}
			nsName := NamespaceName(sandbox)

			// Converge to Ready state
			_, _ = reconciler.Reconcile(ctx, req)
			_, _ = reconciler.Reconcile(ctx, req)

			return sandbox, req, nsName
		}

		It("should detect out-of-band deletion of ResourceQuota and recreate it", func() {
			_, req, nsName := setupReadySandbox("test-sbx-drift-quota")

			quota := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: ResourceQuotaName}, quota)).To(Succeed())
			Expect(k8sClient.Delete(ctx, quota)).To(Succeed())

			// Reconcile heals deletion
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			recreated := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: ResourceQuotaName}, recreated)).To(Succeed())
		})

		It("should detect out-of-band mutation of LimitRange and restore desired specs", func() {
			_, req, nsName := setupReadySandbox("test-sbx-drift-limits")

			lr := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: LimitRangeName}, lr)).To(Succeed())

			// Mutate limits
			lr.Spec.Limits = []corev1.LimitRangeItem{}
			Expect(k8sClient.Update(ctx, lr)).To(Succeed())

			// Reconcile heals drift
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			restored := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: LimitRangeName}, restored)).To(Succeed())
			Expect(restored.Spec.Limits).To(HaveLen(1))
		})

		It("should detect out-of-band namespace label modification and restore labels", func() {
			_, req, nsName := setupReadySandbox("test-sbx-drift-labels")

			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)).To(Succeed())
			delete(ns.Labels, LabelManaged)
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			// Reconcile restores label
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			restored := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, restored)).To(Succeed())
			Expect(restored.Labels).To(HaveKeyWithValue(LabelManaged, "true"))
		})
	})

	Context("NetworkIsolation Toggle", func() {
		It("should delete NetworkPolicy when NetworkIsolation is disabled", func() {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sbx-netpol-toggle",
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 1 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}
			nsName := NamespaceName(sandbox)

			// Converge to Ready
			_, _ = reconciler.Reconcile(ctx, req)
			_, _ = reconciler.Reconcile(ctx, req)

			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: NetworkPolicyName}, np)).To(Succeed())

			// Toggle NetworkIsolation to false
			latest := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			latest.Spec.NetworkIsolation = false
			Expect(k8sClient.Update(ctx, latest)).To(Succeed())

			// Reconcile deletes NetworkPolicy
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: nsName, Name: NetworkPolicyName}, np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			cond := meta.FindStatusCondition(latest.Status.Conditions, platformv1alpha1.ConditionTypeNetworkReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("NetworkIsolationDisabled"))
		})
	})

	Context("TTL Expiration and Teardown", func() {
		It("should transition to Expired phase and delete child namespace while preserving Sandbox CR", func() {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sbx-expire",
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 1 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}
			nsName := NamespaceName(sandbox)

			// Converge to Ready
			_, _ = reconciler.Reconcile(ctx, req)
			_, _ = reconciler.Reconcile(ctx, req)

			// Force ExpiresAt into the past
			latest := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			past := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			latest.Status.ExpiresAt = &past
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())

			// Reconcile handles expiration
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Assert Sandbox CR still exists with Expired status
			expiredSandbox := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, expiredSandbox)).To(Succeed())
			Expect(expiredSandbox.Status.Phase).To(Equal(platformv1alpha1.SandboxPhaseExpired))
			Expect(meta.IsStatusConditionTrue(expiredSandbox.Status.Conditions, platformv1alpha1.ConditionTypeExpired)).To(BeTrue())
			Expect(meta.IsStatusConditionFalse(expiredSandbox.Status.Conditions, platformv1alpha1.ConditionTypeReady)).To(BeTrue())

			// Assert child namespace was marked for deletion
			ns := &corev1.Namespace{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)
			if err == nil {
				Expect(ns.DeletionTimestamp.IsZero()).To(BeFalse())
			} else {
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})
	})

	Context("Controller Restart State Preservation", func() {
		It("should preserve persistent ExpiresAt across new reconciler instances", func() {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sbx-restart",
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 3 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}

			// First reconciler pass initializes
			_, _ = reconciler.Reconcile(ctx, req)
			_, _ = reconciler.Reconcile(ctx, req)

			first := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, first)).To(Succeed())
			initialExpiresAt := first.Status.ExpiresAt
			Expect(initialExpiresAt).NotTo(BeNil())

			// Simulate controller restart by creating a new reconciler instance
			reconcilerRestarted := &SandboxReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			result, err := reconcilerRestarted.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			second := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, second)).To(Succeed())
			Expect(second.Status.ExpiresAt.Time).To(Equal(initialExpiresAt.Time))
		})
	})

	Context("Deletion and Finalizer Lifecycle", func() {
		It("should initiate child namespace deletion and poll until namespace is removed", func() {
			sandbox := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sbx-deletion",
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration:      metav1.Duration{Duration: 1 * time.Hour},
					ResourceTier:     platformv1alpha1.ResourceTierSmall,
					NetworkIsolation: true,
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandbox.Name}}
			nsName := NamespaceName(sandbox)

			// Converge to Ready
			_, _ = reconciler.Reconcile(ctx, req)
			_, _ = reconciler.Reconcile(ctx, req)

			// Delete Sandbox CR
			latest := &platformv1alpha1.Sandbox{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			Expect(k8sClient.Delete(ctx, latest)).To(Succeed())

			// Reconcile enters deletion flow
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// Awaits child namespace deletion with requeue
			Expect(result.RequeueAfter).To(Equal(2 * time.Second))

			// Verify phase is Terminating
			Expect(k8sClient.Get(ctx, req.NamespacedName, latest)).To(Succeed())
			Expect(latest.Status.Phase).To(Equal(platformv1alpha1.SandboxPhaseTerminating))

			// Verify child namespace has deletion initiated
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)).To(Succeed())
			Expect(ns.DeletionTimestamp.IsZero()).To(BeFalse())

			// Simulate envtest namespace finalizer removal (absence of kube-controller-manager)
			// When namespace is gone (or simulated as not found):
			sandboxNoNs := &platformv1alpha1.Sandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sbx-gone",
					Finalizers: []string{SandboxCleanupFinalizer},
				},
				Spec: platformv1alpha1.SandboxSpec{
					TtlDuration: metav1.Duration{Duration: 1 * time.Hour},
				},
			}
			Expect(k8sClient.Create(ctx, sandboxNoNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sandboxNoNs)).To(Succeed())

			reqGone := ctrl.Request{NamespacedName: types.NamespacedName{Name: sandboxNoNs.Name}}
			resGone, errGone := reconciler.Reconcile(ctx, reqGone)
			Expect(errGone).NotTo(HaveOccurred())
			Expect(resGone).To(Equal(ctrl.Result{}))

			// Finalizer is removed and CR is removed
			goneCR := &platformv1alpha1.Sandbox{}
			errGone = k8sClient.Get(ctx, reqGone.NamespacedName, goneCR)
			Expect(apierrors.IsNotFound(errGone)).To(BeTrue())
		})
	})
})
