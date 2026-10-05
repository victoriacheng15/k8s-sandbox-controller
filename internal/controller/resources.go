package controller

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

const (
	// LabelManaged marks resources managed by the sandbox controller.
	LabelManaged = "sandbox.dev/managed"
	// LabelSandboxName tracks the parent Sandbox resource name.
	LabelSandboxName = "sandbox.dev/sandbox-name"

	// ResourceQuotaName is the standard name for sandbox resource quotas.
	ResourceQuotaName = "sbx-quota"
	// LimitRangeName is the standard name for sandbox limit ranges.
	LimitRangeName = "sbx-limits"
	// NetworkPolicyName is the standard name for sandbox network isolation policies.
	NetworkPolicyName = "sbx-isolation"
)

// NamespaceName returns the deterministic child namespace name for a Sandbox.
func NamespaceName(sandbox *platformv1alpha1.Sandbox) string {
	return "sbx-" + sandbox.Name
}

// CommonLabels returns standardized metadata labels for sandbox child resources.
func CommonLabels(sandbox *platformv1alpha1.Sandbox) map[string]string {
	return map[string]string{
		LabelManaged:                   "true",
		LabelSandboxName:               sandbox.Name,
		"app.kubernetes.io/managed-by": "k8s-sandbox-controller",
	}
}

// DesiredNamespace returns the desired dedicated Namespace for a Sandbox.
func DesiredNamespace(sandbox *platformv1alpha1.Sandbox) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   NamespaceName(sandbox),
			Labels: CommonLabels(sandbox),
		},
	}
}

// DesiredResourceQuota returns the desired ResourceQuota according to the specified tier.
func DesiredResourceQuota(sandbox *platformv1alpha1.Sandbox) *corev1.ResourceQuota {
	hard := quotaForTier(sandbox.Spec.ResourceTier)

	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceQuotaName,
			Namespace: NamespaceName(sandbox),
			Labels:    CommonLabels(sandbox),
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: hard,
		},
	}
}

func quotaForTier(tier platformv1alpha1.ResourceTier) corev1.ResourceList {
	switch tier {
	case platformv1alpha1.ResourceTierMedium:
		return corev1.ResourceList{
			corev1.ResourceRequestsCPU:    resource.MustParse("2"),
			corev1.ResourceRequestsMemory: resource.MustParse("4Gi"),
			corev1.ResourceLimitsCPU:      resource.MustParse("4"),
			corev1.ResourceLimitsMemory:   resource.MustParse("8Gi"),
			corev1.ResourcePods:           resource.MustParse("25"),
		}
	case platformv1alpha1.ResourceTierLarge:
		return corev1.ResourceList{
			corev1.ResourceRequestsCPU:    resource.MustParse("4"),
			corev1.ResourceRequestsMemory: resource.MustParse("8Gi"),
			corev1.ResourceLimitsCPU:      resource.MustParse("8"),
			corev1.ResourceLimitsMemory:   resource.MustParse("16Gi"),
			corev1.ResourcePods:           resource.MustParse("50"),
		}
	case platformv1alpha1.ResourceTierSmall:
		fallthrough
	default:
		return corev1.ResourceList{
			corev1.ResourceRequestsCPU:    resource.MustParse("1"),
			corev1.ResourceRequestsMemory: resource.MustParse("1Gi"),
			corev1.ResourceLimitsCPU:      resource.MustParse("2"),
			corev1.ResourceLimitsMemory:   resource.MustParse("2Gi"),
			corev1.ResourcePods:           resource.MustParse("10"),
		}
	}
}

// DesiredLimitRange returns container-level defaults and constraints.
func DesiredLimitRange(sandbox *platformv1alpha1.Sandbox) *corev1.LimitRange {
	return &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LimitRangeName,
			Namespace: NamespaceName(sandbox),
			Labels:    CommonLabels(sandbox),
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
	}
}

// DesiredNetworkPolicy returns the default-deny NetworkPolicy with intra-namespace and DNS egress allowances.
func DesiredNetworkPolicy(sandbox *platformv1alpha1.Sandbox) *networkingv1.NetworkPolicy {
	dnsPort := intstr.FromInt(53)
	udpProtocol := corev1.ProtocolUDP
	tcpProtocol := corev1.ProtocolTCP

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkPolicyName,
			Namespace: NamespaceName(sandbox),
			Labels:    CommonLabels(sandbox),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow ingress from any pod within the same sandbox namespace
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Allow egress to any pod within the same sandbox namespace
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
				{
					// Allow egress to cluster DNS on port 53 (UDP and TCP)
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &udpProtocol,
							Port:     &dnsPort,
						},
						{
							Protocol: &tcpProtocol,
							Port:     &dnsPort,
						},
					},
				},
			},
		},
	}
}
