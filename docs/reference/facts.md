# Facts reference

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

## SDK pins

Exact pins from `go.mod`, with no `replace` directives and no `go.work`. The file wins if this page ever lags behind.

| Module | Pin |
| --- | --- |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.2.0-rc.2` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.12` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

Go version in `go.mod`: `1.26.10`.

The manifest ([manifest.yaml](../../manifest.yaml)) stays the source of truth for this list. If a fact kind ever changes, the manifest and this page change together.
