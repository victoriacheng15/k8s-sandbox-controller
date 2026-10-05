package controller

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

var _ = Describe("Resource Generators", func() {
	var sandbox *platformv1alpha1.Sandbox

	BeforeEach(func() {
		sandbox = &platformv1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "developer-app",
			},
			Spec: platformv1alpha1.SandboxSpec{
				ResourceTier:     platformv1alpha1.ResourceTierSmall,
				NetworkIsolation: true,
			},
		}
	})

	Context("NamespaceName", func() {
		It("should prefix the sandbox name with sbx-", func() {
			Expect(NamespaceName(sandbox)).To(Equal("sbx-developer-app"))
		})
	})

	Context("CommonLabels", func() {
		It("should contain the required managed and identity labels", func() {
			labels := CommonLabels(sandbox)
			Expect(labels).To(HaveKeyWithValue(LabelManaged, "true"))
			Expect(labels).To(HaveKeyWithValue(LabelSandboxName, "developer-app"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "k8s-sandbox-controller"))
		})
	})

	Context("DesiredNamespace", func() {
		It("should set the deterministic namespace name and labels", func() {
			ns := DesiredNamespace(sandbox)
			Expect(ns.Name).To(Equal("sbx-developer-app"))
			Expect(ns.Labels).To(HaveKeyWithValue(LabelManaged, "true"))
			Expect(ns.Labels).To(HaveKeyWithValue(LabelSandboxName, "developer-app"))
		})
	})

	Context("DesiredResourceQuota", func() {
		It("should generate small tier quotas by default", func() {
			sandbox.Spec.ResourceTier = platformv1alpha1.ResourceTierSmall
			quota := DesiredResourceQuota(sandbox)

			Expect(quota.Name).To(Equal(ResourceQuotaName))
			Expect(quota.Namespace).To(Equal("sbx-developer-app"))
			Expect(quota.Spec.Hard[corev1.ResourceRequestsCPU]).To(Equal(resource.MustParse("1")))
			Expect(quota.Spec.Hard[corev1.ResourceRequestsMemory]).To(Equal(resource.MustParse("1Gi")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsCPU]).To(Equal(resource.MustParse("2")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsMemory]).To(Equal(resource.MustParse("2Gi")))
			Expect(quota.Spec.Hard[corev1.ResourcePods]).To(Equal(resource.MustParse("10")))
		})

		It("should generate medium tier quotas", func() {
			sandbox.Spec.ResourceTier = platformv1alpha1.ResourceTierMedium
			quota := DesiredResourceQuota(sandbox)

			Expect(quota.Spec.Hard[corev1.ResourceRequestsCPU]).To(Equal(resource.MustParse("2")))
			Expect(quota.Spec.Hard[corev1.ResourceRequestsMemory]).To(Equal(resource.MustParse("4Gi")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsCPU]).To(Equal(resource.MustParse("4")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsMemory]).To(Equal(resource.MustParse("8Gi")))
			Expect(quota.Spec.Hard[corev1.ResourcePods]).To(Equal(resource.MustParse("25")))
		})

		It("should generate large tier quotas", func() {
			sandbox.Spec.ResourceTier = platformv1alpha1.ResourceTierLarge
			quota := DesiredResourceQuota(sandbox)

			Expect(quota.Spec.Hard[corev1.ResourceRequestsCPU]).To(Equal(resource.MustParse("4")))
			Expect(quota.Spec.Hard[corev1.ResourceRequestsMemory]).To(Equal(resource.MustParse("8Gi")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsCPU]).To(Equal(resource.MustParse("8")))
			Expect(quota.Spec.Hard[corev1.ResourceLimitsMemory]).To(Equal(resource.MustParse("16Gi")))
			Expect(quota.Spec.Hard[corev1.ResourcePods]).To(Equal(resource.MustParse("50")))
		})
	})

	Context("DesiredLimitRange", func() {
		It("should set container default requests and limits", func() {
			lr := DesiredLimitRange(sandbox)
			Expect(lr.Name).To(Equal(LimitRangeName))
			Expect(lr.Namespace).To(Equal("sbx-developer-app"))
			Expect(lr.Spec.Limits).To(HaveLen(1))

			item := lr.Spec.Limits[0]
			Expect(item.Type).To(Equal(corev1.LimitTypeContainer))
			Expect(item.Default[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
			Expect(item.Default[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
			Expect(item.DefaultRequest[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(item.DefaultRequest[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
		})
	})

	Context("DesiredNetworkPolicy", func() {
		It("should configure default-deny with intra-namespace and DNS egress allowances", func() {
			np := DesiredNetworkPolicy(sandbox)
			Expect(np.Name).To(Equal(NetworkPolicyName))
			Expect(np.Namespace).To(Equal("sbx-developer-app"))
			Expect(np.Spec.PolicyTypes).To(ConsistOf(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))

			// Ingress intra-namespace allowance
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From[0].PodSelector).NotTo(BeNil())

			// Egress intra-namespace and DNS allowances
			Expect(np.Spec.Egress).To(HaveLen(2))
			Expect(np.Spec.Egress[0].To).To(HaveLen(1))
			Expect(np.Spec.Egress[0].To[0].PodSelector).NotTo(BeNil())

			Expect(np.Spec.Egress[1].Ports).To(HaveLen(2))
			Expect(np.Spec.Egress[1].Ports[0].Port.IntValue()).To(Equal(53))
			Expect(*np.Spec.Egress[1].Ports[0].Protocol).To(Equal(corev1.ProtocolUDP))
			Expect(np.Spec.Egress[1].Ports[1].Port.IntValue()).To(Equal(53))
			Expect(*np.Spec.Egress[1].Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))
		})
	})
})
