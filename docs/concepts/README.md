# Concepts

## What this is

A pre-release collector that asks a Prometheus or Grafana Mimir target a modest question — what are you scraping, and which rules are you running? — and turns the answers into facts for Eshu. Metric samples never leave the provider.

## Why it exists

Eshu wants the shape of your metric surface, not the numbers flowing through it: which scrape targets and recording or alerting rules an instance truly serves right now.

## The one difference to keep straight

Prometheus targets get their active scrape targets read. Mimir targets skip that read entirely, because Mimir does not expose scrape-target state. Both report their rules. If a Mimir activation shows no targets, that is the design talking, not a bug.

## Claims, grants, and redaction

One claim names one metric target; the host validates and commits, and core owns admission and storage. The four kinds are core-owned, so host-side grants gate emission (with a fail-closed stub inside the handler as a seatbelt). Samples, raw PromQL, scrape URLs, label values, annotations, tenant IDs, and token values never enter a fact — SHA-256 fingerprints and presence flags stand in where identity matters.

## Boundaries

It never writes to Prometheus or Mimir, never stores samples, holds no Postgres, graph, queue, lease, reducer, API, or MCP handle, and never imports Eshu core internals. It does not build the durable envelope or commit facts, and it never asks Mimir for scrape targets.
