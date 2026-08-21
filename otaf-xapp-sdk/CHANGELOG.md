# Changelog

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this SDK follows [semantic versioning](https://semver.org/spec/v2.0.0.html).

Releases are tagged with the module's directory prefix, `otaf-xapp-sdk/v1.0.0`,
because a bare version tag is not visible to a submodule.

## [1.0.0] - 2026-08-20

First release.

### Added

- `e2` decodes what a RAN advertises over E2SM-KPM, builds subscriptions to it,
  and reads the reports that come back. The action definition format is taken
  from the report style the RAN published rather than assumed.
- `appmgr` registers an xApp with the xApp manager, and checks the descriptor
  against what the platform would reject before anything is sent.
- `errs` and `retry` classify a failure once, so retrying, logging and alerting
  all read the same label.
- `xapptest` stands in for the xApp manager and carries a report and a function
  definition captured from a RAN.
- `xappctl new` generates an xApp that registers itself, with the services the
  platform addresses it by and a Dockerfile that installs what RMR needs.

### Notes

The E2AP and E2SM-KPM codec is compiled as part of the `e2` package, so there
is nothing to install and nothing to link against. Its provenance and the
changes made to it are recorded in `e2/NOTICE`.

A generated xApp additionally uses the O-RAN SC xApp frame for RMR, which does
require the RMR library. Several of the platform's Go modules declare paths
their repositories do not serve, so the generated `go.mod` carries the
replacements that makes them resolve.

[1.0.0]: https://github.com/coranlabs/OTAF/releases/tag/otaf-xapp-sdk%2Fv1.0.0
