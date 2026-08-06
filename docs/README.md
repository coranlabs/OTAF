# What OTAF is

The **Open Telecom Application Framework** is a set of Go SDKs for writing
O-RAN applications, sharing one model of what an application is: something that
observes the network, decides, acts, and is packaged so a platform can onboard
and lifecycle-manage it.

O-RAN splits network control across three timescales, and gave each its own
application type, its own host and its own interfaces. That split is real and
necessary — a decision about spectrum reuse across a region cannot be made in a
millisecond, and a scheduling decision cannot wait a minute. But it means three
different execution environments, three sets of interfaces, and three ways to
package the same shape of work.

OTAF makes the shape common. What differs between an rApp, an xApp and a dApp
is where it runs, how fast it must answer, and what it is allowed to touch —
not how you write it.

## The layers

```
  Network layer     RU ── DU ── CU              the radio and the protocol stack
                     │     │     │
                     │   E3│     │E3            dApps run inside the node
                     │     └──┬──┘
                     │     E2 │                 telemetry and control
  Execution layer    │   Near-RT RIC            xApps: 10 ms – 1 s
                     │        │ A1              policy, not commands
                     └──── Non-RT RIC / SMO     rApps: seconds and longer
  Platform layer          R1 · O1               onboarding, data, management
```

**Network layer.** The radio units, distributed units and centralised units
that carry traffic. Everything above exists to steer it.

**Execution layer.** The two RICs. The Near-RT RIC hosts xApps and speaks E2 to
the nodes. The Non-RT RIC, inside the SMO, hosts rApps and speaks A1 down to
the Near-RT RIC.

**Platform layer.** The services an application talks to rather than the
network: onboarding and lifecycle, data subscription and publication, service
exposure, and configuration management.

## The three application types

### rApp — network intelligence

Runs in the Non-RT RIC as part of the SMO. Timescale of a second and upwards,
often minutes. Sees the whole network rather than one cell, which is what makes
it the right place for decisions needing breadth or history: capacity planning,
energy saving, policy authoring, anomaly detection over a long window, training
the models the faster loops use.

It reaches the network two ways. Over **O1** it configures nodes directly,
through the SMO's controller. Over **A1** it issues *policies* to the Near-RT
RIC — statements of intent that the RIC enforces continuously, at a timescale
the rApp itself could never hold.

Data comes over **R1**, the interface to the platform: an rApp subscribes to
what other producers publish, and publishes what it derives.

### xApp — near-real-time RAN intelligence

Runs in the Near-RT RIC. Timescale between 10 ms and 1 s — fast enough for
mobility, load balancing and admission control, and to react to conditions an
rApp would only see afterwards.

It speaks **E2** to the CU and DU: subscribing to indications and issuing
control. It receives **A1** policies from above and works inside them, which is
how the two loops coordinate without fighting: the rApp says what good looks
like, the xApp decides moment to moment how to get there.

### dApp — real-time RAN intelligence

Runs **inside** the CU or DU rather than in a RIC, because below about 10 ms
there is no time to leave the node. That is where scheduling, beam management
and spectrum sensing live — decisions needing PHY and MAC telemetry that never
leaves the stack.

The interface is **E3**, carrying setup, subscription, indication, control and
report between the node and the dApp hosted on it. Bridging E3 with E2 lets
dApps, xApps and rApps coordinate hierarchically rather than contend over the
same resources.

dApps are newer than the other two: the concept comes from O-RAN's research
group rather than a ratified working-group specification, and the interface is
still settling. The SDK will follow it as it does.

## How they fit together

The three are one control system at different speeds, not three products:

- The **rApp** learns from breadth and history, and expresses conclusions as
  policy.
- The **xApp** applies that policy to what is happening now, per cell and per
  user.
- The **dApp** acts inside the node, where the decision is too fast to travel.

A conflict between them is resolved by timescale: the slower loop sets the
envelope, the faster loop moves within it.

## What an OTAF SDK gives you

The same in each case, adapted to where the application runs:

**Lifecycle.** One HTTP server, the health and readiness endpoints the platform
probes, Prometheus metrics, signal handling and ordered shutdown.

**Configuration.** One file from the deployment, overridden per field by the
environment, so secrets stay in a Secret and everything else in a ConfigMap.

**Ingest.** Sources feeding one handler, with explicit backpressure, counters,
and a dead-letter queue for what could not be processed.

**Platform clients.** Typed access to the interfaces that application can use,
with failures that classify themselves — whose fault, worth retrying, worth
waking someone — so retrying, dead-lettering, logging and metrics all follow
from one label.

**Descriptors and packaging.** The application's package built and validated
offline against the platform's own rules, so a package that passes locally is
one the platform accepts.

**Testing.** A harness and fakes for the platform services, so the logic can be
tested without a cluster.

What no OTAF SDK provides is a decision. The interface for a verdict is one
method and every implementation is yours: what the numbers mean is the whole of
an application's value.

## Descriptors and onboarding

An O-RAN application is not deployed by hand. It is described, uploaded, and
lifecycle-managed by the platform — and the description is where most first
attempts fail, because the errors surface late and far from their cause.

Each SDK builds its application's package and checks it against what the
platform actually reads, before it is uploaded: identifiers that must be unique
or must stay stable, resources named by file whose names other files must match
exactly, and the handful of shapes accepted at upload but rejected during
deployment.

## Status

The rApp SDK is released at v1.0.0. The xApp and dApp SDKs are planned and will
follow the same structure and the same guarantees.
