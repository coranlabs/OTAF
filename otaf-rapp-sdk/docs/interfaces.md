# Talking to the network

## Reading measurements

A RAN emits the same counters as 3GPP TS 32.435 XML or as VES `perf3gpp`
events. `pm` decodes both into one shape, detecting the encoding from the
payload, so a single parse call handles either.

A report carries the format it was decoded from, the distinguished name of the
element that emitted it, the beginning and end of the collection period, the
granularity, and its measurements. Each measurement carries the measurement
family it belongs to, the fully qualified object it was taken against, a suspect
flag, a timestamp, and the counters as name and value.

Counters are kept as written — both encodings carry text and some counters are
not numbers. Typed accessors convert: `Float`, `Int`, `FloatOr`, `Sum`,
`Distribution`, `Names`.

Navigate by object, which is what you key per-entity state on. A report can list
its objects sorted and deduplicated, return every measurement for one of them,
fold those into a single merged measurement when counters are split across
groups, or hand you one counter for one object directly.

Object names are **qualified against the element that reported them**. Elements
write their measurement object names relative to the managed element, so left
alone the same cell looks like a different entity depending on which file it
arrived in.

The suspect flag marks a collection period the element could not measure
cleanly. Acting on it is a decision, not a detail.

Failures classify themselves as permanent bad data, so returning them unchanged
is enough — retry skips them, the dead-letter queue drops them, the API answers
400. Codes: `PM_UNKNOWN_FORMAT`, `PM_DECODE_FAILED`,
`PM_NOT_A_MEASUREMENT_FILE`, `PM_NO_PERF_EVENTS`.

**Handled:** 3GPP both counter-name forms (positioned entries and a whitespace
separated string), both result forms (individual result elements and a single
results string), several measurement data blocks, ISO 8601 granularity periods,
the suspect flag, namespace present or absent. VES a single event, an event list
or a bare array; the measurement info id as object or string; both the string
and integer counter-name lists; both the string and integer value forms; the
suspect flag as boolean or string. Other domains are skipped, not failed.

## Consuming from other rApps

You construct a consumer from the information coordination service config; it
returns nothing when no endpoint is set, and a nil consumer is safe to hold.

Declaring what you want takes a subscription: the job id, the information type
id, the path it should be delivered to — which must match an ingest source's
path — and a definition carrying whatever the type expects, such as a delivery
interval. Add the consumer to the application as a component.

Declaring intent is not placing the job. The component places it on start and
keeps retrying what the platform has not accepted, because an rApp routinely
starts before the one producing what it needs. Ask the consumer what is pending
to surface anything outstanding.

A job can exist and deliver nothing, because no producer serves its type. Two
queries tell you which: the job's status reports whether it is delivering, and
the information type reports whether it is available. Ask before subscribing.

The job id and the owner must be stable across restarts, or a restarting rApp
accumulates duplicate jobs.

> **The mismatch that reports nothing.** An information type is registered under
> its **file's base name**, and the information type id must equal it exactly.
> Name the type in upper case with underscores while the file uses lower case
> with hyphens and nothing is rejected: the participant retries for ever, the
> instance sits in DEPLOYING, and because the DME element deploys first the Helm
> install never begins. Validation cross-checks this.

## Publishing over R1

An rApp registers as a producer in its package; at runtime the SDK answers the
callbacks that implies and calls your snapshot method when a job is due, handing
it the job and expecting the bytes to deliver.

Return nothing to deliver nothing this round. Cadence comes from the job when
the consumer asked for one, otherwise the producer's default. Snapshot is a
direct call, not an HTTP request to your own API, so delivery never depends on
how your endpoints are secured.

## Acting on the network

| | `sdnr` | `a1` |
| --- | --- | --- |
| Reaches | one node, over O1 | the Near-RT RIC, over A1 |
| Grain | a configuration change you make | a policy the RIC enforces continuously |
| Lifetime | until you change it back | until withdrawn, or the RIC drops it |
| Validated by | the node's YANG model | the policy type's JSON schema |

### O1

The controller client builds a mount path from the managed element and the
function beneath it, each named the way the node's YANG model names them, and
then patches that path with your body.

Three helpers separate what the node refused from what never reached it: not
found, rejected, and unreachable. Only the last is worth retrying, and only if
the call is idempotent.

### A1

Register before placing anything, and keep registering — let the keep-alive
lapse and every policy is withdrawn. Construct the client from the A1 config,
which returns nothing when no endpoint is configured, and add it to the
application as a component.

As a component it registers on start, heartbeats, and re-registers by itself if
the platform forgets it. Keep the service id stable so an rApp reclaims what it
created before a restart.

Placing a policy is two steps. First ask for a RIC that serves the policy type
you want for the managed element you care about; unavailable RICs are skipped.
Then put the policy: its own id, the RIC's id, the policy type id, whether it is
transient, and the policy data as raw JSON matching the type's schema.

Putting creates and replaces, so re-applying a decision is one call. Marking a
policy transient tells the RIC it may drop rather than restore it — right for a
decision only valid now. Deleting treats absent as success, and there is a
delete-all that withdraws everything this rApp placed.

Acceptance is not enforcement. Policy status returns when it was last modified
plus a raw status whose contents are defined by the RIC, not by A1.

A rejection means the data did not match the type's schema — the service sends
no explanation, so the client supplies one. Fetch the schema through the policy
type query when the reason is not obvious.

**Stopping does not withdraw policies.** A rolling restart would otherwise
revert the network for as long as the rApp takes to come back. Opt in by asking
the client to deregister on stop, when standing down is the intent. An rApp that
stops for good still has its policies withdrawn once the keep-alive lapses.
