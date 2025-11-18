# OTAF rApp SDK

A Go SDK for building rApps for the O-RAN Non-RT RIC. It implements the
platform-facing concerns — lifecycle, configuration, ingest, R1, A1, O1 and
packaging — so that an rApp contains only its own logic.

Apache 2.0. Requires Go 1.23 or newer and six direct dependencies.

## Packages

| Package | Purpose |
| --- | --- |
| `errs` | Failure categories, from which severity, retryability and HTTP status are derived |

## Licence

Apache 2.0. Maintained by coRAN Labs.
