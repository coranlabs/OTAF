# Changelog

Notable changes to the OTAF rApp SDK. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[semantic versioning](https://semver.org/spec/v2.0.0.html).

From 1.0.0 on, anything exported keeps working for the life of the major
version. A change that cannot preserve that waits for 2.0.0, which under Go's
module rules would move the import path to `/v2`.

## [Unreleased]

### Added

- `errs` — failure classification by category, with severity, retryability and
  HTTP status following from it. The helpers work on any error, and any package
  can classify its own by implementing small string-returning methods, so
  `a1`, `r1`, `sdnr` and `config` classify without importing anything.
- `pm` — decodes 3GPP TS 32.435 measurement files and VES perf3gpp events into
  one shape, covering both counter-name forms, both result forms, multiple
  measData blocks, and the suspect flag. Counter names and what they mean stay
  with the rApp.
- `dlq` — parks messages the handler could not process, on disk, and replays
  them with growing backoff. Failures marked `retry.Permanent` are dropped
  rather than parked, so the queue does not fill with data no retry will fix.

### Fixed

- `rappctl package` now mints a fresh automation composition **element id** on
  every build. The platform remembers element ids and refuses one it has seen
  before, and rows survive a failed deploy, so a package shipping the id
  committed to the repository could be onboarded once per environment and never
  again — failing the second time with a duplicate-element error that points
  nowhere near the cause.

### Added

- Validation rules for three platform rejections that surface late and
  unhelpfully: an information type named differently from its file (the
  instance sits in DEPLOYING for ever with nothing logged), a comma in
  `apiProvFuncInfo` (the gateway refuses it as a tag; 502 at SME deploy), and a
  service API published at `/` or at a templated path (no route can be built).
- `rappctl package --check-charts` warns when the chart repository already
  publishes the version being built, which would mean the older chart — with
  its old values and secrets — is what actually deploys.
- Documentation for the operational path the SDK could not previously reach:
  the exact onboarding calls, the strict teardown order, that rApp Manager
  paths take the rApp **name** and never the returned id, the exposure-layer
  leftovers that break the next deploy, the chart-repository allow-list needed
  before the first prime, and that built packages contain live credentials.

### Changed

- The classification is now load-bearing rather than available. `pm` returns
  already-classified failures, so no caller labels a decode failure by hand;
  the ingest pipeline logs at the level the severity calls for with the
  category and code attached; `app.Fatal` reports the category, so a crash loop
  can be told apart from a slow start; and every handler failure is counted as
  `rapp_failures_total{category,code}`.
- `sdnr` now returns a typed `*sdnr.Error` rather than formatted strings,
  carrying the method, path, status and cause, with `IsNotFound`, `IsRejected`
  and `IsUnreachable`. A request that never reached the controller is now
  distinguishable from one the node refused, and only the first is retried.
- `analytics` — the machinery around a decision, and none of the decision:
  per-entity sample history with staleness measured against arrival, a bounded
  journal, a per-entity cooldown, wall-clock-aligned counters, and textbook
  statistics helpers. It ships the `Classifier` interface and no
  implementations of it, deliberately.

## [1.0.0]

First release.

### Added

- `app` — rApp lifecycle: HTTP server, `/health`, `/ready`, `/status`,
  `/metrics`, signal handling and ordered shutdown. Liveness answers on the
  rApp's own state and readiness on its dependencies, so a platform outage
  cannot get a working rApp restarted.
- `config` — YAML from the deployment's ConfigMap with per-field environment
  overrides via `env` struct tags, at any nesting depth.
- `ingest` — sources feeding one handler, with an explicit overflow policy,
  counters, and an observer hook for timing.
- `ingest/httpsrc` — receive data pushed over HTTP. Answers `503` rather than
  `200` when its buffer is full, so a sender retries instead of being told data
  was accepted and then discarded.
- `ingest/kafkasrc` — consume a platform topic, with SASL/SCRAM. Offsets are
  committed only after a message is queued.
- `r1` — data producer (supervision, job callbacks, per-job delivery cadence)
  and data consumer (information type discovery, standing subscriptions that
  are retried until the platform accepts them).
- `a1` — steer the Near-RT RIC: service registration with keep-alive and
  automatic re-registration, RIC and policy type discovery, and the policy
  lifecycle.
- `sdnr` — RESTCONF to the managed network through the SMO controller.
- `influx` — buffered time-series writes and Flux queries.
- `auth` — operator accounts, server-side sessions, and a guard that leaves
  platform paths reachable.
- `health` — dependency probes behind readiness, logged only on transitions.
- `metrics` — Prometheus exposition. Counters are read from the rApp's own
  snapshots at scrape time rather than mirrored, so the numbers cannot drift
  from what `/status` reports.
- `retry` — exponential backoff with jitter, and errors that say for themselves
  whether another attempt could help.
- `rapptest` — a harness and fakes for the platform services, so an rApp's own
  logic can be tested without a cluster.
- `csar` — build the rApp package and check it the way the platform will,
  offline.
- `rappctl` — `new`, `package`, `validate` and `hashpw`.

### Notes

- Deployment items are declared in the ASD's `artifacts` block. Anything under
  `properties.deploymentItems` is not read by the platform and leaves the
  package with nothing to deploy; `rappctl validate` rejects that shape.
- Versions are emitted as quoted strings everywhere. Left bare, YAML reads a
  version such as `1.10` as the number `1.1`, and the chart that gets packaged
  is not the one that was asked for.
- Stopping an rApp does not withdraw its A1 policies. A rolling restart would
  otherwise revert the network for as long as the rApp takes to come back.
  Standing down is opt-in through `a1.DeregisterOnStop(true)`.

[Unreleased]: https://github.com/coranlabs/OTAF/otaf-rapp-sdk/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/coranlabs/OTAF/otaf-rapp-sdk/releases/tag/v1.0.0
