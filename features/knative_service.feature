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
          "containerImage": "quay.io/redhat-user-workloads/test/component1@sha256:abc123"
        },
        {
          "name": "component-2",
          "containerImage": "quay.io/redhat-user-workloads/test/component2@sha256:def456"
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

  @optional
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
          "containerImage": "quay.io/redhat-user-workloads/test/signed-container@sha256:789xyz"
        }
      ]
    }
    """
    When the snapshot is created
    And the TaskRun completes successfully
    Then a VSA should be created in Rekor
    And the VSA should contain the verification results
    And the VSA should be properly signed

  Scenario: Event processing performance
    Given 10 snapshots are created simultaneously
    When all snapshots are processed
    Then all TaskRuns should be created within 30 seconds
    And all TaskRuns should complete successfully
    And no events should be lost

  Scenario: Service restart resilience
    Given a valid snapshot is created
    And the TaskRun is in progress
    When the knative service is restarted
    Then the TaskRun should continue to completion
    And the service should resume processing new events

