# Operating the Prometheus-Mimir collector

Running this collector is mostly a matter of getting the binary and manifest to the host, then reading result states with confidence. One difference from its siblings catches people out, so we'll put it up front where you can't miss it.

## How the host runs it

The extension host uses its `process` runner. One `collector-sdk/v1alpha1` Request arrives on stdin, the collector reads the target the claim names, and one Result leaves on stdout. No arguments (or `--sdk-stdio`) serves one claim. That's the only serving mode you'll find.

```bash
docker run -i IMAGE < request.json
go run ./cmd/collector < request.json > result.json
```

## The image, and the seed that isn't

The `Dockerfile` builds one static binary at `/usr/local/bin/collector` on a distroless base with no shell, running as user `65532:65532`. The entrypoint is `/usr/local/bin/collector --sdk-stdio`, so the image serves a claim the moment you feed it stdin.

Now the quirk: this binary has no `seed` subcommand. Read `cmd/collector/main.go` and you'll see it accepts only the stdio claim mode and rejects extra arguments outright. Please don't copy a `collector seed --dest ...` step from a sibling collector's deployment, because it will fail here and waste your afternoon. How the binary and `manifest.yaml` reach the component home belongs to the deployment side, not to a command in this repo. The image-publish workflow mentions copying the published digest into an activation's `seed.image` key, and that key is host-side config, not something this binary parses.

Manifest identity: `dev.eshu.collector.prometheus-mimir`, version `0.1.0`, collector kind `prometheus_mimir`, platform `linux/amd64`, compatible core `>=0.0.5 <0.2.0`. The manifest's artifact image carries an all-zero placeholder digest. Use the digest from the publish job summary for anything real.

## Result states, translated

You'll meet four states, and each one suggests a different next move:

- `complete`: every read for the claimed target finished cleanly. Carry on.
- `partial`: facts plus coverage warnings, for endpoints the target hid from you or doesn't support. Useful data, honest caveats.
- `retryable`: try again later. Rate limits, transport errors, `5xx` responses, and stale claims land here.
- `terminal`: stop and fix. Permission failures, `bad_data` responses from the provider API, invalid config, a denied grant, or the wrong collector kind.

A failed claim emits no facts, which keeps your reasoning simple. Statuses carry a bounded failure class and counts only, never a provider body, a token, a tenant ID, or a scrape URL.

Telemetry deserves a precise sentence or two. Names live in `telemetry.go` as constants so the host, dashboards, and this collector share one vocabulary: spans `prometheus_mimir.observe` and `prometheus_mimir.fetch`, plus counters under `eshu_dp_prometheus_mimir_` (the prefix the manifest declares), such as `eshu_dp_prometheus_mimir_facts_emitted_total` and `eshu_dp_prometheus_mimir_rate_limited_total`. The host owns emitting those signals, and labels stay bounded to `provider`, `status_class`, `fact_kind`, and `reason`. The binary itself exposes no metrics endpoint, since neither the `Dockerfile` nor `main.go` defines one. Don't go scraping a port that doesn't exist.

## Troubleshooting

Symptom first, cause second. That's how these usually unfold:

- Everything fails terminal right after a config edit. Check for borrowed keys from sibling examples, especially a `limits` stanza or `max_result_bytes`. Unknown keys fail closed here.
- Prometheus targets look fine but Mimir shows no scrape targets. That's correct behavior, not a bug. Mimir gets rules only, by design.
- A target you disabled now errors like an unconfigured scope. Also by design. Disabled targets never get a client, so their claims have nowhere to land.
- Results keep coming back partial with hidden or unsupported warnings. Look at what the credential can actually see, and confirm the target serves `/api/v1/rules` where you think it does. A path prefix slip shows up exactly like this.
- Grants deny claims at activation time. Compare your grant registry against [grant.example.json](../../grant.example.json) for scope, version, and schema. One stale version string is enough.

When in doubt, run the collector locally against the same config with a recorded request. The result JSON tells you the state and class, and that's usually enough to pick a lane.

## Build, test, and release

```bash
make ci          # lint + test + race
make lint        # vet, golangci-lint, format diff, tidy diff, 500-line cap
make test        # go build ./... and go test ./... -count=1
make race        # race tests plus the coverage floor (94.7%)
make actionlint  # workflow lint
docker build -t eshu-collector-prometheus-mimir:local .
```

CI in `.github/workflows/ci.yml` is the authority, and `make ci` mirrors it locally. [CONTRIBUTING.md](../../CONTRIBUTING.md) spells out the local-checks contract if you want the fine print.

Images publish from `.github/workflows/image-publish.yml`. Note the name, because this repo has no `release.yml`. Pushes to `main` and version tags publish, pull requests publish nothing, and the workflow deploys nothing. A `main` push produces moving `main`, `latest`, and `sha-<long-sha>` tags on GHCR under the repository name. A version tag must look like `vMAJOR.MINOR.PATCH[-prerelease]`, match the manifest version check in the workflow, and point at the protected `main` head. Copy the digest from the job summary into your activation. The workflow signs and attests the image, and you'd do well to read its current steps before your runbook leans on the details.

## SDK pins

Exact pins from `go.mod`, with no `replace` directives and no `go.work`. The file wins if this page ever lags behind.

| Module | Pin |
| --- | --- |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.2.0-rc.2` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.12` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

Go version in `go.mod`: `1.26.10`.
