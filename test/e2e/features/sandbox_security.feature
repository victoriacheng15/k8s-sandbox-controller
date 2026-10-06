Feature: Sandbox Security and Admission Controls
  As a platform operator
  I want sandbox namespaces to enforce network isolation and CEL admission policies
  So that workloads cannot access unauthorized networks or violate security baselines

  Scenario: Toggling network isolation on a sandbox
    Given a Sandbox named "e2e-net-box" exists and is "Ready"
    When I disable network isolation on Sandbox "e2e-net-box"
    Then the NetworkPolicy named "sbx-isolation" in namespace "sbx-e2e-net-box" should be removed

  Scenario: Validating admission policy blocks latest image tags
    Given a Sandbox named "e2e-sec-box" exists and is "Ready"
    When I attempt to create a Pod named "bad-tag" in namespace "sbx-e2e-sec-box" with image "nginx:latest"
    Then the Pod creation should be rejected with message "Container image must not use the :latest tag"

  Scenario: Validating admission policy blocks unauthorized registries
    Given a Sandbox named "e2e-sec-box" exists and is "Ready"
    When I attempt to create a Pod named "bad-reg" in namespace "sbx-e2e-sec-box" with image "untrusted.io/app:v1"
    Then the Pod creation should be rejected with message "Container image must be pulled from an approved registry"

  Scenario: Compliant workload is admitted to sandbox
    Given a Sandbox named "e2e-sec-box" exists and is "Ready"
    When I create a compliant Pod named "good-app" in namespace "sbx-e2e-sec-box"
    Then the Pod named "good-app" in namespace "sbx-e2e-sec-box" should exist

  Scenario: Non-sandbox namespaces bypass admission constraints
    Given an unmanaged namespace named "e2e-unmanaged" exists
    When I attempt to create a Pod named "legacy-app" in namespace "e2e-unmanaged" with image "nginx:latest"
    Then the Pod named "legacy-app" in namespace "e2e-unmanaged" should exist
