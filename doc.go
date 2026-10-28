// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

// Package collector is the Eshu Prometheus-Mimir collector, extracted from
// the Eshu core repository (eshu-hq/eshu#8864, part of epic #6707).
//
// It runs through the Eshu component-extension host with the process runner:
// the host writes one bounded collector-sdk/v1alpha1 request to stdin and
// reads one result from stdout. A claim names one metric source scope; the
// collector reads bounded Prometheus-compatible metadata (targets on
// Prometheus only, rules on both providers) and emits the four core
// observability kinds at schema 1.0.0. It is pre-release and holds no
// producer authority until its reviewed cutover.
//
// Boundary: this package imports only the published eshu-sdk modules
// (github.com/eshu-hq/eshu-sdk/collector,
// github.com/eshu-hq/eshu-sdk/factschema), the standard library, and its own
// internal helpers. It never imports go/internal, another collector
// implementation, or any core Postgres, graph, queue, lease, reducer, API,
// MCP, or workflow handle; the import-boundary test enforces this. The host,
// not this package, builds the core fact envelope, validates the result, and
// owns claims and commits.
//
// Evidence is metadata-only: metric samples, raw PromQL, scrape URLs, label
// values, annotations, tenant IDs, and token values never leave the provider
// response. Facts carry presence flags and SHA-256 fingerprints instead.
// Credentials resolve from environment variables named in the activation
// config and never enter facts, source URIs, logs, or statuses.
package collector

// Seed sweep complete: every seed removed, lanes must run green.
