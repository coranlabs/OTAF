# Writing an xApp

## How it fits together

An xApp is a process that registers itself, subscribes to one or more RANs, and
acts on what they report. The SDK covers the registering, the subscribing and
the reading. What to do with a measurement is the whole of an xApp's value and
none of the SDK's business.

Three things reach the network: registration, subscription placement, and the
messages RMR carries. The first two are HTTP and belong to this SDK. The third
is the O-RAN SC xApp frame's, and a generated xApp uses it directly.

## Registering

Nothing works until an xApp is registered. The routing manager builds its route
table from what the xApp manager holds, so an unregistered xApp has no route,
and a subscription placed by one is refused inside the RIC before the RAN ever
hears about it. The failure is reported as a routing problem, which is some
distance from its cause.

Registration is also where the endpoint names are settled. They are derived
from the xApp's name rather than from where the process happens to listen, and
whatever runs the xApp has to answer on them. The descriptor's endpoint methods
report the names that will be used, so the deployment can be made to match.

Two details are easy to get wrong and quiet when you do. The route port is
fixed by the platform whatever the descriptor says, so declaring another leaves
the xApp registered and unreachable; validation refuses it rather than letting
it through. And the descriptor travels as text holding JSON, because the
platform decodes that field a second time — sent as an object it is rejected,
with an error that does not say so.

## Finding out what a RAN offers

A RAN publishes what it can report when it sets up its E2 connection, and the
platform keeps it. Decoding that tells you which report styles exist, which
counters each carries, and which action definition format a subscription for
that style has to use.

The formats differ per style, and a subscription built with the wrong one is
refused by the RAN. Taking the format from what the RAN advertised rather than
from a constant is the difference between a subscription that works and one
that fails for reasons the error does not explain.

Identify a service model by its object identifier, never by the function
number. The number is assigned by the RAN and differs between them; the
identifier does not.

Measurement identifiers are optional and some RANs omit them entirely,
reporting by name alone. A counter is matched by name for that reason.

## Subscribing

A subscription carries two encoded payloads: when the RAN should report, and
what it should report. Both are built from the RAN's own description of itself,
so asking for a style it does not offer, or a counter it does not measure, is
refused before anything is encoded.

Placing the subscription returns as soon as the subscription manager has taken
the request. Whether the RAN accepted it arrives later, over HTTP, at the
address the registration published. Something has to be listening there or the
subscription is never completed.

Expect to retry. The routing manager needs a moment after registration before
it will build a route, and a subscription placed in that window is refused. The
failure says so in its category, so backing off and trying again is the correct
response and stopping is not.

## Reading what comes back

A report arrives wrapped in E2AP with the service model payload inside, so
reading one means unwrapping the outer message and then decoding the inner. The
SDK does both, and keeps them separate because what is inside depends on which
RAN function the report came from.

The measurements arrive as rows of values with the names in a separate list,
aligned by position. A counter may be an integer, a real, or absent for that
interval, and the three are kept apart rather than flattened into a zero: a RAN
that had nothing to report is saying something different from one that measured
zero.

## When things fail

Every failure carries a category, and everything else follows from it. A
descriptor the platform refuses will be refused again, so it stops there. A
platform that is not answering yet is worth another attempt, which is the
ordinary case while a cluster is still coming up. A payload that will not
decode is neither, and retrying sends the same bytes again.

Retrying reads that category rather than guessing, so a caller does not decide
per site what is worth repeating. Backoff carries jitter, which matters when a
fleet of xApps recovers from one outage: without it they return in step and
strike the recovering service together.

## Testing

Registration is the only thing in this SDK that reaches the network, so it is
the only thing that needs standing in for. The test package provides that, and
can be made to refuse so an xApp's own handling of a refusal is exercised
rather than assumed.

Everything else takes bytes and returns values, and is tested by handing it
bytes. The samples that come with the test package are a report and a function
definition a RAN actually sent, rather than something built to agree with the
decoder it is checking.
