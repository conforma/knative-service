# Acceptance tests

Acceptance tests are defined using [Cucumber](https://cucumber.io/) in
[Gherkin](https://cucumber.io/docs/gherkin/) syntax, the steps are implemented
in Go with the help of [Godog](https://github.com/cucumber/godog/).

Feature files written in Gherkin are kept in the [features](../../features/)
directory.

Entry point for the tests is the [acceptance_test.go](acceptance_test.go), which
uses the established Go test to launch Godog.

## Running

To run the acceptance tests either run:

    $ make acceptance

from the root of the repository.

Or move into the acceptance module:

    $ cd acceptance

and run the acceptance_test.go test using `go test`:

    $ go test ./...

The latter is useful for specifying additional arguments. Currently, the
following are supported:

  * `-persist` if specified the test environment will persist after test
    execution, making it easy to recreate test failures and debug the knative
    service or acceptance test code
  * `-restore` run the tests against the persisted environment with `-persist`
  * `-no-colors` disable colored output, useful when running in a terminal that
    doesn't support color escape sequences
  * `-tags=...` comma separated tags to run, e.g. `@optional` - to run only the
    scenarios tagged with `@optional`, or `@optional,~@wip` to run all scenarios that
    are tagged with `@optional` but not with `@wip`

These arguments need to be prefixed with `-args` parameter, for example:

    $ go test ./acceptance -args -persist -tags=@focus

The `-tags` argument is for selecting acceptance test scenarios.

Also notice that there are different ways of specifying the path to the
acceptance tests. `./...` can only be be used if `-args` is NOT used. Use,
`./acceptance` or `github.com/conforma/knative-service/acceptance`
in such cases.

Depending on your setup Testcontainer's ryuk container might need to be run as
privileged container. For that, $HOME/.testcontainers.properties needs to be
created with:

    ryuk.container.privileged=true

## Architecture

The acceptance tests use a combination of:

1. **Kind cluster** - Local Kubernetes cluster for testing
2. **Knative Serving & Eventing** - For the knative service runtime
3. **Tekton Pipelines** - For TaskRun execution
4. **Test containers** - For managing test infrastructure
5. **Godog/Cucumber** - For BDD-style test scenarios

## Test Structure

Tests are organized into step definition packages:

- `knative/` - Steps for knative service deployment and management
- `kubernetes/` - Steps for Kubernetes cluster operations
- `snapshot/` - Steps for creating and managing snapshot resources
- `tekton/` - Steps for TaskRun verification and monitoring
- `testenv/` - Test environment management utilities
- `log/` - Logging utilities for test execution

## Writing Tests

When writing new acceptance tests:

1. **Use descriptive scenario names** that clearly indicate what is being tested
2. **Follow the Given-When-Then pattern** for clear test structure
3. **Keep scenarios focused** on a single aspect of functionality
4. **Use snapshots** for output verification when appropriate
5. **Tag optional scenarios** with `@optional` for features that may be split into separate stories
6. **Include error scenarios** to test failure handling

## Test Environment

The tests create a complete Kubernetes environment including:

- Kind cluster with Knative installed
- Knative service deployed and configured
- Tekton pipelines for enterprise contract verification
- Optional Rekor instance for VSA testing

## Debugging

Use the `-persist` flag to keep the test environment running after test completion:

    $ cd acceptance && go test . -args -persist

This allows you to inspect the cluster state, check logs, and debug issues manually.

## Known Issues

`context deadline exceeded: failed to start container` may occur in some
cases. `sudo systemctl restart docker` usually fixes it.

## Running on MacOS

Running on MacOS has been tested using podman machine. Listed below are the recommended
settings for podman machine:

    $ podman machine init --cpus 4 --memory 8192 --disk-size 100
    $ podman machine start

