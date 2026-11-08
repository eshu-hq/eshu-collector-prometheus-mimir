# eshu-collector-prometheus-mimir

Status: pre-release. This collector is not yet the live Prometheus/Mimir producer for Eshu; Eshu core still owns admission and storage.

This collector reads live Prometheus and Grafana Mimir metric metadata for Eshu and returns it as facts (small, checked records Eshu stores as evidence).

## What it collects

For each work request, the collector reads the one metric target the claim names, through bounded REST calls, and emits metadata only. It never stores metric samples.

`manifest.yaml` declares 4 fact kinds. Each kind uses schema version `1.0.0` and source confidence `reported` (the provider itself reported the value).

| Fact kind | What it records | Provider read behind it |
| --- | --- | --- |
| `observability.source_instance` | One configured metric target | The configured target itself |
| `observability.observed_target` | One live scrape target (Prometheus only) | `GET /api/v1/targets?state=active` |
| `observability.observed_rule` | One live recording or alerting rule | `GET /api/v1/rules` |
| `observability.coverage_warning` | A gap: hidden, unsupported, throttled, truncated, or stale data | Any read above that could not finish cleanly |

Mimir exposes no scrape-target state, so Mimir targets are never asked for `/api/v1/targets`; rules are read on both providers.

Metric samples, raw PromQL, scrape URLs, label values, annotations, tenant IDs, and token values never enter a fact. Facts carry presence flags and SHA-256 fingerprints (short hashes) instead.

## How data flows

```mermaid
flowchart LR
    A[Prometheus or Mimir API] --> B[this collector]
    B --> C[Eshu extension host]
    C --> D[Eshu core]
```

Text version: Prometheus or Mimir API → this collector → Eshu extension host → Eshu core. The host hands the collector one claim (one bounded unit of work), validates the result, and owns claims and commits. Core owns admission, projection, and persistence.

## When to use it

Use it when Eshu needs live evidence about which scrape targets and rules a Prometheus or Mimir target actually serves.

Do not use it to query metric values, run PromQL for people, or replace Prometheus or Mimir. It reads target and rule metadata only and writes nothing to either provider.

## How Eshu runs it

The Eshu component-extension host runs this program with its `process` runner:

- The host writes one bounded `collector-sdk/v1alpha1` Request to standard input (stdin).
- The collector reads the claimed target and writes one `collector-sdk/v1alpha1` Result to standard output (stdout).
- With no arguments, or with `--sdk-stdio`, the binary serves one claim this way. That is the only serving mode.

Local smoke run with a recorded request:

```bash
docker run -i IMAGE < request.json
go run ./cmd/collector < request.json > result.json
```

### Image — and no seed subcommand in this repo

The image defined by `Dockerfile` holds one static binary at `/usr/local/bin/collector` on a distroless base (no shell), running as user `65532:65532`, with entrypoint `/usr/local/bin/collector --sdk-stdio`.

Unlike some sibling collectors, this binary has **no `seed` subcommand**: `cmd/collector/main.go` accepts only the stdio claim mode and rejects extra arguments. Do not copy a `collector seed --dest ...` step from another collector's README into a deployment for this one. How the binary and `manifest.yaml` reach the component home is owned by the deployment side, not by a command in this repo. The image-publish workflow notes that a person copies the published digest into the activation's `seed.image`; that `seed.image` key belongs to the host/deployment config, not to a subcommand here.

Manifest identity: `dev.eshu.collector.prometheus-mimir`, version `0.1.0`, collector kind `prometheus_mimir`, platform `linux/amd64`, compatible core `>=0.0.5 <0.2.0`. The manifest artifact image is the all-zero placeholder digest; use the digest from the publish job summary for a real deployment.

### Grants

All 4 kinds are core-owned, so the host needs a live producer grant (permission to emit a core-owned kind) per kind before accepting them. `grant.example.json` lists the 4 grants (scope `prometheus_mimir`, version `0.1.0`, schema `1.0.0`), and `manifest_grant_test.go` locks manifest and grant example together.

The handler carries a fail-closed grant stub (`grant.go`): with an authorizer attached, a missing grant fails the claim terminal as grant-denied with no facts. The host enforces the live grant registry at install, activation, and emission.

### Stable identity when replacing an older collector

Scope and instance IDs are part of every stable fact key. If this activation replaces an existing metric feed, keep `targets[].scope_id`, `targets[].instance_id`, and `host.sourceSystem` (`prometheus_mimir`) equal to that feed's values, as `config.example.yaml` warns. New values make Eshu ingest the same feed as a new scope.

## Configuration

The host reads an activation file. `host` and `process` configure the host; everything under `config` reaches this collector. `config.example.yaml` shows one Prometheus target and one Mimir target with placeholder values and no secrets.

| Key | Meaning |
| --- | --- |
| `targets` | Required, at least one enabled target. Unknown keys fail closed. |
| `targets[].provider` | Required: `prometheus` or `mimir`. |
| `targets[].scope_id` | Required. The claim scope selects the target; duplicate scope IDs fail. |
| `targets[].instance_id` | Required. Recorded as the fact source instance. |
| `targets[].base_url` | Required. Provider API origin. |
| `targets[].path_prefix` | Optional path prefix. |
| `targets[].token_env` | Optional. **Name** of the environment variable holding the Bearer token. Omit for credential-free targets. |
| `targets[].tenant_id` | Optional literal tenant ID. |
| `targets[].tenant_id_env` | Optional. **Name** of the environment variable holding the tenant ID; wins over `tenant_id`. |
| `targets[].resource_limit` | Bound on records read per target (example: `100`; `0` selects the code default). |
| `targets[].stale_after` | Freshness window (example: `1h`). |
| `targets[].enabled` | Disabled targets are dropped before any client is built; their claims fail like an unconfigured scope. |
| `targets[].declared_ids` | Declared rule IDs for comparison hints (example lists one). |
| `targets[].observed_only_hint` | Hint for declared-vs-observed comparison, which core owns. |

There is no `limits` stanza and no `max_result_bytes` key in this collector's config. Do not add one from another collector's example; unknown keys are rejected.

### Credentials

Secrets travel by environment-variable name only: `token_env` and `tenant_id_env` name variables the collector process reads at claim time. A named-but-unset variable fails closed with an error that names the variable, never a value. Resolved credentials never enter facts, source URIs, logs, or statuses.

Give the credential only the read access the two `GET` endpoints above need. This repo does not name Prometheus or Mimir auth scopes or roles, because the code does not define them.

## Claim budget and operator signals

Result states an operator will see:

| State | Meaning |
| --- | --- |
| `complete` | Every read for the claimed target finished on its own. |
| `partial` | Facts plus coverage warnings for endpoints the target hid or does not support. |
| `retryable` | Try again later: rate limit, transport error, `5xx`, or stale claim. |
| `terminal` | Stop: permission failure, bad data (`bad_data` API status), invalid config, grant denied, or wrong collector kind. |

A failed claim emits no facts. Statuses carry a bounded failure class and counts only — never a provider body, token, tenant ID, or scrape URL.

Telemetry names are preserved in `telemetry.go` as constants so the host, dashboards, and this collector share one vocabulary: spans `prometheus_mimir.observe` and `prometheus_mimir.fetch`, and counters under `eshu_dp_prometheus_mimir_` (the prefix the manifest declares), such as `eshu_dp_prometheus_mimir_facts_emitted_total` and `eshu_dp_prometheus_mimir_rate_limited_total`. The host owns emission of those signals; labels stay bounded to `provider`, `status_class`, `fact_kind`, and `reason`. This README does not claim a metrics endpoint on the binary, because `Dockerfile` and `main.go` define none.

## Eshu dependencies

Exact pins from `go.mod` in this repo. No `replace` directives and no `go.work`.

| Module | Pin |
| --- | --- |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.2.0-rc.2` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.12` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

Go version in `go.mod`: `1.26.10`.

## What it never does

- It never writes to Prometheus or Mimir and never stores samples.
- It has no Postgres, graph, queue, lease, reducer, API, or MCP handle, and never imports Eshu core internals (`boundary_test.go` enforces this). MCP is a separate Eshu surface; this collector defines no MCP or query tools.
- It never builds the durable fact envelope or commits facts; the host does.
- It never asks Mimir for scrape targets.
- It never emits kinds outside the 4 in `manifest.yaml`.

Parity evidence lives in `testdata/8864-parity-prometheus-mimir.json` with `parity_test.go` and `parity_signals_test.go`. Regenerate fixtures only from the source they were cut from, never from this package's own output.

## Build, test, and release

```bash
make ci          # lint + test + race
make lint        # vet, golangci-lint, format diff, tidy diff, 500-line cap
make test        # go build ./... and go test ./... -count=1
make race        # race tests plus coverage floor (94.7%)
make actionlint  # workflow lint
docker build -t eshu-collector-prometheus-mimir:local .
```

CI (`.github/workflows/ci.yml`) is the authority; `make ci` mirrors it locally. See [CONTRIBUTING.md](CONTRIBUTING.md) for the local checks contract.

Images publish from `.github/workflows/image-publish.yml` (this repo has no `release.yml`) on pushes to `main` and on version tags. Nothing publishes from a pull request and the workflow deploys nothing. A `main` push publishes moving `main`, `latest`, and `sha-<long-sha>` tags to GHCR under the repository name; a version tag must be `vMAJOR.MINOR.PATCH[-prerelease]`, match the manifest version check in the workflow, and point at the protected `main` head. Copy the digest from the job summary into the activation; the workflow signs and attests the image as its steps define — read the workflow for the exact current steps before relying on them.

## Docs and related repos

- [eshu](https://github.com/eshu-hq/eshu) — Eshu core and the extension host that runs this collector.
- [eshu-sdk](https://github.com/eshu-hq/eshu-sdk) — the collector and fact-schema modules pinned above.
- [eshu-deploy](https://github.com/eshu-hq/eshu-deploy) — deployment for Eshu components.

Local files: [config.example.yaml](config.example.yaml), [grant.example.json](grant.example.json), [manifest.yaml](manifest.yaml), [Dockerfile](Dockerfile), and [CONTRIBUTING.md](CONTRIBUTING.md).

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md). License: MIT — see [LICENSE](LICENSE). This repo has no `SECURITY.md` file.
