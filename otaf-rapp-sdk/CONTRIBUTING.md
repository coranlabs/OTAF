# Contributing to the OTAF rApp SDK

## Getting set up

Go 1.23 or newer, plus `helm` if you want to run the packaging path.

```bash
make build     # compile everything and the CLI
make test      # run the tests
make check     # what CI runs: lint, race, end to end
```

The end-to-end target generates an rApp into a temporary directory, compiles it,
builds its package and validates it. If that passes, the promise the scaffold
makes still holds.

## What belongs in the SDK

Anything every rApp has to do, and nothing that decides what an rApp does.

The line matters. "Keep the last N samples per cell and mark them stale after
five minutes" is plumbing. "A cell with high error rates and high load is
RF-limited rather than congested" is a judgement about radio networks, and it
belongs in an rApp, not here. If a change encodes an opinion about what the
right answer is, it is probably in the wrong repository.

Practical tests for a proposed addition:

- Would at least two unrelated rApps need it?
- Can an rApp that does not want it avoid importing it?
- Does it work when the thing it talks to is absent? Every platform client
  returns a nil client when unconfigured, and a nil client is safe to hold.

## Conventions

**Errors carry what the caller needs to decide.** Platform clients return typed
errors with classification helpers — is it not found, was it rejected — rather
than strings to match on. They classify themselves by implementing four small
methods: one returning the category, one the code, one the HTTP status, and one
reporting whether a retry could help.

No import is needed for that: the failure package finds them by interface. Do
it, and retrying, the dead-letter queue, logging and the failure counter all
behave correctly with nothing else changed.

A package with no error type of its own returns the failure package's error
type directly, as the measurement decoder and the config loader do. Codes must
come from a fixed set — one built from per-message data would give the failure
metric unbounded cardinality.

**Comments explain why, not what.** The code says what it does. A comment earns
its place by recording something a reader could not recover from the code: a
constraint the platform imposes, a decision that looks arbitrary but is not, a
failure mode the shape guards against.

**Tests state the behaviour, not the implementation.** A failure message should
tell you what broke for a user. Prefer test servers that answer the way the real
service does over mocks that answer the way the code expects.

**Nothing reaches the network in a unit test.** Tests must pass on a laptop
with no cluster. Where a real service was used to establish a contract, encode
what it returned into a fake rather than reaching for it again.

## Adding a platform client

Follow the A1 and R1 clients:

1. A config type with environment tags on everything the chart may override, a
   report of whether it was configured, and a validation method.
2. A constructor that returns no client and no error when unconfigured.
3. A typed error carrying the four classification methods above.
4. A ping method so readiness can track it.
5. A name and a start method if it needs a background loop, which makes it an
   application component.
6. A fake in the test package so rApp authors can test against it.

## Changing the packaging rules

The packaging package mirrors what rApp Manager does when a package is uploaded
and primed. Every rule exists because the platform enforces it. When adding one,
say in the finding's hint what the platform does, so an operator hitting it
knows why it matters rather than just that a linter objected.

Do not add rules that reflect taste. A package that would onboard must pass.

## Commits and releases

Write commit subjects as `package: what changed`, in the imperative:

```
a1: re-register after the platform forgets the service
csar: reject deployment items declared under properties
```

Releases are Git tags of the form `v1.1.0`. The SDK is 1.x, so anything
exported keeps working for the life of the major version: an addition is a
minor release, a fix is a patch, and a change that cannot preserve an existing
signature waits for 2.0.0 — which under Go's module rules moves the import path
to `/v2` and is therefore a deliberate, announced event rather than a tidy-up.

Update the version constant and the changelog in the release commit; the
scaffold reads the version from the constant, so a generated rApp always asks
for the SDK that generated it.

## Reporting a security issue

Please report privately rather than opening an issue.
