# Contributing to eshu-collector-prometheus-mimir

Welcome — thanks for helping. The short version: run the local checks before you push, keep credentials out of code and logs, and never weaken a test to make a change land.

```bash
make ci          # lint + test + race
make lint        # vet, golangci-lint, format diff, tidy diff, 500-line cap
make test        # go build ./... and go test ./... -count=1
make race        # race tests plus the coverage floor (94.7%)
make actionlint  # workflow lint
docker build -t eshu-collector-prometheus-mimir:local .
```

The root [CONTRIBUTING.md](../../CONTRIBUTING.md) stays the authority for CI lanes and floors. This page mirrors it so the docs tree is self-contained; if the two ever disagree, believe the root file and fix this page.

The test suite is the product contract: the parity, boundary, manifest, and
collector smoke tests must stay green. Never weaken them to land a change.

Credentials travel by env reference, never inline. The handler resolves
token and tenant variables through its env seam; tests inject a stub.
Never log a token, tenant ID, URL, or provider body.

## Local checks

You need Go (see `go.mod`) and `golangci-lint` v2.12.2.

```sh
make ci   # lint + test + race, the whole local contract
```

`make lint` runs `go vet`, whole-repo `golangci-lint run` (config in
`.golangci.yml`, org lint set; no baselines), the gofumpt format check,
`go mod tidy -diff`, and the 500-line cap on `.go` files. `make race`
also enforces the statement-coverage floor (`COVERAGE_FLOOR`, at 94.7%,
past the 90% finish line; it may only rise). `make actionlint` mirrors
the actionlint lane.

## CI lanes

`.github/workflows/ci.yml` is a thin caller: a `changes` job first, so a
PR only pays for lanes its paths can affect, then a `lanes` job that runs
the lane bodies inside the pinned `eshu-hq/.github` reusable
(go-collector-ci.yml at an exact SHA), then the `ci-complete` aggregator,
the only required check. Go sources run `lint`, `test`, `race`,
`boundary`, `govulncheck`, and `gosec`; fixtures and embedded config
(`testdata/**`, JSON, `manifest.yaml`, `grant.example.json`) run `test`
only; shell scripts run `shell` (vacuous until the first script lands);
`Dockerfile` and `.dockerignore` changes run `image`, a no-push build that
also proves the non-root user and digest-pinned bases; dependency changes
run `govulncheck`; workflow changes run `actionlint`
plus the Go lanes.

Weekly, `govulncheck` and `gosec` run on a schedule and fail red on
findings; triage rides the run page and the uploaded SARIF artifact.
Neither lane uploads to the Security tab nor files an issue: the reusable
runs on `contents: read` (fork safety), so per-scan filing waits on the
upstream hook (eshu-hq/.github#10). Dependabot proposes weekly grouped
`gomod` and `github-actions` bumps.

The manifest keeps its all-zero placeholder digest on purpose: it is
`go:embed`'d into the binary, so a committed real digest would belong to
an older build. `TestPlaceholderImageGate` fails loudly if one is
committed; the release workflow stamps the deployed digest when it lands.
