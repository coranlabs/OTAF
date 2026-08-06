# Writing an rApp

## How it fits together

One binary, one HTTP server, one ingest pipeline, plus whatever components the
rApp adds. The application's run function owns all of it and stops it in order.

You construct the application from the rApp section of your settings and then
pass options for the pieces you use: a logger, an auth guard, the ingest
pipeline, and a component for each background part such as an R1 producer or a
dead-letter queue.

Platform endpoints are registered first, so your routes can never shadow a
probe. Sources and components that serve HTTP are wired automatically: anything
able to register routes on the router gets its endpoints added, and anything
that can list open paths has those exempted from the operator guard.

## Configuration

Configuration is loaded from one YAML file — normally the chart's ConfigMap,
found through the config path variable — after which the environment overrides
individual fields.

Your settings type nests one section per component: the rApp itself, and a
section for each platform client you use, such as the controller and the
time-series store. Each section is the config type that client publishes.

Any field tagged with an environment variable name is overridden when that
variable is set and non-empty, at any depth. An empty variable counts as unset,
so a chart rendering a blank value never wipes out the file. That is how secrets
come from a Secret while everything else comes from the ConfigMap.

## Ingest

```
source ──┐
source ──┼──▶ queue ──▶ worker ──▶ Handle()
source ──┘
```

The queue **blocks** by default, applying backpressure rather than losing data.
Setting the overflow policy to dropping inverts that when the newest data
matters more than all of it; both are counted. One worker by default, because
logic accumulating state per cell usually depends on ordering — the worker count
is an option that lifts it.

The HTTP source answers `503` when its buffer is full, so the sender retries
instead of being told data was accepted and then dropped.

## Health and metrics

| Endpoint | Answers | Used for |
| --- | --- | --- |
| `/health` | is the process alive? | liveness probe |
| `/ready` | can it do its job? | readiness probe |
| `/status` | everything, in detail | operators |
| `/metrics` | Prometheus exposition | scraping |

`/health` deliberately ignores dependencies: restarting will not fix an
unreachable controller, and a liveness probe that failed on it would crash-loop
the rApp during someone else's outage.

Adding a dependency is one call on the application's health registry, naming it
and giving the function that probes it — typically a client's ping. That single
line puts the dependency in readiness, on `/status`, and in
`rapp_dependency_up`. Probes log only on transitions.

Metrics: `rapp_build_info`, `rapp_ingest_messages_total{outcome}`,
`rapp_ingest_queue_depth`, `rapp_handler_duration_seconds{source,outcome}`,
`rapp_dependency_up{dependency}`, `rapp_failures_total{category,code}`, plus Go
runtime collectors. Counters are read from the rApp's
own snapshots at scrape time, so `/status` and `/metrics` cannot disagree.

Publish your own on the same endpoint by registering them with the registerer
the application's metrics component exposes.

The scaffold ships a ServiceMonitor, off by default. Enable it where the
Prometheus Operator is installed and label it with whatever your Prometheus
selects on:

```bash
kubectl get prometheus -A -o jsonpath='{..serviceMonitorSelector}'
```

## Failures

A category answers three questions the message cannot: whose fault, worth
retrying, worth waking someone.

| Category | Severity | Retryable | HTTP |
| --- | --- | --- | --- |
| `config` — someone set it up wrong | critical | no | 500 |
| `platform` — a platform service refused or was away | error | yes | 502 |
| `network` — the RAN refused or was unreachable | warning | yes | 502 |
| `data` — the data itself is wrong | warning | no | 400 |
| `internal` — a bug | error | no | 500 |

You raise one by constructing an error with a category, a fixed code and a
message — for a malformed payload, the data category with a code naming it.

One label, four consequences: retry does not retry it, the dead-letter queue
drops rather than parks it, logging warns with the code attached, metrics count
it under `category="data"`. Change the category to platform and all four flip.

Nothing has to be wrapped — the helpers that read category, code, severity and
status work on any error and report `unknown` rather than assuming harmless.
Override the defaults by marking an error transient or permanent, or by
attaching a status or a field.

Log through the failure helper: the level follows the severity, and a critical
failure still logs at error — a library that killed the process would take that
decision away from you.

Errors from any package classify themselves by implementing small
string-returning methods for the category and code, and a boolean for whether a
retry could help. No import is required.

## Not losing data

The pipeline holds messages in memory; a restart loses them. The dead-letter
queue parks failures on disk and replays them.

Integration is three steps: construct the queue from its config, wrap your
engine with it when building the pipeline, and add the queue to the application
as a component. That is the whole of it.

Only failures that might pass are kept: a data or config category — or an error
explicitly marked permanent — is counted and dropped, so the queue never fills
with what no retry will fix.

Its configuration section takes a directory, a maximum number of entries, a
maximum age and a maximum number of attempts. Leaving the directory empty keeps
the queue in memory, and a restart then loses it.

Three separate ways it gives up, each counted: `exhausted`, `expired`,
`overflow`.
Overflow drops the **oldest**. A file that will not decode is renamed with a
corrupt suffix rather than deleted. Operate it by listing entries, reading
stats, retrying all or one, and discarding.

The directory needs a volume that outlives the pod.

## Deciding over time

`analytics` is the machinery around a decision and none of the decision. **It
ships no classifiers** — the interface is one method and every implementation
is yours.

```
observe ──▶ decide ──▶ guard ──▶ act ──▶ record ──▶ count
Registry    Classifier  Cooldown   yours   Journal   Buckets
```

You create a registry for your KPI type, giving it your classifier, how many
samples to keep per entity, and how long without a sample counts as stale. Then
you observe: an entity id, the time of the report and the KPI. The result tells
you whether the sample was accepted and whether the verdict changed, so you can
act on the transition rather than on every report.

Your part is the classifier. It receives the samples held for one entity and
returns a verdict: a score, a set of named signals, and a state with a reason.
The statistics helpers give you the mean, the latest value and the slope over a
series, so a classifier is usually a handful of lines that pull one counter out
of each sample and compare it against your own limit.

Samples not newer than the last are rejected — a repeated report entering the
window twice corrupts a trend invisibly. Staleness is measured against
**arrival**, so a stopped feed is detected even while replaying old timestamps.

The guard stops an rApp acting on every report until a KPI it influences
catches up. You ask the cooldown whether this entity is allowed to act now, and
you mark it only when the action succeeded — a failed attempt does not consume
the cooldown.

Time is passed in, not read from a clock: live logic wants wall time, replayed
data wants the sample's time, and only you know which.

The journal is a bounded in-memory record for a human looking at a running
rApp, not an audit trail — anything that must survive a restart belongs in the
time-series store. Buckets are wall-clock aligned so a restart does not shift
them. Statistics are textbook only: `Mean`, `Min`, `Max`, `Last`, `Percentile`,
`StdDev`, `Slope`, `ChangePct`, `Clamp01`, all NaN-tolerant and zero on empty.

## Testing

`rapptest` drives your logic without a cluster. The fakes are the real clients
pointed at stand-in servers, so the code takes its production path.

A test constructs the fakes it needs — policy management, a time-series store,
a controller — each returning both the client to hand to your engine and a
handle to assert on. You build your engine from those, wrap it in the harness,
send it reports, and then assert on what the fakes recorded: the policy that was
placed, the writes that were made, the points that were persisted.

Nothing runs in the background: the handler has returned by the time a send
does, so assertions need no polling.

| | |
| --- | --- |
| Send one, send several | drive the handler; fails the test on error |
| Send expecting an error | when the refusal is the point |
| Snapshot, snapshot-is-empty | assert what you publish |
| Policy, reject | assert policies; make the platform refuse |
| Writes, reject-writes | assert O1 writes; make the node refuse |
| Lines, flush | assert persisted points |

You do not need tests for config loading, the HTTP surface, backpressure, the
R1 or A1 exchanges, or packaging. Those are the SDK's and it has its own. Test
what only you can: what your rApp concludes, and what it does about it.

## Shutdown

`SIGTERM` cancels the root context: the server stops accepting, in-flight
requests finish, the pipeline drains what it queued, components return.
Deployments are `Recreate` rather than rolling — two instances of an rApp that
acts on the network is rarely wanted.
