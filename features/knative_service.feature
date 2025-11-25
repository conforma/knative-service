Feature: Knative Service Job Triggering
  The knative service should trigger enterprise contract verification jobs when snapshots are created

  Background:
    Given a cluster running
    Given a working namespace
    Given Knative is installed and configured
    Given the knative service is deployed

  Scenario: Snapshot triggers Job creation
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
    Then a Job should be created
    And the Job should have the correct parameters
    And the Job parameters should have correct values

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
    Then a Job should be created for each component
    And all Jobs should have the correct parameters
    And all Jobs parameters should have correct values

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
    And the Job completes successfully
    Then a VSA should be created in Rekor
    And the VSA should contain the verification results
    And the VSA should be properly signed

