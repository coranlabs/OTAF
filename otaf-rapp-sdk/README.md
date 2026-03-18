# OTAF rApp SDK

A Go SDK for building rApps for the O-RAN Non-RT RIC. It implements the
platform-facing concerns — lifecycle, configuration, ingest, R1, A1, O1 and
packaging — so that an rApp contains only its own logic.

Apache 2.0. Requires Go 1.23 or newer and six direct dependencies.

## Packages

| Package | Purpose |
| --- | --- |
| `errs` | Failure categories, from which severity, retryability and HTTP status are derived |
| `log` | Log levels determined by failure severity |
| `retry` | Exponential backoff with jitter, governed by the classification carried by the error |
| `config` | YAML configuration with per-field environment overrides at any nesting depth |
| `health` | Dependency probes evaluated behind readiness, logged on state transitions only |
| `metrics` | Prometheus exposition, derived from live snapshots at scrape time |
| `auth` | Operator accounts, server-side sessions, and an authorisation guard that exempts platform paths |
| `ingest` | Multiple sources feeding a single handler, with backpressure and counters |
| `ingest/httpsrc` | HTTP source for data delivered by a DME job |
| `ingest/kafkasrc` | Kafka source with SASL/SCRAM authentication |
| `pm` | Decoding of 3GPP TS 32.435 XML and VES perf3gpp into a common representation |
| `dlq` | Persistent dead-letter queue with replay |
| `analytics` | Per-entity history, verdicts, cooldowns, journals and counters. No classifier implementations are provided |
| `sdnr` | RESTCONF access to the managed network through the SMO controller |
| `influx` | Buffered time-series writes and Flux queries |
| `a1` | Near-RT RIC integration: service registration with keep-alive, RIC and policy type discovery, policy lifecycle |
| `r1` | R1 producer and consumer: supervision, job callbacks, information type discovery, subscriptions |

## Licence

Apache 2.0. Maintained by coRAN Labs.
