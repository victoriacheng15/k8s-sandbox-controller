//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/victoriacheng15/k8s-sandbox-controller/api/v1alpha1"
)

type scenarioState struct {
	client          client.Client
	ctx             context.Context
	lastErr         error
	lastSandboxName string
	runID           string
}

func (s *scenarioState) resolve(name string) string {
	if s.runID == "" {
		return name
	}
	// Preserve well-known resource names inside the namespace (e.g., sbx-quota, sbx-limits, sbx-isolation)
	if name == "sbx-quota" || name == "sbx-limits" || name == "sbx-isolation" {
		return name
	}
	if strings.HasPrefix(name, "sbx-") {
		base := strings.TrimPrefix(name, "sbx-")
		return "sbx-" + base + "-" + s.runID
	}
	return name + "-" + s.runID
}

func getClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = platformv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
	return client.New(cfg, client.Options{Scheme: s})
}

func eventually(timeout, interval time.Duration, condition func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := condition()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for condition", timeout)
		}
		time.Sleep(interval)
	}
}

// InitializeScenario registers all Gherkin step bindings with Godog.
func InitializeScenario(ctx *godog.ScenarioContext) {
	s := &scenarioState{
		ctx: context.Background(),
	}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		cl, err := getClient()
		if err != nil {
			return ctx, err
		}
		s.client = cl
		s.lastErr = nil
		s.runID = fmt.Sprintf("%x", time.Now().UnixNano()%0xfffff)
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if s.lastSandboxName != "" && s.client != nil {
			var sbx platformv1alpha1.Sandbox
			sbx.Name = s.lastSandboxName
			_ = client.IgnoreNotFound(s.client.Delete(s.ctx, &sbx))
		}
		return ctx, nil
	})

	ctx.Step(`^a Kubernetes cluster is running$`, s.aKubernetesClusterIsRunning)
	ctx.Step(`^I create a Sandbox named "([^"]*)" with tier "([^"]*)" and network isolation enabled$`, s.iCreateASandboxNamedWithTierAndNetworkIsolationEnabled)
	ctx.Step(`^the Sandbox phase should transition to "([^"]*)"$`, s.theSandboxPhaseShouldTransitionTo)
	ctx.Step(`^the child namespace "([^"]*)" should exist with label "([^"]*)" set to "([^"]*)"$`, s.theChildNamespaceShouldExistWithLabelSetTo)
	ctx.Step(`^a ResourceQuota named "([^"]*)" should exist in namespace "([^"]*)"$`, s.aResourceQuotaNamedShouldExistInNamespace)
	ctx.Step(`^a LimitRange named "([^"]*)" should exist in namespace "([^"]*)"$`, s.aLimitRangeNamedShouldExistInNamespace)
	ctx.Step(`^a NetworkPolicy named "([^"]*)" should exist in namespace "([^"]*)"$`, s.aNetworkPolicyNamedShouldExistInNamespace)
	ctx.Step(`^a Sandbox named "([^"]*)" exists and is "([^"]*)"$`, s.aSandboxNamedExistsAndIs)
	ctx.Step(`^the ResourceQuota named "([^"]*)" in namespace "([^"]*)" is deleted$`, s.theResourceQuotaNamedInNamespaceIsDeleted)
	ctx.Step(`^the controller should heal the drift and recreate "([^"]*)" in namespace "([^"]*)"$`, s.theControllerShouldHealTheDriftAndRecreateInNamespace)
	ctx.Step(`^I create an ephemeral Sandbox named "([^"]*)" with TTL duration "([^"]*)"$`, s.iCreateAnEphemeralSandboxNamedWithTTLDuration)
	ctx.Step(`^the Sandbox phase should eventually transition to "([^"]*)"$`, s.theSandboxPhaseShouldEventuallyTransitionTo)
	ctx.Step(`^the child namespace "([^"]*)" should eventually be terminated$`, s.theChildNamespaceShouldEventuallyBeTerminated)
	ctx.Step(`^I delete the Sandbox named "([^"]*)"$`, s.iDeleteTheSandboxNamed)
	ctx.Step(`^the Sandbox named "([^"]*)" should be completely removed$`, s.theSandboxNamedShouldBeCompletelyRemoved)
	ctx.Step(`^I disable network isolation on Sandbox "([^"]*)"$`, s.iDisableNetworkIsolationOnSandbox)
	ctx.Step(`^the NetworkPolicy named "([^"]*)" in namespace "([^"]*)" should be removed$`, s.theNetworkPolicyNamedInNamespaceShouldBeRemoved)
	ctx.Step(`^I attempt to create a Pod named "([^"]*)" in namespace "([^"]*)" with image "([^"]*)"$`, s.iAttemptToCreateAPodNamedInNamespaceWithImage)
	ctx.Step(`^the Pod creation should be rejected with message "([^"]*)"$`, s.thePodCreationShouldBeRejectedWithMessage)
	ctx.Step(`^I create a compliant Pod named "([^"]*)" in namespace "([^"]*)"$`, s.iCreateACompliantPodNamedInNamespace)
	ctx.Step(`^the Pod named "([^"]*)" in namespace "([^"]*)" should exist$`, s.thePodNamedInNamespaceShouldExist)
	ctx.Step(`^an unmanaged namespace named "([^"]*)" exists$`, s.anUnmanagedNamespaceNamedExists)
}

func (s *scenarioState) aKubernetesClusterIsRunning() error {
	if s.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}
	var nsList corev1.NamespaceList
	return s.client.List(s.ctx, &nsList, client.Limit(1))
}

func (s *scenarioState) iCreateASandboxNamedWithTierAndNetworkIsolationEnabled(name, tier string) error {
	name = s.resolve(name)
	s.lastSandboxName = name
	var resTier platformv1alpha1.ResourceTier
	switch strings.ToLower(tier) {
	case "medium":
		resTier = platformv1alpha1.ResourceTierMedium
	case "large":
		resTier = platformv1alpha1.ResourceTierLarge
	default:
		resTier = platformv1alpha1.ResourceTierSmall
	}
	sbx := &platformv1alpha1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: platformv1alpha1.SandboxSpec{
			TtlDuration:      metav1.Duration{Duration: 2 * time.Hour},
			ResourceTier:     resTier,
			NetworkIsolation: true,
		},
	}
	err := s.client.Create(s.ctx, sbx)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (s *scenarioState) theSandboxPhaseShouldTransitionTo(expectedPhase string) error {
	return eventually(45*time.Second, time.Second, func() (bool, error) {
		var sbx platformv1alpha1.Sandbox
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: s.lastSandboxName}, &sbx); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return string(sbx.Status.Phase) == expectedPhase, nil
	})
}

func (s *scenarioState) theChildNamespaceShouldExistWithLabelSetTo(nsName, labelKey, labelVal string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var ns corev1.Namespace
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: nsName}, &ns); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return ns.Labels[labelKey] == labelVal, nil
	})
}

func (s *scenarioState) aResourceQuotaNamedShouldExistInNamespace(quotaName, nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var rq corev1.ResourceQuota
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: quotaName, Namespace: nsName}, &rq); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func (s *scenarioState) aLimitRangeNamedShouldExistInNamespace(lrName, nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var lr corev1.LimitRange
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: lrName, Namespace: nsName}, &lr); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func (s *scenarioState) aNetworkPolicyNamedShouldExistInNamespace(npName, nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var np networkingv1.NetworkPolicy
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: npName, Namespace: nsName}, &np); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func (s *scenarioState) aSandboxNamedExistsAndIs(name, phase string) error {
	name = s.resolve(name)
	s.lastSandboxName = name
	var sbx platformv1alpha1.Sandbox
	err := s.client.Get(s.ctx, types.NamespacedName{Name: name}, &sbx)
	if apierrors.IsNotFound(err) {
		sbx = platformv1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: platformv1alpha1.SandboxSpec{
				TtlDuration:      metav1.Duration{Duration: 2 * time.Hour},
				ResourceTier:     platformv1alpha1.ResourceTierSmall,
				NetworkIsolation: true,
			},
		}
		if createErr := s.client.Create(s.ctx, &sbx); createErr != nil {
			return createErr
		}
	} else if err != nil {
		return err
	}
	return s.theSandboxPhaseShouldTransitionTo(phase)
}

func (s *scenarioState) theResourceQuotaNamedInNamespaceIsDeleted(quotaName, nsName string) error {
	var rq corev1.ResourceQuota
	rq.Name = quotaName
	rq.Namespace = nsName
	return client.IgnoreNotFound(s.client.Delete(s.ctx, &rq))
}

func (s *scenarioState) theControllerShouldHealTheDriftAndRecreateInNamespace(quotaName, nsName string) error {
	return s.aResourceQuotaNamedShouldExistInNamespace(quotaName, nsName)
}

func (s *scenarioState) iCreateAnEphemeralSandboxNamedWithTTLDuration(name, ttlStr string) error {
	name = s.resolve(name)
	d, err := time.ParseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid ttl duration %q: %w", ttlStr, err)
	}
	s.lastSandboxName = name
	sbx := &platformv1alpha1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: platformv1alpha1.SandboxSpec{
			TtlDuration:      metav1.Duration{Duration: d},
			ResourceTier:     platformv1alpha1.ResourceTierSmall,
			NetworkIsolation: true,
		},
	}
	return s.client.Create(s.ctx, sbx)
}

func (s *scenarioState) theSandboxPhaseShouldEventuallyTransitionTo(expectedPhase string) error {
	return s.theSandboxPhaseShouldTransitionTo(expectedPhase)
}

func (s *scenarioState) theChildNamespaceShouldEventuallyBeTerminated(nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(45*time.Second, time.Second, func() (bool, error) {
		var ns corev1.Namespace
		err := s.client.Get(s.ctx, types.NamespacedName{Name: nsName}, &ns)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return ns.DeletionTimestamp != nil, nil
	})
}

func (s *scenarioState) iDeleteTheSandboxNamed(name string) error {
	name = s.resolve(name)
	var sbx platformv1alpha1.Sandbox
	sbx.Name = name
	return client.IgnoreNotFound(s.client.Delete(s.ctx, &sbx))
}

func (s *scenarioState) theSandboxNamedShouldBeCompletelyRemoved(name string) error {
	name = s.resolve(name)
	return eventually(45*time.Second, time.Second, func() (bool, error) {
		var sbx platformv1alpha1.Sandbox
		err := s.client.Get(s.ctx, types.NamespacedName{Name: name}, &sbx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (s *scenarioState) iDisableNetworkIsolationOnSandbox(name string) error {
	name = s.resolve(name)
	var sbx platformv1alpha1.Sandbox
	if err := s.client.Get(s.ctx, types.NamespacedName{Name: name}, &sbx); err != nil {
		return err
	}
	sbx.Spec.NetworkIsolation = false
	return s.client.Update(s.ctx, &sbx)
}

func (s *scenarioState) theNetworkPolicyNamedInNamespaceShouldBeRemoved(npName, nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var np networkingv1.NetworkPolicy
		err := s.client.Get(s.ctx, types.NamespacedName{Name: npName, Namespace: nsName}, &np)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (s *scenarioState) iAttemptToCreateAPodNamedInNamespaceWithImage(podName, nsName, image string) error {
	nsName = s.resolve(nsName)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: nsName,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: image,
				},
			},
		},
	}
	s.lastErr = s.client.Create(s.ctx, pod)
	return nil
}

func (s *scenarioState) thePodCreationShouldBeRejectedWithMessage(msg string) error {
	if s.lastErr == nil {
		return fmt.Errorf("expected pod creation to fail, but it succeeded")
	}
	if !strings.Contains(s.lastErr.Error(), msg) {
		return fmt.Errorf("expected error containing %q, got: %v", msg, s.lastErr)
	}
	return nil
}

func (s *scenarioState) iCreateACompliantPodNamedInNamespace(podName, nsName string) error {
	nsName = s.resolve(nsName)
	nonRoot := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: nsName,
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &nonRoot,
			},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:1.27.1-alpine",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
		},
	}
	s.lastErr = s.client.Create(s.ctx, pod)
	return s.lastErr
}

func (s *scenarioState) thePodNamedInNamespaceShouldExist(podName, nsName string) error {
	nsName = s.resolve(nsName)
	return eventually(30*time.Second, time.Second, func() (bool, error) {
		var pod corev1.Pod
		if err := s.client.Get(s.ctx, types.NamespacedName{Name: podName, Namespace: nsName}, &pod); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func (s *scenarioState) anUnmanagedNamespaceNamedExists(nsName string) error {
	nsName = s.resolve(nsName)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	err := s.client.Create(s.ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
