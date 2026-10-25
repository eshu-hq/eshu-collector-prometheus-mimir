// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"time"
)

// Failure classes are the bounded provider failure labels stored in workflow
// status and telemetry. They mirror the in-tree SDK vocabulary verbatim.
const (
	// FailureAuthDenied marks missing or unauthorized metric credentials.
	FailureAuthDenied = "auth_denied"
	// FailureRateLimited marks provider throttling as retryable.
	FailureRateLimited = "rate_limited"
	// FailureRetryable marks transport or server-side failures as retryable.
	FailureRetryable = "retryable"
	// FailureTerminal marks malformed provider responses or unsupported setup.
	FailureTerminal = "terminal"
	// FailureInvalidConfig marks an activation config the collector cannot run.
	FailureInvalidConfig = "invalid_config"
	// FailureStaleClaim marks a claim for an unconfigured scope.
	FailureStaleClaim = "stale_claim"
	// FailureGrantDenied marks a missing, expired, or revoked producer grant.
	FailureGrantDenied = "grant_denied"
	// FailureCollectorKind marks a claim naming another collector kind.
	FailureCollectorKind = "collector_kind_mismatch"
)

const (
	// ResourceClassTarget marks a Prometheus scrape target observation.
	ResourceClassTarget = "target"
	// ResourceClassRule marks a Prometheus-compatible rule observation.
	ResourceClassRule = "rule"
	// RuleTypeAlerting marks an alerting rule.
	RuleTypeAlerting = "alerting"
	// RuleTypeRecording marks a recording rule.
	RuleTypeRecording = "recording"
)

const (
	// FreshnessCurrent marks a live response observed during the current scan.
	FreshnessCurrent = "current"
	// FreshnessUnknown marks a source-local freshness gap.
	FreshnessUnknown = "unknown"
	// FreshnessStale marks live evidence older than the configured window.
	FreshnessStale = "stale"
	// FreshnessPermissionHidden marks provider data hidden by credentials.
	FreshnessPermissionHidden = "permission_hidden"
)

const (
	// OutcomeObserved marks live provider evidence that was accepted.
	OutcomeObserved = "observed"
	// OutcomePartial marks an incomplete source-local read.
	OutcomePartial = "partial"
	// OutcomePermissionHidden marks evidence hidden by provider permissions.
	OutcomePermissionHidden = "permission_hidden"
	// OutcomeUnsupported marks a provider endpoint outside the supported API.
	OutcomeUnsupported = "unsupported"
	// OutcomeRejected marks malformed or unsafe source data.
	OutcomeRejected = "rejected"
	// OutcomeStale marks accepted evidence outside the freshness window.
	OutcomeStale = "stale"
)

const (
	// MatchStateNotCompared means reducer comparison has not run.
	MatchStateNotCompared = "not_compared"
	// MatchStateMatchedDeclared means a live identity has a declared counterpart.
	MatchStateMatchedDeclared = "matched_declared"
)

const (
	// WarningManualProviderResource marks manual provider-only resources.
	WarningManualProviderResource = "manual_provider_resource"
	// WarningPermissionHidden marks permission-hidden provider data.
	WarningPermissionHidden = "permission_hidden"
	// WarningRateLimited marks provider rate limiting.
	WarningRateLimited = "rate_limited"
	// WarningUnsupported marks an unsupported provider endpoint.
	WarningUnsupported = "unsupported"
	// WarningPartial marks a partial read.
	WarningPartial = "partial"
	// WarningStale marks provider evidence older than the freshness window.
	WarningStale = "stale"
	// WarningTruncated marks a resource class where the client-side
	// resource_limit cap dropped provider-returned records.
	WarningTruncated = "truncated"
)

// SourceClassObserved marks live provider evidence.
const SourceClassObserved = "observed"

// SourceKind values mark the provider family of one observation.
const (
	// SourceKindPrometheus marks Prometheus provider evidence.
	SourceKindPrometheus = "prometheus"
	// SourceKindMimir marks Mimir provider evidence.
	SourceKindMimir = "mimir"
)

// EnvelopeContext carries durable claim-boundary identity for metric facts.
// The host, not this collector, derives the core fact envelope (fact ID,
// fencing token, collector kind) from the claim; none of those appear here.
type EnvelopeContext struct {
	ScopeID             string
	GenerationID        string
	CollectorInstanceID string
	SourceInstanceID    string
	SourceKind          string
	SourceSystem        string
	SourceURI           string
	ObservedAt          time.Time
}

// SourceInstance is one bounded Prometheus-compatible source observation.
type SourceInstance struct {
	Provider          string
	SourceInstanceID  string
	TenantPresent     bool
	TenantFingerprint string
	TenantRedacted    bool
}

// Target is one bounded live Prometheus scrape target observation.
type Target struct {
	ProviderObjectID   string
	ScrapePool         string
	Health             string
	ScrapeURL          string
	ScrapeURLRedacted  bool
	LabelKeys          []string
	DiscoveredKeys     []string
	LastScrapeAt       time.Time
	LastErrorRedacted  bool
	DeclaredMatchState string
	FreshnessState     string
	Outcome            string
	ManuallyCreated    bool
}

// Rule is one bounded live Prometheus-compatible rule observation.
type Rule struct {
	ProviderObjectID   string
	GroupName          string
	RuleName           string
	RuleType           string
	Health             string
	Query              string
	QueryRedacted      bool
	LabelKeys          []string
	AnnotationKeys     []string
	LastEvaluationAt   time.Time
	DeclaredMatchState string
	FreshnessState     string
	Outcome            string
	ManuallyCreated    bool
}

// Warning is one bounded coverage or redaction warning.
type Warning struct {
	ResourceClass string
	ResourceID    string
	Reason        string
}

// CollectionStats carries bounded counters for telemetry and tests.
type CollectionStats struct {
	PagesFetched int
	Targets      int
	Rules        int
	Warnings     int
	Redactions   int
	RateLimits   int
	Retries      int
	Stale        int
	Partial      bool
	// Truncated is true when any resource class hit its client-side
	// resource_limit cap and dropped provider-returned records.
	Truncated bool
}

// CollectionResult is one bounded Prometheus-compatible metadata snapshot.
type CollectionResult struct {
	Source     SourceInstance
	Targets    []Target
	Rules      []Rule
	Warnings   []Warning
	ObservedAt time.Time
	Stats      CollectionStats
}

// EvidenceClient fetches bounded metric metadata for one target.
type EvidenceClient interface {
	CollectObservedMetadata(context.Context, TargetConfig) (CollectionResult, error)
}

// ClientFactory builds a metric evidence client for a validated target.
type ClientFactory func(TargetConfig) (EvidenceClient, error)

// TargetConfig describes one Prometheus-compatible API target.
type TargetConfig struct {
	Provider         string
	ScopeID          string
	InstanceID       string
	BaseURL          string
	PathPrefix       string
	Token            string
	TenantID         string
	ResourceLimit    int
	StaleAfter       time.Duration
	Enabled          bool
	DeclaredIDs      map[string]struct{}
	ObservedOnlyHint bool
}

// String returns a redacted summary so a stray %v never prints credentials.
func (t TargetConfig) String() string {
	return "prometheus_mimir target scope=" + t.ScopeID
}

// GoString keeps %#v from printing credentials too.
func (t TargetConfig) GoString() string { return t.String() }
