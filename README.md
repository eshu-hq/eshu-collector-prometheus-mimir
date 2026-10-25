# eshu-collector-prometheus-mimir

Eshu Prometheus-Mimir collector, extracted from the Eshu core repository.

**Pre-release. Holds no producer authority until its reviewed cutover
(eshu-hq/eshu#8864, part of epic #6707).**

The collector runs through the Eshu component-extension host with the
`process` runner. The host writes one bounded `collector-sdk/v1alpha1` request
to stdin and reads one result from stdout. The collector gets no Postgres,
graph, queue, lease, reducer, API, or MCP handle, and it never imports
`github.com/eshu-hq/eshu/go/internal/...` (`boundary_test.go` enforces this).
The host builds the core fact envelope, validates the result, and owns claims
and commits.

## What it collects

For the metric target named by the claim it reads, through bounded REST calls:

- `GET /api/v1/targets?state=active` (Prometheus only; Mimir exposes no
  scrape-target state and is never asked),
- `GET /api/v1/rules` (both providers).

It emits the four core `observability.*` kinds at schema `1.0.0`:
`source_instance`, `observed_target`, `observed_rule`, and
`coverage_warning`. Metric samples, raw PromQL, scrape URLs, label values,
annotations, tenant IDs, and token values never leave the provider response:
facts carry presence flags and SHA-256 fingerprints instead.

Result states: `complete`; `partial` when an endpoint the target hid or does
not support became warning facts; `retryable` for rate limits, transport
errors, and 5xx; `terminal` for permission, bad-data, invalid configuration,
stale-claim, grant-denied, and collector-kind failures. A failed claim emits
no facts.

## Pins

Exact published SDK pair from
[SDK compatibility](https://github.com/eshu-hq/eshu/blob/main/docs/public/extend/sdk-compatibility.md):

- `github.com/eshu-hq/eshu-sdk/collector@v0.1.0-rc.2`
- `github.com/eshu-hq/eshu-sdk/factschema@v0.1.0-rc.2`

No `replace` directives and no `go.work`. Verify with
`GOPROXY=proxy.golang.org go list -m <module>@<version>` for each pin.

## Parity evidence

`testdata/8864-parity-prometheus-mimir.json` is the cutover fixture pack
copied verbatim from the in-tree collector (cut from a live HTTP run with
fixed generations). `parity_test.go` replays three live `httptest` scopes
against the extracted producer and compares scope by scope on scope ID,
generation ID, fact-kind multiset, stable keys, and payloads (clock-owned
`observed_at` excluded). `parity_signals_test.go` pins truncation,
permission-hidden partial reads, retryable transport failure with bounded
retries, stale-claim rejection, redaction, collector-kind rejection, and the
fail-closed grant stub. Run `go test ./...` and `go vet ./...`.

## Operator signals

Span and counter names preserve the in-tree vocabulary verbatim
(`telemetry.go`): `prometheus_mimir.observe`, `prometheus_mimir.fetch`,
`eshu_dp_prometheus_mimir_fetch_duration_seconds`,
`eshu_dp_prometheus_mimir_provider_requests_total`,
`eshu_dp_prometheus_mimir_facts_emitted_total`,
`eshu_dp_prometheus_mimir_rate_limited_total`,
`eshu_dp_prometheus_mimir_retries_total`,
`eshu_dp_prometheus_mimir_redactions_total`, and
`eshu_dp_prometheus_mimir_stale_total`. Labels stay bounded to `provider`,
`status_class`, `fact_kind`, and `reason`. The extension host records grant
decisions per emission (#6709).

## Producer grants, revocation, and rollback

Core-owned kinds need one live #6709 grant per kind naming this producer,
version `0.1.0`, schema `1.0.0`, and scope `prometheus_mimir`
(`grant.example.json`, locked by `manifest_grant_test.go`). The handler ships
a fail-closed stub (`grant.go`): with an authorizer attached, a missing grant
fails the claim terminal with no facts; without one it preserves the in-tree
first-party behavior for parity and replay. The host enforces the live
registry at install, activation, and every emission. Revocation and rollback
follow the in-tree rehearsal plan: stop new claims, drain leases, revoke the
four grants, restore the in-tree producer, replay from the last committed
generation, and verify no dual authority and no lost facts.
