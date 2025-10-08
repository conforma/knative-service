Feature: Knative Service Task Triggering
  The knative service should trigger enterprise contract verification tasks when snapshots are created

  Background:
    Given a cluster running
    Given a working namespace
    Given Knative is installed and configured
    Given the knative service is deployed

  Scenario: Snapshot triggers TaskRun creation
    Given a valid snapshot with specification
    """
    {
      "application": "test-app",
      "displayName": "test-snapshot",
      "displayDescription": "Test snapshot for acceptance testing",
      "components": [
        {
          "name": "test-component",
          "containerImage": "quay.io/redhat-user-workloads/rhtap-contract-tenant/golden-container/golden-container@sha256:185f6c39e5544479863024565bb7e63c6f2f0547c3ab4ddf99ac9b5755075cc9"
        }
      ]
    }
    """
    When the snapshot is created in the cluster
    Then a TaskRun should be created
    And the TaskRun should have the correct parameters
    And the TaskRun should reference the enterprise contract bundle
    And the TaskRun should succeed

  Scenario: Multiple components in snapshot
    Given a valid snapshot with multiple components
    """
    {
      "application": "multi-component-app",
      "displayName": "multi-component-snapshot",
      "displayDescription": "Snapshot with multiple components",
      "components": [
        {
          "name": "component-1",
          "containerImage": "quay.io/redhat-user-workloads/test/component1@sha256:1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b"
        },
        {
          "name": "component-2",
          "containerImage": "quay.io/redhat-user-workloads/test/component2@sha256:9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d8c7b6a5f4e3d2c1b0a9f8e"
        }
      ]
    }
    """
    When the snapshot is created in the cluster
    Then a TaskRun should be created for each component
    And all TaskRuns should have the correct parameters
    And all TaskRuns should succeed

  Scenario: Invalid snapshot handling
    Given an invalid snapshot with specification
    """
    {
      "application": "invalid-app",
      "displayName": "invalid-snapshot",
      "components": [
        {
          "name": "invalid-component"
        }
      ]
    }
    """
    When the snapshot is created in the cluster
    Then no TaskRun should be created
    And an error event should be logged

  Scenario: Namespace isolation
    Given a snapshot in namespace "test-namespace-1"
    And a snapshot in namespace "test-namespace-2"
    When both snapshots are created
    Then TaskRuns should be created in their respective namespaces
    And TaskRuns should not interfere with each other

  Scenario: Bundle resolution
    Given a valid snapshot
    And enterprise contract policy configuration
    When the snapshot is created
    Then the TaskRun should resolve the correct bundle
    And the TaskRun should use the latest bundle version
    And the TaskRun should execute successfully

  Scenario: VSA creation in Rekor
    Given Rekor is running and configured
    And a valid snapshot with specification
    """
    {
      "application": "rekor-test-app",
      "displayName": "rekor-test-snapshot",
      "components": [
        {
          "name": "rekor-test-component",
          "containerImage": "quay.io/redhat-user-workloads/test/signed-container@sha256:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
        }
      ]
    }
    """
    When the snapshot is created
    And the TaskRun completes successfully
    Then a VSA should be created in Rekor
    And the VSA should contain the verification results
    And the VSA should be properly signed

