# Deploying an xApp

## What the platform needs from you

Two steps, and they are independent. Run the workload, then tell the xApp
manager about it. Deploying without registering leaves an xApp that runs and
receives nothing; registering without deploying leaves a route to an address
that does not answer.

There is a second path, where the platform installs the workload itself from a
chart. It needs a chart repository, an onboarder and a Helm installation behind
the xApp manager, and it is not required. Registration alone is what makes the
routing manager carry traffic.

## Names

The routing manager addresses an xApp by names derived from its name, not by
where the process listens. A service of that name has to resolve, and what is
behind it has to answer.

The same name is used twice, for different things. The routing manager routes
RMR to it, and the subscription manager posts its answer to it over HTTP. One
service therefore has to carry both the messaging ports and the HTTP port. Put
the HTTP port on a service of its own and the subscription is placed, accepted,
and then never completed, because the answer has nowhere to go.

The messaging library also has to announce itself under that same name. Where
it announces something else, sends are dropped and nothing reports it.

## Ports

The route port is fixed at 4561 by the platform. It is not read from the
descriptor: the routing manager opens its connection there whatever the
descriptor declares. An xApp that asks for another port registers cleanly and
is then unreachable, and the only sign is that the routing manager could not
open a socket to it.

The messaging library listens on the data port and the one after it, so the
pair has to land such that the second is the route port.

## Building

The codec is compiled as part of the SDK, so building an xApp needs a C
compiler and nothing else installed. A generated xApp additionally uses the
xApp frame for messaging, which does require the messaging library at build and
run time; the generated container file installs it.

Because of that library the binary is tied to the C library of the system that
built it, and will not run on an older one. Build it in the same image it runs
in rather than copying a binary between machines.

Several of the platform's Go modules declare import paths their repositories do
not serve. A build fails naming a transitive dependency rather than the cause.
The generated module file carries the replacements that resolve them; they take
effect only in an application, which is why they are not in the SDK.

## Removing one

Undo both steps. Deregister, then delete the workload. An xApp left registered
after its workload is gone keeps a route pointing at nothing, and the next
subscription placed against that name is accepted and then silently unanswered.
