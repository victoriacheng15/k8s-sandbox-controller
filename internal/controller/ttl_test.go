package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

var _ = Describe("TTL Logic", func() {
	var (
		sandbox  *platformv1alpha1.Sandbox
		baseTime time.Time
	)

	BeforeEach(func() {
		baseTime = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
		sandbox = &platformv1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-sandbox",
			},
			Spec: platformv1alpha1.SandboxSpec{
				TtlDuration: metav1.Duration{Duration: 2 * time.Hour},
			},
		}
	})

	Context("CalculateExpiresAt", func() {
		It("should return existing ExpiresAt without alteration when already set", func() {
			existing := metav1.NewTime(baseTime.Add(10 * time.Hour))
			sandbox.Status.ExpiresAt = &existing

			expiresAt := CalculateExpiresAt(sandbox, baseTime)
			Expect(expiresAt).To(Equal(&existing))
		})

		It("should use CreationTimestamp when available", func() {
			creation := metav1.NewTime(baseTime)
			sandbox.CreationTimestamp = creation

			expiresAt := CalculateExpiresAt(sandbox, baseTime.Add(1*time.Hour))
			expected := metav1.NewTime(baseTime.Add(2 * time.Hour))
			Expect(expiresAt.Time).To(Equal(expected.Time))
		})

		It("should use fallback baseTime when CreationTimestamp is zero", func() {
			expiresAt := CalculateExpiresAt(sandbox, baseTime)
			expected := metav1.NewTime(baseTime.Add(2 * time.Hour))
			Expect(expiresAt.Time).To(Equal(expected.Time))
		})
	})

	Context("IsExpired", func() {
		It("should return false when ExpiresAt is not set", func() {
			Expect(IsExpired(sandbox, baseTime)).To(BeFalse())
		})

		It("should return false when current time is before expiration", func() {
			expiresAt := metav1.NewTime(baseTime.Add(2 * time.Hour))
			sandbox.Status.ExpiresAt = &expiresAt

			now := baseTime.Add(1 * time.Hour)
			Expect(IsExpired(sandbox, now)).To(BeFalse())
		})

		It("should return true when current time equals expiration", func() {
			expiresAt := metav1.NewTime(baseTime.Add(2 * time.Hour))
			sandbox.Status.ExpiresAt = &expiresAt

			now := baseTime.Add(2 * time.Hour)
			Expect(IsExpired(sandbox, now)).To(BeTrue())
		})

		It("should return true when current time is after expiration", func() {
			expiresAt := metav1.NewTime(baseTime.Add(2 * time.Hour))
			sandbox.Status.ExpiresAt = &expiresAt

			now := baseTime.Add(3 * time.Hour)
			Expect(IsExpired(sandbox, now)).To(BeTrue())
		})
	})

	Context("RemainingTTL", func() {
		It("should return 0 when ExpiresAt is not set", func() {
			Expect(RemainingTTL(sandbox, baseTime)).To(Equal(time.Duration(0)))
		})

		It("should return positive duration when active", func() {
			expiresAt := metav1.NewTime(baseTime.Add(2 * time.Hour))
			sandbox.Status.ExpiresAt = &expiresAt

			now := baseTime.Add(30 * time.Minute)
			Expect(RemainingTTL(sandbox, now)).To(Equal(90 * time.Minute))
		})

		It("should return 0 when already expired", func() {
			expiresAt := metav1.NewTime(baseTime.Add(2 * time.Hour))
			sandbox.Status.ExpiresAt = &expiresAt

			now := baseTime.Add(4 * time.Hour)
			Expect(RemainingTTL(sandbox, now)).To(Equal(time.Duration(0)))
		})
	})
})
