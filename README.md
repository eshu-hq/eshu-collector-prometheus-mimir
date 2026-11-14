# eshu-collector-prometheus-mimir

Status: pre-release. It isn't the live Prometheus/Mimir producer for Eshu yet, and Eshu core still owns admission and storage.

This collector asks a Prometheus or Grafana Mimir target a modest question: what are you scraping, and which rules are you running? It turns the answers into facts for Eshu. Metric samples never leave the provider, because Eshu wants the shape of your metric surface, not the numbers flowing through it.

```mermaid
flowchart LR
    A["Prometheus or Mimir API"] --> B["This collector"]
    B --> C["Eshu extension host"]
    C --> D["Eshu core"]
```

In words: the provider answers, this collector translates, the host validates, and core stores. One claim (one bounded unit of work) names one metric target, and the collector reads just that target.

## Pick this up if…

Pick it up when Eshu should know which scrape targets and recording or alerting rules a Prometheus or Mimir instance truly serves right now.

Put it back down if you want metric values, ad-hoc PromQL, or a replacement for either system. It reads target and rule metadata only, and it writes nothing to the provider.

## Fastest smoke test

```bash
docker run -i IMAGE < request.json
go run ./cmd/collector < request.json > result.json
```

One request in, one result out. That's the whole serving story. Deployment details, including one quirk about seeding you'll want to know before copying a sibling's setup, live in [docs/guides/operations.md](docs/guides/operations.md).

## Reading map

| Your goal | Go to |
| --- | --- |
| see how targets and rules become facts | [docs/architecture/README.md](docs/architecture/README.md) |
| configure Prometheus and Mimir targets | [docs/reference/configuration.md](docs/reference/configuration.md) |
| ship it, watch it, fix it | [docs/guides/operations.md](docs/guides/operations.md) |

Files you'll open along the way: [config.example.yaml](config.example.yaml), [grant.example.json](grant.example.json), [manifest.yaml](manifest.yaml), and the [Dockerfile](Dockerfile).

## Related repos

- [eshu](https://github.com/eshu-hq/eshu): core and the extension host that runs this collector.
- [eshu-sdk](https://github.com/eshu-hq/eshu-sdk): the collector and fact-schema modules pinned here.
- [eshu-deploy](https://github.com/eshu-hq/eshu-deploy): deployment wiring for Eshu components.

## One difference to keep straight

Prometheus and Mimir share most of this collector, but not all of it. Prometheus targets get their active scrape targets read. Mimir targets skip that read entirely, because Mimir doesn't expose scrape-target state, and both still report their rules. If a Mimir activation shows no targets, that's the design talking, not a bug you need to chase.

Manifest version today is `0.1.1` for collector kind `prometheus_mimir`.
Exact SDK pins and the publish flow sit in [docs/guides/operations.md](docs/guides/operations.md).
If a fact kind ever changes, the manifest and [docs/architecture/README.md](docs/architecture/README.md) change together.

## Developing

Run `make ci` before you push. [docs/guides/operations.md](docs/guides/operations.md) has the full rundown near the end.

## Contributing, security, license

Contributions start at [CONTRIBUTING.md](CONTRIBUTING.md). The license is MIT, see [LICENSE](LICENSE). There's no `SECURITY.md` file in this repo.
