// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

const (
	// ComponentID is the manifest identity of this collector. Keep it in sync
	// with metadata.id in manifest.yaml.
	ComponentID = "dev.eshu.collector.prometheus-mimir"
	// CollectorKind is the durable collector family identifier for live
	// Prometheus-compatible metric metadata.
	CollectorKind = "prometheus_mimir"
	// SourceSystem is the source system name facts carry when the claim does
	// not name one. Hosted claims carry the activation's source system.
	SourceSystem = "prometheus_mimir"
	// MetricsPrefix is the component-owned metric prefix the manifest declares.
	// Span and counter names below preserve the in-tree
	// eshu_dp_prometheus_mimir_* vocabulary verbatim; see telemetry.go and the
	// README section "Operator signals".
	MetricsPrefix = "eshu_dp_prometheus_mimir_"
	// SchemaVersion is the schema version of every emitted observability fact.
	SchemaVersion = "1.0.0"
	// GrantVersion is the collector artifact version the producer grants bind.
	GrantVersion = "0.1.0"
	// ProviderPrometheus identifies Prometheus API evidence.
	ProviderPrometheus = "prometheus"
	// ProviderMimir identifies Grafana Mimir API evidence.
	ProviderMimir = "mimir"
	// ScopeKindMetricSource identifies one configured metric API target.
	ScopeKindMetricSource = "metric_source"
	// RedactionVersion is the metadata-only redaction policy. It travels in
	// every payload and as the reason on every SDK redaction entry.
	RedactionVersion = "observability-live-prometheus-mimir-v1"
)

// Core observability fact kinds. They are core-owned: the host requires a
// producer grant for each before accepting them from this component.
const (
	// FactKindSourceInstance identifies one bounded metric source observation.
	FactKindSourceInstance = "observability.source_instance"
	// FactKindObservedTarget identifies one live scrape target observation.
	FactKindObservedTarget = "observability.observed_target"
	// FactKindObservedRule identifies one live rule observation.
	FactKindObservedRule = "observability.observed_rule"
	// FactKindCoverageWarning identifies one bounded coverage warning.
	FactKindCoverageWarning = "observability.coverage_warning"
)

// FactKinds returns every fact kind this collector may emit, in the order the
// manifest declares them.
func FactKinds() []string {
	return []string{
		FactKindSourceInstance,
		FactKindObservedTarget,
		FactKindObservedRule,
		FactKindCoverageWarning,
	}
}

// Contract returns the SDK fact families this collector may emit.
func Contract() sdk.Contract {
	kinds := FactKinds()
	facts := make([]sdk.FactDeclaration, 0, len(kinds))
	for _, kind := range kinds {
		facts = append(facts, sdk.FactDeclaration{
			Kind:             kind,
			SchemaVersions:   []string{SchemaVersion},
			SourceConfidence: []sdk.SourceConfidence{sdk.SourceConfidenceReported},
		})
	}
	return sdk.Contract{ProtocolVersion: sdk.ProtocolVersionV1Alpha1, Facts: facts}
}
