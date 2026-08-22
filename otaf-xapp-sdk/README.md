# OTAF xApp SDK

A Go SDK for building xApps for the O-RAN Near-RT RIC. It implements the
platform-facing concerns — registration, E2AP, the E2SM-KPM service model and
failure handling — so that an xApp contains only its own logic.

Apache 2.0. Requires Go 1.23 or newer and a C compiler. The SDK itself has no
Go dependencies.

## Installation

```bash
go install github.com/coranlabs/OTAF/otaf-xapp-sdk/cmd/xappctl@latest
```

## Getting started

```bash
xappctl new my-xapp
cd my-xapp
go mod tidy
docker build -t my-xapp:1.0.0 .
kubectl apply -f deploy/my-xapp.yaml
```

The `new` command generates a Go module and entry point, a Dockerfile, and a
Kubernetes manifest comprising a Deployment and the two services the platform
addresses the xApp by. The generated xApp registers itself on startup and
deregisters on shutdown.

## Reading a RAN

An xApp asks the platform what a RAN offers, subscribes to part of it, and
decodes what comes back.

```go
fn, err := e2.DecodeRANFunction(definition)

sub, err := fn.Subscribe(e2.Request{
    Style:        1,
    Measurements: []string{"RRC.ConnMean", "RRC.ConnMax"},
    Period:       10 * time.Second,
    Cell:         &e2.Cell{PLMN: plmn, NRCI: nrci},
})

ind, err := e2.DecodeIndication(payload)
report, err := e2.DecodeIndicationMessage(ind.Message)
report.Latest()["RRC.ConnMean"]
```

The action definition format is taken from the report style the RAN advertised
rather than assumed, because the formats differ per style and a mismatch is
rejected by the RAN.

## Packages

| Package | Purpose |
| --- | --- |
| `e2` | E2AP indications and the E2SM-KPM service model: what a RAN offers, subscriptions to it, and the measurements it reports |
| `appmgr` | Registration with the xApp manager, and offline validation of the descriptor |
| `errs` | Failure categories, from which severity, retryability and HTTP status are derived |
| `retry` | Exponential backoff with jitter, governed by the classification carried by the error |
| `submgr` | Placing subscriptions with the subscription manager, and withdrawing them |
| `xapptest` | A stand-in xApp manager and samples captured from a RAN, for testing without a cluster |

All packages are optional; an xApp imports only those it requires.

## Command line interface

```
xappctl new <name>            create a new xApp repository
```

## Behaviour and defaults

| Subject | Behaviour |
| --- | --- |
| Registration | Until an xApp is registered the routing manager has nowhere to send its traffic, and a subscription is refused before it reaches the RAN |
| Endpoint names | Routes are built from names derived from the xApp name, not from where the process listens. Whatever runs the xApp has to answer on them |
| RMR route port | Fixed at 4561 in the platform, whatever the descriptor declares. Any other value leaves the xApp registered and unreachable, and `Validate` refuses it |
| Subscription callback | The subscription manager posts its response to the same name the routing manager routes to, so that one service carries both ports |
| Descriptor encoding | The descriptor is sent as a string holding JSON, because the platform decodes that field a second time. Sent as an object it is rejected |
| Report styles | Only styles whose action definition format is 1 or 3 can be built. The rest are refused by name rather than encoded into something the RAN would reject |
| Measurement identifiers | Optional, and a CU-CP omits them. A measurement is matched by name |

## Documentation

| Document | Contents |
| --- | --- |
| [Writing an xApp](docs/guide.md) | Registration, discovery, subscribing, decoding, failure handling, testing |
| [Deploying an xApp](docs/deployment.md) | Names, ports, building, and removing one |

## Requirements

Go 1.23 or newer, and a C compiler: the E2AP and E2SM-KPM codec is compiled as
part of the package, so there is nothing to install and nothing to link
against.

A generated xApp additionally uses the O-RAN SC xApp frame for RMR, which
requires the RMR library at build and run time. The generated Dockerfile
installs it.

## Licence

Apache 2.0. Maintained by coRAN Labs.
