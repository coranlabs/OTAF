# Packaging and deployment

rApp Manager onboards an rApp as a CSAR. Packaging builds it and checks it the
way the platform will.

## Layout

The package is a zip archive named for the rApp and its version, holding:

| Part | What it is |
| --- | --- |
| TOSCA metadata | points at the descriptor |
| Definitions | the application service descriptor and its type definitions |
| Manifest | named for the rApp |
| Deployment artifacts | the Helm charts the platform uploads |
| Files, under Acm | the composition definition and the instance bodies |
| Files, under Dme | producer and consumer information types, information producers and consumers |
| Files, under Sme | providers, service APIs and invokers |

Everything under the files section is discovered by directory — drop a JSON
file in the right place and it is picked up.

The ONAP type names in the composition definition are the identifiers the
automation composition runtime registers and matches on exactly. They stay as
written.

## Resources are named by file

A resource's identifier is its file's base name, stripped of directory and
extension. A service API file named for the rApp's API becomes exactly that
name, which is what the rApp instance refers to.

The name is cut at its **last** dot, so a file carrying a version in its name
loses everything from that dot onward. Validation rejects more than one dot.

This bites hardest with information types — see the DME warning in
[Talking to the network](interfaces.md#consuming-from-other-rapps).

## The descriptor

A package descriptor drives the build. It carries:

| Field | What it sets |
| --- | --- |
| Name, version, provider, description | how the rApp identifies itself |
| Descriptor id | this package |
| Descriptor invariant id | this rApp, across versions |
| Charts | for each, where the chart is and the repository to upload it to |
| Resource directory | where the files section is read from |
| Output directory | where the built package is written |

Keep both identifiers stable: the platform refuses a package whose descriptor
id it has onboarded before, and the invariant id is what ties versions
together. A chart may be given as a directory, which is packaged with `helm`,
or as a prebuilt chart archive.

## Deployment items are declared as artifacts

Each chart is declared in the descriptor's **artifacts** block, keyed by the
rApp name and version, typed as an application service descriptor deployment
item. Its properties name the artifact type as a Helm chart, the target server,
the repository URI to upload to, and an item id.

Deployment items are read **only** from that artifacts block, and whatever is
there replaces anything declared elsewhere. A descriptor listing charts under
the properties deployment-items key instead produces an empty list, and priming
fails with *"No deployment items found in ASD metadata"*. An empty repository
URI fails the same step. Both are validation errors here.

## Identifiers the platform substitutes

Two placeholder identifiers are replaced at prime and instance time: one
standing in for the primed composition's id, one for the new instance's id.
Both read as capitalised do-not-change tokens. Leave them exactly as written.

A third is handled for you. Each entry in the composition's elements list is
keyed by a UUID, repeated in the element's own id field. **Packaging replaces
both with a fresh UUID on every build.** The platform remembers element ids and
refuses one it has seen — and rows survive a failed deploy, and sometimes a
teardown. A package shipping the committed id could be onboarded once per
environment and never again, failing with a duplicate-element error that points
nowhere near the cause. So: rebuild rather than re-upload an old CSAR.

## Validation

```bash
rappctl validate dist/my-rapp-1.0.0.csar
```

| Rule | Checks |
| --- | --- |
| `package-name` | the file ends in `.csar` |
| `required-files` | the TOSCA metadata and the composition definition are present |
| `tosca-entry-definitions` | the entry definitions key exists and resolves |
| `asd-parse` / `asd-structure` / `asd-descriptor` | the descriptor parses, has the node, carries both identifiers |
| `deployment-items` | at least one artifact, correctly typed, with a target URI |
| `artifact-files` | every artifact file is present; unreferenced charts warn |
| `resource-names` | one dot per name, no two resolving to the same identifier |
| `acm-instance` | every chart an instance asks for is shipped, name and version |
| `dme-info-types` | every information type named exists as a file |
| `sme-provider` | no commas in the provider function info |
| `sme-service-api` | no resource at the root path or at a templated path |
| `chart-repository` | (opt-in) the version is not already published |

The last three exist because the platform accepts the package and fails later,
somewhere unhelpful:

- A **comma in the provider function info** becomes a gateway tag, and the
  gateway refuses commas → 502 "Unable to deploy SME".
- A service API published at the **root path**, or at one carrying a templated
  segment, is not a route the gateway can build → same 502.
- A **mistyped information type** is not rejected at all → the instance sits in
  DEPLOYING for ever.

Packaging runs the same checks on its own output.

## Republishing a chart

Priming uploads the packaged charts. If the repository already holds that chart
**name and version**, it keeps the copy it has — so the *old* chart deploys,
with the old values and secrets, however carefully you rebuilt. Nothing reports
it.

Bump the version for every content change, or delete the published copy:

```bash
curl -X DELETE {chartmuseum}/api/charts/{name}/{version}
```

Packaging with the chart check enabled asks the repository and warns. It
reaches the network, so it is opt-in.

## Built packages are sensitive

Real values are baked into the chart before packaging, so a built package and
the chart archive inside it contain live credentials. Never commit one, never
publish one, never attach one to an issue. The scaffold's ignore rules cover
them.

## Onboarding

### Before the first one

The Kubernetes participant only installs from chart repositories it has been
told to trust. Otherwise priming fails with **"Helm Repository not permitted"**:

```bash
kubectl get configmap onap-policy-clamp-ac-k8s-ppnt-configmap -n onap -o yaml
```

The repository URI you upload to must appear there. One-off per environment.

### Four calls

```bash
NAME=cell-watch; VER=1.0.0

curl -sf -X POST {rm}/rapps/$NAME -F "file=@dist/$NAME-$VER.csar"

curl -sf -X PUT {rm}/rapps/$NAME -H 'Content-Type: application/json' \
     -d '{"primeOrder":"PRIME"}'
until curl -sf {rm}/rapps/$NAME | grep -q '"state":"PRIMED"'; do sleep 5; done

INSTANCE=$(curl -sf -X POST {rm}/rapps/$NAME/instance \
  -H 'Content-Type: application/json' -d '{
    "acm": { "instance": "cell-watch-instance" },
    "dme": { "infoTypesProducer": ["cell-watch-state"],
             "infoProducer": "cell-watch-producer" },
    "sme": { "serviceApis": "cell-watch-api",
             "providerFunction": "cell-watch-provider" }
  }' | jq -r .rappInstanceId)

curl -sf -X PUT {rm}/rapps/$NAME/instance/$INSTANCE \
     -H 'Content-Type: application/json' -d '{"deployOrder":"DEPLOY"}'
until curl -sf {rm}/rapps/$NAME/instance/$INSTANCE | grep -q '"state":"DEPLOYED"'
do sleep 5; done
```

**Address the rApp by name, never by id.** Upload returns a UUID, but every path
takes the **name**; the UUID gives a 404 that reads like the rApp was never
onboarded.

Each resource name in the instance body is a file's base name from the
package's files section.

An instance stuck in DEPLOYING is almost always a name that does not match.

## Removing

Strict order. Each step refuses if the one before it has not happened.

```bash
curl -sf -X PUT {rm}/rapps/$NAME/instance/$INSTANCE \
     -H 'Content-Type: application/json' -d '{"deployOrder":"UNDEPLOY"}'
until curl -sf {rm}/rapps/$NAME/instance/$INSTANCE | grep -q '"state":"UNDEPLOYED"'
do sleep 5; done

curl -sf -X DELETE {rm}/rapps/$NAME/instance/$INSTANCE

curl -sf -X PUT {rm}/rapps/$NAME -H 'Content-Type: application/json' \
     -d '{"primeOrder":"DEPRIME"}'
until curl -sf {rm}/rapps/$NAME | grep -q '"state":"COMMISSIONED"'; do sleep 5; done

curl -sf -X DELETE {rm}/rapps/$NAME
```

Again: name, not the UUID.

### Teardown leaves the exposure layer behind

Depriming does not remove what was published to CAPIF or the gateway routes
created for it. The **next** deploy then fails with 502 "Unable to deploy SME",
saying nothing about the leftovers. After removing an rApp that published
service APIs:

```bash
curl -s {kong-admin}/services | jq -r '.data[].name' | grep '^api_id_'
curl -s -X DELETE {kong-admin}/services/<service>/routes/<route>
curl -s -X DELETE {kong-admin}/services/<service>

curl -s -X DELETE {capif}/published-apis/v1/{apfId}/service-apis/{apiId}
```
