# OTAF rApp SDK

A Go SDK for building rApps for the O-RAN Non-RT RIC. It implements the
platform-facing concerns — lifecycle, configuration, ingest, R1, A1, O1 and
packaging — so that an rApp contains only its own logic.

Apache 2.0. Requires Go 1.23 or newer and six direct dependencies.

## Installation

```bash
go install github.com/coranlabs/OTAF/otaf-rapp-sdk/cmd/rappctl@latest
```

## Getting started

```bash
rappctl new my-rapp
cd my-rapp
go mod tidy
rappctl package
```

The `new` command generates a complete rApp: a Go module and entry point, a
Dockerfile, a Helm chart comprising a ConfigMap, Secret, Service, Deployment and
an optional ServiceMonitor, the package descriptor, and the ACM, DME and SME
registration files. The generated rApp compiles and deploys without
modification.

The `package` command builds the CSAR and validates it. The resulting package is
uploaded to rApp Manager, primed, instantiated and deployed.

## Application interface

An rApp implements three methods on its engine type, in the generated internal
logic package. Only the first is required.

| Method | Invoked | Parameters | Returns |
| --- | --- | --- | --- |
| `Handle` | for each message accepted by an ingest source | context, message | an error, or nil |
| `Snapshot` | when an R1 information job is due for delivery | context, job | the payload to deliver, or nil |
| `Register` | once, during startup | router | nothing |

## Packages

| Package | Purpose |
| --- | --- |
| `app` | Lifecycle management: HTTP server, `/health` `/ready` `/status` `/metrics`, signal handling, ordered shutdown |
| `config` | YAML configuration with per-field environment overrides at any nesting depth |
| `auth` | Operator accounts, server-side sessions, and an authorisation guard that exempts platform paths |
| `health` | Dependency probes evaluated behind readiness, logged on state transitions only |
| `metrics` | Prometheus exposition, derived from live snapshots at scrape time |
| `log` | Log levels determined by failure severity |
| `ingest` | Multiple sources feeding a single handler, with backpressure and counters |
| `ingest/httpsrc` | HTTP source for data delivered by a DME job |
| `ingest/kafkasrc` | Kafka source with SASL/SCRAM authentication |
| `pm` | Decoding of 3GPP TS 32.435 XML and VES perf3gpp into a common representation |
| `dlq` | Persistent dead-letter queue with replay |
| `r1` | R1 producer and consumer: supervision, job callbacks, information type discovery, subscriptions |
| `a1` | Near-RT RIC integration: service registration with keep-alive, RIC and policy type discovery, policy lifecycle |
| `sdnr` | RESTCONF access to the managed network through the SMO controller |
| `influx` | Buffered time-series writes and Flux queries |
| `analytics` | Per-entity history, verdicts, cooldowns, journals and counters. No classifier implementations are provided |
| `errs` | Failure categories, from which severity, retryability and HTTP status are derived |
| `retry` | Exponential backoff with jitter, governed by the classification carried by the error |
| `csar` | Package construction and validation equivalent to the platform's own checks |
| `rapptest` | Test harness and platform fakes for testing without a cluster |

All packages are optional; an rApp imports only those it requires.

## Command line interface

```
rappctl new <name>            create a new rApp repository
rappctl package               build the rApp package from rapp-package.yaml
rappctl validate <file.csar>  check a package the way the platform does
rappctl hashpw                hash an operator password
```

## Behaviour and defaults

| Subject | Behaviour |
| --- | --- |
| Ingest overflow | The pipeline applies backpressure by default. The overflow policy may be set to discard instead, where recency is preferred to completeness. Both outcomes are counted |
| Liveness and readiness | Liveness reflects the state of the rApp itself; readiness reflects the state of its dependencies |
| A1 policies on shutdown | Policies are retained when the rApp stops. Withdrawal on shutdown is opt-in |
| R1 delivery | The producer invokes the snapshot method directly rather than the rApp's HTTP API, so enabling authentication does not affect delivery |
| Secrets | Supplied through the chart's values. Values set on an automation composition instance are not applied to the chart |
| Deferred platform failures | An information type whose identifier differs from its file name leaves the instance in DEPLOYING indefinitely, and a comma in the provider function information is rejected by the gateway. Both are detected by `rappctl validate` |

## Documentation

| Document | Contents |
| --- | --- |
| [Writing an rApp](docs/guide.md) | Assembly, configuration, ingest, health, failure handling, durability, analytics, testing |
| [Talking to the network](docs/interfaces.md) | PM decoding, DME, O1 and A1 |
| [Packaging and deployment](docs/packaging.md) | CSAR layout, validation, onboarding and teardown |

## Requirements

Go 1.23 or newer. Helm is required only when a chart is supplied as a directory
rather than as a prebuilt archive.

## Licence

Apache 2.0. Maintained by coRAN Labs.
