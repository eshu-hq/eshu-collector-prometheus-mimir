# Getting started

The extension host normally runs this binary. Two smoke runs and one activation get you going.

## 1. Smoke-test it

```bash
docker run -i IMAGE < request.json
go run ./cmd/collector < request.json > result.json
```

One request in, one result out — the whole serving story.

## 2. Know the seed quirk before you deploy

This binary has **no `seed` subcommand**, unlike its siblings. It accepts only the stdio claim mode and rejects extra arguments. How the binary and `manifest.yaml` reach the component home belongs to the deployment side. Read the [operations guide](../guides/operations.md) before copying a sibling's setup.

## 3. Write the activation

[config.example.yaml](../../config.example.yaml) shows one Prometheus target and one Mimir target side by side. Set each target's `provider` (`prometheus` reads active scrape targets too; `mimir` reads rules only), keep `host.sourceSystem: prometheus_mimir`, and name secrets by environment variable only. Every key is in [Configuration](../reference/configuration.md) — note there is no `limits` stanza in this collector, whatever a sibling example suggests.

## 4. Grants

All four kinds are core-owned, so the host needs a live producer grant per kind. [grant.example.json](../../grant.example.json) lists them; [Configuration](../reference/configuration.md) explains the stub-vs-host split.

## Where next

- How targets and rules become facts: [Architecture](../architecture/README.md).
- Result states and trouble: the [operations guide](../guides/operations.md).
