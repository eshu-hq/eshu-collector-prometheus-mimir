// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// TestPrometheusMimir8864ParityTruncationIsInBand pins the resource_limit
// cut as an in-band signal: the truncated generation still commits with a
// coverage_warning, so the cutover gate can tell a capped provider read
// from a complete one.
func TestPrometheusMimir8864ParityTruncationIsInBand(t *testing.T) {
	t.Parallel()

	var truncated parityScope
	for _, ps := range parityScopes() {
		if ps.name == "prometheus truncated" {
			truncated = ps
		}
	}
	result, _ := parityLiveResult(t, truncated)
	if result.State != sdk.ResultPartial {
		t.Fatalf("state = %q, want partial for truncated generation", result.State)
	}
	reasons := warningReasons(result.Facts)
	if reasons[WarningTruncated] == 0 {
		t.Fatalf("warning reasons = %v, want a truncated coverage_warning", reasons)
	}
	counts := parityKindMultiset(result.Facts)
	if counts[FactKindObservedTarget] != 1 {
		t.Fatalf("observed_target facts = %d, want 1 capped target", counts[FactKindObservedTarget])
	}
}

// TestPrometheusMimir8864ParityPartialTargetsStayComplete pins the
// permission-hidden half of partial reads: a 403 on targets stays in-band
// as a coverage_warning while rules still commit, so the generation is
// partial but not lost.
func TestPrometheusMimir8864ParityPartialTargetsStayComplete(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/targets":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v1/rules":
			writeParityJSON(t, w, parityPrometheusRules())
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	ps := parityScopes()[0]
	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           parityClaim(ps),
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	if result.State != sdk.ResultPartial {
		t.Fatalf("state = %q, want partial for permission-hidden targets", result.State)
	}
	reasons := warningReasons(result.Facts)
	if reasons[WarningPermissionHidden] == 0 {
		t.Fatalf("warning reasons = %v, want a permission_hidden coverage_warning", reasons)
	}
	if counts := parityKindMultiset(result.Facts); counts[FactKindObservedRule] != 2 {
		t.Fatalf("observed_rule facts = %d, want 2 committed rules", counts[FactKindObservedRule])
	}
}

// TestPrometheusMimir8864ParityRetryableTransportSkipsClaim pins the
// retryable half of failure classification: a 500 exhausts bounded retries
// and fails the claim retryable with no facts, so the dispatcher (not the
// producer) owns the retry.
func TestPrometheusMimir8864ParityRetryableTransportSkipsClaim(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ps := parityScopes()[0]
	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           parityClaim(ps),
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("state = %q, want retryable for transport failure", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts = %d, want 0 on transport failure", len(result.Facts))
	}
	if want := maxHTTPRetries + 1; attempts != want {
		t.Fatalf("attempts = %d, want %d bounded retries", attempts, want)
	}
}

// TestPrometheusMimir8864ParityStaleClaimRejected pins claim scoping: a
// work item for an unconfigured scope fails retryable with no facts, so a
// revoked or reassigned scope never writes under a stale claim.
func TestPrometheusMimir8864ParityStaleClaimRejected(t *testing.T) {
	t.Parallel()

	ps := parityScopes()[0]
	mux := http.NewServeMux()
	ps.serve(t, mux, &paritySeen{})
	server := httptest.NewServer(mux)
	defer server.Close()

	claim := parityClaim(ps)
	claim.Scope.ID = "prometheus:parity:revoked"
	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("state = %q, want retryable for stale claim", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts = %d, want 0 for unconfigured scope", len(result.Facts))
	}
}

// TestPrometheusMimir8864ParityRedactionHolds pins the metadata-only
// contract at the fact layer: no emitted fact may carry raw scrape URLs,
// label values, PromQL, annotations, or tenant values, even though the
// provider served all of them.
func TestPrometheusMimir8864ParityRedactionHolds(t *testing.T) {
	t.Parallel()

	var rendered []string
	for _, ps := range parityScopes() {
		result, _ := parityLiveResult(t, ps)
		for _, fact := range result.Facts {
			rendered = append(rendered, strings.ToLower(anyParityJSON(t, fact.Payload)))
		}
	}
	forbidden := []string{
		"10.0.0.1", "10.0.0.2", "node-1", "node-2",
		"sum(rate(secret_total", "secret_bytes", "up == 0",
		"parity alert", "parity record", "mimir alert",
		"tenant-acme", "connection refused",
	}
	for _, payload := range rendered {
		for _, value := range forbidden {
			if strings.Contains(payload, strings.ToLower(value)) {
				t.Fatalf("fact payload leaked forbidden value %q", value)
			}
		}
	}
}

// TestPrometheusMimir8864ParityGrantStubFailsClosed documents the
// delegation delta: with a grant authorizer attached, a missing grant fails
// the claim terminal with no facts. The parity path above runs authorizer-free
// (like the in-tree first-party path); the extracted producer must fail
// closed once the host wires grant enforcement.
func TestPrometheusMimir8864ParityGrantStubFailsClosed(t *testing.T) {
	t.Parallel()

	ps := parityScopes()[1]
	mux := http.NewServeMux()
	ps.serve(t, mux, &paritySeen{})
	server := httptest.NewServer(mux)
	defer server.Close()

	handler := Handler{Authorizer: FailClosedAuthorizer{}}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           parityClaim(ps),
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("state = %q, want terminal for missing grant", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts = %d, want 0 when grant denies emission", len(result.Facts))
	}
}

// TestPrometheusMimir8864ParityDisabledTargetSkipsBeforeClient pins the
// in-tree construction rule (source.go:53): disabled targets are dropped
// before client construction, so a claim for a disabled scope is never
// contacted and fails retryable with no facts, like an unconfigured scope.
func TestPrometheusMimir8864ParityDisabledTargetSkipsBeforeClient(t *testing.T) {
	t.Parallel()

	hits := 0
	disabledServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(disabledServer.Close)

	ps := parityScopes()[0]
	mux := http.NewServeMux()
	ps.serve(t, mux, &paritySeen{})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	enabled := map[string]any{
		"provider":           ps.provider,
		"scope_id":           ps.scopeID,
		"instance_id":        ps.instanceID,
		"base_url":           server.URL,
		"resource_limit":     ps.resourceLimit,
		"enabled":            true,
		"declared_ids":       []any{},
		"observed_only_hint": ps.observedOnly,
	}
	disabled := map[string]any{
		"provider":       ProviderMimir,
		"scope_id":       "mimir:parity:disabled",
		"instance_id":    "mimir-disabled",
		"base_url":       disabledServer.URL,
		"resource_limit": 50,
		"enabled":        false,
		"declared_ids":   []any{},
	}
	config := map[string]any{"targets": []any{enabled, disabled}}

	claim := parityClaim(ps)
	claim.Scope.ID = "mimir:parity:disabled"
	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          config,
	})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("state = %q, want retryable for disabled scope", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts = %d, want 0 for disabled scope", len(result.Facts))
	}
	if hits != 0 {
		t.Fatalf("disabled target contacted %d times, want 0 (skip before client construction)", hits)
	}
}

// TestParseSettingsRequiresEnabledTarget pins the construction half of the
// disabled rule: an activation config with only disabled targets is
// rejected, mirroring in-tree "at least one enabled metric target".
func TestParseSettingsRequiresEnabledTarget(t *testing.T) {
	t.Parallel()

	config := map[string]any{"targets": []any{map[string]any{
		"provider":    ProviderPrometheus,
		"scope_id":    "prometheus:parity:off",
		"instance_id": "prometheus-off",
		"base_url":    "http://127.0.0.1:9",
		"enabled":     false,
	}}}
	if _, err := ParseSettings(config, "", nil); err == nil {
		t.Fatal("ParseSettings(all-disabled) error = nil, want enabled-target requirement")
	}
}
