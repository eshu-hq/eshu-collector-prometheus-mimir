// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

// Telemetry vocabulary preserved verbatim from the in-tree producer (see
// SHIM_MAP.md section 4). The host owns emission; these constants keep the
// external producer, dashboards, and the cutover gate on one vocabulary.
const (
	// SpanObserve wraps one claimed Prometheus/Mimir observation.
	SpanObserve = "prometheus_mimir.observe"
	// SpanFetch wraps one bounded Prometheus-compatible REST fetch.
	SpanFetch = "prometheus_mimir.fetch"
	// MetricFetchDuration is the fetch-duration histogram.
	MetricFetchDuration = "eshu_dp_prometheus_mimir_fetch_duration_seconds"
	// MetricProviderRequests counts bounded provider requests.
	MetricProviderRequests = "eshu_dp_prometheus_mimir_provider_requests_total"
	// MetricFactsEmitted counts emitted facts by fact kind.
	MetricFactsEmitted = "eshu_dp_prometheus_mimir_facts_emitted_total"
	// MetricRateLimited counts rate-limited provider responses.
	MetricRateLimited = "eshu_dp_prometheus_mimir_rate_limited_total"
	// MetricRetries counts bounded HTTP retries.
	MetricRetries = "eshu_dp_prometheus_mimir_retries_total"
	// MetricRedactions counts redacted fields.
	// #nosec G101 -- metric-name constant, not a credential
	MetricRedactions = "eshu_dp_prometheus_mimir_redactions_total"
	// MetricStale counts stale observations.
	MetricStale = "eshu_dp_prometheus_mimir_stale_total"
)

// MetricLabels lists the only dimensions facts and statuses may carry:
// provider, status class, fact kind, and reason. Instance IDs, target URLs,
// label values, raw PromQL, tenant IDs, tenant headers, and token values must
// never appear in metric labels.
const (
	LabelProvider    = "provider"
	LabelStatusClass = "status_class"
	LabelFactKind    = "fact_kind"
	LabelReason      = "reason"
)
