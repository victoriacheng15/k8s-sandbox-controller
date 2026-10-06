Feature: Sandbox Lifecycle Management
  As a platform operator
  I want developer sandboxes to provision child namespaces, quotas, and limits deterministically
  And clean up resources when deleted or expired

  Scenario: Provisioning child resources for a new sandbox
    Given a Kubernetes cluster is running
    When I create a Sandbox named "e2e-dev-box" with tier "small" and network isolation enabled
    Then the Sandbox phase should transition to "Ready"
    And the child namespace "sbx-e2e-dev-box" should exist with label "sandbox.dev/managed" set to "true"
    And a ResourceQuota named "sbx-quota" should exist in namespace "sbx-e2e-dev-box"
    And a LimitRange named "sbx-limits" should exist in namespace "sbx-e2e-dev-box"
    And a NetworkPolicy named "sbx-isolation" should exist in namespace "sbx-e2e-dev-box"

  Scenario: Auto-healing child resource drift
    Given a Sandbox named "e2e-dev-box" exists and is "Ready"
    When the ResourceQuota named "sbx-quota" in namespace "sbx-e2e-dev-box" is deleted
    Then the controller should heal the drift and recreate "sbx-quota" in namespace "sbx-e2e-dev-box"

  Scenario: Expiring an ephemeral sandbox after TTL duration
    Given a Kubernetes cluster is running
    When I create an ephemeral Sandbox named "e2e-ttl-box" with TTL duration "5s"
    Then the Sandbox phase should eventually transition to "Expired"
    And the child namespace "sbx-e2e-ttl-box" should eventually be terminated

  Scenario: Deleting a sandbox triggers finalizer cleanup
    Given a Sandbox named "e2e-dev-box" exists and is "Ready"
    When I delete the Sandbox named "e2e-dev-box"
    Then the child namespace "sbx-e2e-dev-box" should eventually be terminated
    And the Sandbox named "e2e-dev-box" should be completely removed
