// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// Handler runs one claim. The zero value is usable: it reads the process
// environment, the wall clock, and the real Prometheus-compatible HTTP
// client. Grant enforcement stays a fail-closed stub: attach an Authorizer
// to deny emission without a live grant; nil preserves the in-tree
// first-party behavior for parity and replay.
type Handler struct {
	// Getenv resolves credential environment variables; nil selects an
	// empty lookup at the call site in cmd/collector.
	Getenv func(string) string
	// Now supplies the clock; nil selects time.Now.
	Now func() time.Time
	// NewClient builds the evidence client; nil selects the HTTP client.
	NewClient ClientFactory
	// Authorizer enforces producer grants; nil skips the lookup.
	Authorizer GrantAuthorizer
	// Log receives one bounded JSON diagnostic line per claim; nil discards it.
	Log io.Writer
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h Handler) getenv(key string) string {
	if h.Getenv != nil {
		return h.Getenv(key)
	}
	return ""
}

func (h Handler) newClient(target TargetConfig) (EvidenceClient, error) {
	if h.NewClient != nil {
		return h.NewClient(target)
	}
	return NewHTTPClient(HTTPClientConfig{BaseURL: target.BaseURL})
}

// Handle collects the metric target named by the claim and returns the SDK
// result the host validates and commits. It never returns facts together
// with a failure state, never lets a credential into a result, and echoes
// the claim unchanged so the host's identity check passes.
func (h Handler) Handle(ctx context.Context, request Request) sdk.Result {
	started := h.now()
	claim := request.Claim
	if claim.CollectorKind != "" && claim.CollectorKind != CollectorKind {
		return h.terminalResult(claim, started, FailureCollectorKind, "metric source cannot collect work items for another collector kind")
	}
	if strings.TrimSpace(claim.Scope.ID) == "" || strings.TrimSpace(claim.GenerationID) == "" {
		return h.terminalResult(claim, started, FailureInvalidConfig, "claim scope and generation are required")
	}
	settings, err := ParseSettings(request.Config, claim.Scope.ID, h.getenv)
	if err != nil {
		var cfgErr ConfigError
		class := FailureInvalidConfig
		if errors.As(err, &cfgErr) {
			class = cfgErr.FailureClass()
		}
		if strings.Contains(err.Error(), "is not configured") {
			return h.retryableResult(claim, started, FailureStaleClaim, 0)
		}
		return h.terminalResult(claim, started, class, "invalid activation config")
	}
	var target *TargetConfig
	for i := range settings.Targets {
		if settings.Targets[i].ScopeID == strings.TrimSpace(claim.Scope.ID) {
			target = &settings.Targets[i]
			break
		}
	}
	if target == nil {
		return h.retryableResult(claim, started, FailureStaleClaim, 0)
	}
	if err := checkGrants(h.Authorizer, claim.Scope.ID, FactKinds()); err != nil {
		return h.terminalResult(claim, started, FailureGrantDenied, "producer grant denies emission")
	}
	client, err := h.newClient(*target)
	if err != nil {
		return h.terminalResult(claim, started, FailureInvalidConfig, "invalid target client")
	}
	collected, err := client.CollectObservedMetadata(ctx, *target)
	if err != nil {
		if ctx.Err() != nil {
			return h.retryableResult(claim, started, FailureRetryable, latencyMillis(h.now().Sub(started)))
		}
		failure := classifiedProviderFailure(err)
		class := failure.FailureClass()
		latency := latencyMillis(h.now().Sub(started))
		if failure.TerminalFailure() {
			return h.failureResult(claim, started, class, true, latency)
		}
		return h.failureResult(claim, started, class, false, latency)
	}
	observedAt := collected.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = started
	}
	envCtx := EnvelopeContext{
		ScopeID:             target.ScopeID,
		GenerationID:        claim.GenerationID,
		CollectorInstanceID: firstNonBlank(claim.InstanceID, claim.ComponentID),
		SourceInstanceID:    target.InstanceID,
		SourceKind:          normalizedProvider(target.Provider),
		SourceSystem:        firstNonBlank(claim.SourceSystem, SourceSystem),
		SourceURI:           target.BaseURL,
		ObservedAt:          observedAt,
	}
	facts, err := buildFacts(envCtx, collected)
	if err != nil {
		return h.terminalResult(claim, started, FailureTerminal, "invalid provider evidence")
	}
	facts = dedupeFacts(facts)
	latency := latencyMillis(h.now().Sub(started))
	state := sdk.ResultComplete
	statusClass := sdk.StatusComplete
	if collected.Stats.Partial || collected.Stats.Truncated {
		state = sdk.ResultPartial
		statusClass = sdk.StatusWarning
	}
	statuses := []sdk.Status{
		{
			Class:           statusClass,
			FactCount:       len(facts),
			WarningCount:    len(collected.Warnings),
			SourceLatencyMS: latency,
		},
	}
	if collected.Stats.RateLimits > 0 {
		statuses = append(statuses, sdk.Status{
			Class:           sdk.StatusWarning,
			FailureClass:    FailureRateLimited,
			SourceLatencyMS: latency,
		})
	}
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           state,
		Claim:           claim,
		Generation: sdk.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    observedAt,
			FreshnessHint: "prometheus_mimir_observed_metadata",
		},
		Facts:    facts,
		Statuses: statuses,
	}
}

func buildFacts(ctx EnvelopeContext, result CollectionResult) ([]sdk.Fact, error) {
	stats := result.Stats
	stats.Targets = len(result.Targets)
	stats.Rules = len(result.Rules)
	stats.Warnings = len(result.Warnings)
	out := make([]sdk.Fact, 0, 1+len(result.Targets)+len(result.Rules)+len(result.Warnings))
	source, err := newSourceInstanceFact(ctx, result.Source, stats)
	if err != nil {
		return nil, err
	}
	out = append(out, source)
	for _, target := range result.Targets {
		fact, err := newObservedTargetFact(ctx, target)
		if err != nil {
			return nil, err
		}
		out = append(out, fact)
	}
	for _, rule := range result.Rules {
		fact, err := newObservedRuleFact(ctx, rule)
		if err != nil {
			return nil, err
		}
		out = append(out, fact)
	}
	for _, warning := range result.Warnings {
		fact, err := newCoverageWarningFact(ctx, warning)
		if err != nil {
			return nil, err
		}
		out = append(out, fact)
	}
	return out, nil
}

// dedupeFacts collapses facts that share a kind and stable key, keeping the
// last occurrence at the position of the first. The host validator rejects a
// conflicting duplicate outright, so collapsing here keeps a repeated warning
// from failing the whole claim.
func dedupeFacts(facts []sdk.Fact) []sdk.Fact {
	index := make(map[string]int, len(facts))
	out := make([]sdk.Fact, 0, len(facts))
	for _, fact := range facts {
		key := fact.Kind + "\x00" + fact.StableKey
		if at, ok := index[key]; ok {
			out[at] = fact
			continue
		}
		index[key] = len(out)
		out = append(out, fact)
	}
	return out
}

func (h Handler) terminalResult(claim sdk.Claim, started time.Time, class string, _ string) sdk.Result {
	latency := latencyMillis(h.now().Sub(started))
	return h.failureResult(claim, started, class, true, latency)
}

func (h Handler) retryableResult(claim sdk.Claim, started time.Time, class string, _ int) sdk.Result {
	latency := latencyMillis(h.now().Sub(started))
	return h.failureResult(claim, started, class, false, latency)
}

func (h Handler) failureResult(claim sdk.Claim, started time.Time, class string, terminal bool, latency int) sdk.Result {
	state := sdk.ResultRetryable
	if terminal {
		state = sdk.ResultTerminal
	}
	observedAt := h.now()
	if !started.IsZero() {
		observedAt = started
	}
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           state,
		Claim:           claim,
		Generation: sdk.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    observedAt,
			FreshnessHint: "prometheus_mimir_observed_metadata",
		},
		Statuses: []sdk.Status{
			{
				Class:           sdk.StatusFailure,
				FailureClass:    class,
				SourceLatencyMS: latency,
			},
		},
	}
}

func latencyMillis(d time.Duration) int {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > 1<<31-1 {
		return 1<<31 - 1
	}
	return int(ms)
}


