# Configuring the Prometheus-Mimir collector

The host reads your activation file. `host` and `process` configure the host itself, while everything under `config` comes to this collector. [config.example.yaml](../../config.example.yaml) shows one Prometheus target and one Mimir target side by side with placeholder values, which makes it a handy starting point even if you only run one provider.

A word on identity before you fill anything in. Scope and instance IDs are baked into every stable fact key. If this activation replaces an existing metric feed, keep `targets[].scope_id`, `targets[].instance_id`, and `host.sourceSystem` (`prometheus_mimir`) equal to that feed's values, exactly as the example warns. Fresh values make Eshu treat the same feed as a new scope, and untangling that later is nobody's idea of fun.

## Target keys

| Key | What you'll want to know |
| --- | --- |
| `targets` | Required, with at least one enabled target. Unknown keys fail closed, so typos stop the show early. |
| `targets[].provider` | Required: `prometheus` or `mimir`. This decides whether the targets read happens at all. |
| `targets[].scope_id` | Required. The claim scope selects the target. Duplicate scope IDs fail, thankfully. |
| `targets[].instance_id` | Required. Recorded as the fact source instance. |
| `targets[].base_url` | Required. The provider API origin. |
| `targets[].path_prefix` | Optional, for providers mounted below a path. |
| `targets[].token_env` | Optional. Names the environment variable holding the Bearer token. Leave it out for credential-free targets, and never inline a token in the file. |
| `targets[].tenant_id` | Optional literal tenant ID. |
| `targets[].tenant_id_env` | Optional. Names the environment variable holding the tenant ID, and wins over `tenant_id`. |
| `targets[].resource_limit` | Bound on records read per target. The example uses `100`, and `0` selects the code default. |
| `targets[].stale_after` | Freshness window. The Prometheus example uses `1h`. |
| `targets[].enabled` | Disabled targets drop out before any client is built. Their claims then fail like an unconfigured scope, so don't be surprised by that error after disabling one. |
| `targets[].declared_ids` | Declared rule IDs used as comparison hints. The example lists one, `parity.rules:ParityBurn`. |
| `targets[].observed_only_hint` | Companion hint for declared-vs-observed comparison, which core owns. |

Here's a trap worth flagging in friendly terms: there's no `limits` stanza and no `max_result_bytes` key in this collector's config. Sibling collectors have them, which makes copy-paste tempting, but unknown keys get rejected here. The host's `process.stdout_limit_bytes` (64 MiB in the example) is the size guard you actually control.

## Credentials

Secrets travel by environment-variable name only. `token_env` and `tenant_id_env` name variables the collector process reads at claim time. Name a variable that resolves to nothing and the claim fails closed with an error that names the variable, never a value. Resolved credentials stay out of facts, source URIs, logs, and statuses.

Give the credential only the read access the two GET endpoints need. This repo doesn't name Prometheus or Mimir auth scopes or roles, because the code doesn't define them. Plenty of in-cluster targets need no token at all, and that's fine. Just omit `token_env`.

## Grants

All four kinds are core-owned, so the host needs a live producer grant (core's permission to emit a kind it owns) per kind before it accepts these facts. [grant.example.json](../../grant.example.json) lists the four: scope `prometheus_mimir`, version `0.1.1`, schema `1.0.0`. A test, `manifest_grant_test.go`, keeps the manifest and that example locked together so they can't drift apart quietly.

Inside the collector, the handler carries a fail-closed grant stub (see `grant.go`). With an authorizer attached, a missing grant fails the claim terminal as grant-denied with no facts. The real enforcement lives with the host, which checks the live grant registry at install, at activation, and again at emission. Think of the stub as a seatbelt, not the brakes.
