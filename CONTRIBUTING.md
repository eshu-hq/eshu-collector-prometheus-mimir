# Contributing

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
also enforces the statement-coverage floor (`COVERAGE_FLOOR`, at 81.9%
and may only rise toward the 90% finish line). `make actionlint` mirrors
the actionlint lane.

## CI lanes

`.github/workflows/ci.yml` runs a `changes` job first, so a PR only pays
for lanes its paths can affect: Go sources run `lint`, `test`, `race`,
`boundary`, `govulncheck`, and `gosec`; fixtures and embedded config
(`testdata/**`, JSON, `manifest.yaml`, `grant.example.json`) run `test`
only; shell scripts run `shell` (vacuous until the first script lands);
dependency changes run `govulncheck`; workflow changes run `actionlint`
plus the Go lanes. `ci-complete` is the only required check: it fails
when a needed lane fails and passes when lanes were legitimately skipped
(docs-only PRs).

Weekly, `govulncheck` and `gosec` (SARIF to the Security tab, advisory
until its false-positive rate is measured) run on a schedule and file an
issue on findings. Dependabot proposes weekly grouped `gomod` and
`github-actions` bumps.

The manifest keeps its all-zero placeholder digest on purpose: it is
`go:embed`'d into the binary, so a committed real digest would belong to
an older build. `TestPlaceholderImageGate` fails loudly if one is
committed; the release workflow stamps the deployed digest when it lands.
