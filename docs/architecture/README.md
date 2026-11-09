# Architecture

Prometheus and Mimir speak nearly the same dialect, so one collector handles both. It reads scrape targets where they exist and rules everywhere, then reports the shape of things. Samples, queries, and label values stay home.

## The lay of the land

See [Diagrams](../diagrams/README.md) for the picture.

Text version: provider API into this collector, collector into the extension host, host into Eshu core. The host hands over one claim, validates the result, and owns claims and commits. Core owns admission, projection, and persistence. This collector minds the provider conversation and the translation into facts.

## One claim's journey

See [Diagrams](../diagrams/README.md) for the picture.

Without the diagram: the claim names a scope, config resolves that scope to one target, and credentials resolve from the environment before any network call. Prometheus targets get asked for active scrape targets. Mimir targets skip that read entirely, because Mimir exposes no scrape-target state and asking would only earn an error. Both providers then get asked for rules. A failed claim emits no facts at all, so you never have to wonder whether a result is half one thing and half another.

## Facts you'll receive

This table mirrors the `emittedFacts` in `manifest.yaml`. Keep them in step: update both, or neither. Every kind carries schema `1.0.0` and confidence `reported`, meaning the provider reported it directly.

| Fact kind | One per… | Provider read behind it |
| --- | --- | --- |
| `observability.source_instance` | configured metric target | your config entry |
| `observability.observed_target` | live scrape target, Prometheus only | `GET /api/v1/targets?state=active` |
| `observability.observed_rule` | live recording or alerting rule | `GET /api/v1/rules` |
| `observability.coverage_warning` | gap worth knowing about | any read above that hit a wall |

Warnings cover hidden data, unsupported endpoints, throttling, truncation, and stale evidence. You get facts plus warnings for those, because most of the picture still beats none of it.

The redaction line is firm. Metric samples, raw PromQL, scrape URLs, label values, annotations, tenant IDs, and token values never enter a fact. Where identity matters, facts carry SHA-256 fingerprints and presence flags instead, so Eshu can match things up without stockpiling sensitive strings.

## Lines it draws

This collector never writes to Prometheus or Mimir and never stores samples. It holds no Postgres, graph, queue, lease, reducer, API, or MCP handle, and it never imports Eshu core internals. `boundary_test.go` enforces that boundary on every test run. MCP, by the way, is a separate Eshu surface entirely, so don't look for query tools here. The collector doesn't build the durable envelope or commit facts either. And it never asks Mimir for scrape targets. That last one sounds small until you've debugged a collector that did.

Parity evidence lives in `testdata/8864-parity-prometheus-mimir.json` alongside `parity_test.go` and `parity_signals_test.go`, if you ever need to prove this extraction still behaves like the producer it came from. Regenerate fixtures only from their original source, never from this package's own output. That way lies self-fulfilling tests.
