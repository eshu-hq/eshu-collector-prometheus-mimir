// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// coverageClientTarget returns a validated target against a test server.
func coverageClientTarget(t *testing.T, baseURL string, limit int) TargetConfig {
	t.Helper()

	target := TargetConfig{
		Provider: "prometheus", ScopeID: "s", InstanceID: "i",
		BaseURL: baseURL, ResourceLimit: limit,
	}
	mapped, err := validateTarget(target)
	if err != nil {
		t.Fatalf("validateTarget() error = %v, want nil", err)
	}
	return mapped
}

// coverageRulesBody renders a rules payload with the given rule entries.
func coverageRulesBody(rules ...map[string]any) map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"groups": []map[string]any{
				{"name": "g", "file": "rules/g.yaml", "rules": rules},
			},
		},
	}
}

// coverageRuleEntry renders one rule resource entry.
func coverageRuleEntry(name string, query string) map[string]any {
	return map[string]any{
		"name": name, "type": "alerting", "health": "ok", "query": query,
		"lastEvaluation": "2026-09-30T12:00:00Z",
		"labels":         map[string]any{"service": "api"},
		"annotations":    map[string]any{"summary": "burn"},
	}
}

// writeCoverageJSON renders value as a provider response.
func writeCoverageJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode provider response error = %v, want nil", err)
	}
}

// coverageTargetsSuccess serves an empty targets page.
func coverageTargetsSuccess(t *testing.T, mux *http.ServeMux) {
	t.Helper()

	mux.HandleFunc("/api/v1/targets", func(w http.ResponseWriter, _ *http.Request) {
		writeCoverageJSON(t, w, map[string]any{
			"status": "success",
			"data":   map[string]any{"activeTargets": []any{}},
		})
	})
}

// TestCollectRulesTruncatesBeyondTheLimit pins the client-side cap: records
// past the limit are dropped, counted, and warned in-band.
func TestCollectRulesTruncatesBeyondTheLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCoverageJSON(t, w, coverageRulesBody(
			coverageRuleEntry("R1", "up == 0"),
			coverageRuleEntry("R2", "up == 1"),
			coverageRuleEntry("R3", "up == 2"),
		))
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	target := coverageClientTarget(t, server.URL, 1)
	result := &CollectionResult{}
	if err := client.collectRules(context.Background(), target, result); err != nil {
		t.Fatalf("collectRules() error = %v, want nil", err)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want the 1-record cap", len(result.Rules))
	}
	if !result.Stats.Truncated {
		t.Error("Stats.Truncated = false, want true past the limit")
	}
	found := false
	for _, warning := range result.Warnings {
		if warning.Reason == WarningTruncated && warning.ResourceClass == ResourceClassRule {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %+v, want a rule truncation warning", result.Warnings)
	}
}

// TestCollectRulesWarnsOnStaleEvidence pins the freshness path: a rule
// evaluated before the staleness window warns stale and counts stale.
func TestCollectRulesWarnsOnStaleEvidence(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCoverageJSON(t, w, coverageRulesBody(
			map[string]any{
				"name": "OldBurn", "type": "alerting", "health": "ok",
				"query": "up == 0", "lastEvaluation": "2020-01-01T00:00:00Z",
			},
		))
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	target := coverageClientTarget(t, server.URL, 10)
	target.StaleAfter = time.Hour
	result := &CollectionResult{ObservedAt: time.Now().UTC()}
	if err := client.collectRules(context.Background(), target, result); err != nil {
		t.Fatalf("collectRules() error = %v, want nil", err)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(result.Rules))
	}
	if result.Rules[0].FreshnessState != FreshnessStale || result.Rules[0].Outcome != OutcomeStale {
		t.Errorf("rule = %+v, want stale freshness and outcome", result.Rules[0])
	}
	if result.Stats.Stale != 1 {
		t.Errorf("Stats.Stale = %d, want 1", result.Stats.Stale)
	}
	found := false
	for _, warning := range result.Warnings {
		if warning.Reason == WarningStale && warning.ResourceID == result.Rules[0].ProviderObjectID {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %+v, want a stale warning for the rule", result.Warnings)
	}
}

// TestCollectRulesSkipsIdentitylessRecords pins the blank-identity floor:
// a group with no name, rule name, or query contributes nothing.
func TestCollectRulesSkipsIdentitylessRecords(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCoverageJSON(t, w, map[string]any{
			"status": "success",
			"data": map[string]any{
				"groups": []map[string]any{
					{"name": "", "file": "rules/blank.yaml", "rules": []map[string]any{
						{"name": "", "query": ""},
					}},
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	result := &CollectionResult{}
	if err := client.collectRules(context.Background(), coverageClientTarget(t, server.URL, 10), result); err != nil {
		t.Fatalf("collectRules() error = %v, want nil", err)
	}
	if len(result.Rules) != 0 {
		t.Errorf("len(Rules) = %d, want 0 for identityless records", len(result.Rules))
	}
	if result.Stats.Truncated {
		t.Error("Stats.Truncated = true, want false when nothing was considered")
	}
}

// TestCollectMetadataSurfacesProviderAPIErrors pins the API-status path: a
// provider error status becomes a bounded ProviderAPIError with its
// operation token, so claim handling can classify it.
func TestCollectMetadataSurfacesProviderAPIErrors(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	coverageTargetsSuccess(t, mux)
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, _ *http.Request) {
		writeCoverageJSON(t, w, map[string]any{
			"status": "error", "errorType": "bad_data", "error": "bad query",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	_, err = client.CollectObservedMetadata(context.Background(), coverageClientTarget(t, server.URL, 10))
	var apiErr ProviderAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CollectObservedMetadata() error = %v, want a ProviderAPIError", err)
	}
	if apiErr.ErrorType != "bad_data" || apiErr.Operation != "list_rules" {
		t.Errorf("ProviderAPIError = %+v, want bad_data on list_rules", apiErr)
	}
}

// TestCollectMetadataMarksForbiddenEndpointsPartial pins the permission
// path: a 403 becomes an in-band permission-hidden warning, not an error.
func TestCollectMetadataMarksForbiddenEndpointsPartial(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	coverageTargetsSuccess(t, mux)
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	result, err := client.CollectObservedMetadata(context.Background(), coverageClientTarget(t, server.URL, 10))
	if err != nil {
		t.Fatalf("CollectObservedMetadata() error = %v, want the partial path", err)
	}
	if !result.Stats.Partial {
		t.Error("Stats.Partial = false, want true for a hidden endpoint")
	}
	found := false
	for _, warning := range result.Warnings {
		if warning.Reason == WarningPermissionHidden && warning.ResourceClass == ResourceClassRule {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %+v, want a rule permission-hidden warning", result.Warnings)
	}
}

// TestCollectMetadataFailsOnUnexpectedStatus pins the fail-loud path: a
// status outside the bounded vocabulary is an error, never a silent skip.
func TestCollectMetadataFailsOnUnexpectedStatus(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	coverageTargetsSuccess(t, mux)
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	if _, err := client.CollectObservedMetadata(context.Background(), coverageClientTarget(t, server.URL, 10)); err == nil {
		t.Error("CollectObservedMetadata(418) error = nil, want a failure")
	}
}

// TestCollectMetadataRejectsUnknownProviders pins the provider gate before
// any network use.
func TestCollectMetadataRejectsUnknownProviders(t *testing.T) {
	t.Parallel()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: "http://127.0.0.1:9"})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	target := coverageClientTarget(t, "http://127.0.0.1:9", 10)
	target.Provider = "datadog"
	if _, err := client.CollectObservedMetadata(context.Background(), target); err == nil {
		t.Error("CollectObservedMetadata(datadog) error = nil, want a failure")
	}
}

// TestCollectMetadataRedactsTenantWithoutRetaining pins the tenant path: a
// configured tenant marks presence, fingerprints for correlation, and never
// retains the raw value.
func TestCollectMetadataRedactsTenantWithoutRetaining(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Scope-OrgID"); got != "tenant-acme" {
			t.Errorf("X-Scope-OrgID = %q, want the configured tenant", got)
		}
		writeCoverageJSON(t, w, coverageRulesBody())
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v, want nil", err)
	}
	target := coverageClientTarget(t, server.URL, 10)
	target.Provider = ProviderMimir
	target.TenantID = "tenant-acme"
	result, err := client.CollectObservedMetadata(context.Background(), target)
	if err != nil {
		t.Fatalf("CollectObservedMetadata() error = %v, want nil", err)
	}
	if !result.Source.TenantPresent || !result.Source.TenantRedacted {
		t.Errorf("Source = %+v, want tenant presence with redaction", result.Source)
	}
	if result.Source.TenantFingerprint != fingerprint("tenant-acme") {
		t.Errorf("TenantFingerprint = %q, want the tenant fingerprint", result.Source.TenantFingerprint)
	}
}

// TestNewHTTPClientRejectsUnparseableBaseURLs pins client construction:
// a malformed base URL fails before any claim runs.
func TestNewHTTPClientRejectsUnparseableBaseURLs(t *testing.T) {
	t.Parallel()

	if _, err := NewHTTPClient(HTTPClientConfig{BaseURL: "://bad"}); err == nil {
		t.Error("NewHTTPClient(bad URL) error = nil, want a failure")
	}
}

// TestBuildFactsRejectsBlankIdentities pins the identity floor at build
// time: a target or rule without an identity fails the claim loudly.
func TestBuildFactsRejectsBlankIdentities(t *testing.T) {
	t.Parallel()

	ctx := coverageEnvelopeContext()
	targetResult := CollectionResult{Targets: []Target{{ProviderObjectID: "  "}}}
	if _, err := buildFacts(ctx, targetResult); err == nil {
		t.Error("buildFacts(blank target identity) error = nil, want a failure")
	}
	ruleResult := CollectionResult{Rules: []Rule{{GroupName: "", RuleName: "", Query: "  "}}}
	if _, err := buildFacts(ctx, ruleResult); err == nil {
		t.Error("buildFacts(blank rule identity) error = nil, want a failure")
	}
}

// TestCoverageWarningFactDefaults pins the warning defaults: a blank
// reason reads partial and a blank class reads unknown, so a bare warning
// still emits a bounded fact.
func TestCoverageWarningFactDefaults(t *testing.T) {
	t.Parallel()

	fact, err := newCoverageWarningFact(coverageEnvelopeContext(), Warning{})
	if err != nil {
		t.Fatalf("newCoverageWarningFact() error = %v, want nil", err)
	}
	if fact.Payload["warning_kind"] != WarningPartial {
		t.Errorf("warning_kind = %v, want %q", fact.Payload["warning_kind"], WarningPartial)
	}
	if fact.Payload["resource_class"] != "unknown" {
		t.Errorf("resource_class = %v, want unknown", fact.Payload["resource_class"])
	}
	if _, ok := fact.Payload["provider_object_uid"]; ok {
		t.Errorf("payload = %v, want no object UID for a blank resource", fact.Payload)
	}
}

// TestObservedTargetFactMarksManualResources pins the drift-candidate
// path: a manually created target carries its reason for the reducer.
func TestObservedTargetFactMarksManualResources(t *testing.T) {
	t.Parallel()

	fact, err := newObservedTargetFact(coverageEnvelopeContext(), Target{
		ProviderObjectID: "node:9090", ManuallyCreated: true,
	})
	if err != nil {
		t.Fatalf("newObservedTargetFact() error = %v, want nil", err)
	}
	if fact.Payload["manually_created"] != true {
		t.Errorf("payload = %v, want the manual flag", fact.Payload)
	}
	if fact.Payload["drift_candidate_reason"] != WarningManualProviderResource {
		t.Errorf("payload = %v, want the drift-candidate reason", fact.Payload)
	}
}

// TestObservedRuleFactFingerprintsQueryOnlyRules pins the query-identity
// path end to end: a rule with no names is keyed by its query fingerprint
// through normalizeRule into the fact, and the raw query never lands in
// the payload.
func TestObservedRuleFactFingerprintsQueryOnlyRules(t *testing.T) {
	t.Parallel()

	target := TargetConfig{Provider: ProviderPrometheus, ScopeID: "s", InstanceID: "i"}
	normalized := normalizeRule(ruleGroupResource{}, ruleResource{Query: "up == 0"}, target, time.Now().UTC())
	if normalized.ProviderObjectID != fingerprint("up == 0") {
		t.Fatalf("normalizeRule(query only) identity = %q, want the query fingerprint", normalized.ProviderObjectID)
	}
	fact, err := newObservedRuleFact(coverageEnvelopeContext(), normalized)
	if err != nil {
		t.Fatalf("newObservedRuleFact() error = %v, want nil", err)
	}
	if fact.Payload["provider_object_uid"] != fingerprint("up == 0") {
		t.Errorf("provider_object_uid = %v, want the query fingerprint", fact.Payload["provider_object_uid"])
	}
	if fact.Payload["query_redacted"] != true {
		t.Errorf("payload = %v, want the query marked redacted", fact.Payload)
	}
	for key, value := range fact.Payload {
		if text, ok := value.(string); ok && strings.Contains(text, "up == 0") {
			t.Errorf("payload[%q] quotes the raw query, want redaction", key)
		}
	}
}

// TestSourceInstanceFactCarriesTenantFingerprint pins the tenant
// correlation path: presence and redaction survive with the fingerprint.
func TestSourceInstanceFactCarriesTenantFingerprint(t *testing.T) {
	t.Parallel()

	fact, err := newSourceInstanceFact(coverageEnvelopeContext(), SourceInstance{
		TenantPresent: true, TenantRedacted: true, TenantFingerprint: "fp-1",
	}, CollectionStats{})
	if err != nil {
		t.Fatalf("newSourceInstanceFact() error = %v, want nil", err)
	}
	if fact.Payload["tenant_scope_state"] != "configured" || fact.Payload["tenant_redacted"] != true {
		t.Errorf("payload = %v, want tenant scope state with redaction", fact.Payload)
	}
	if fact.Payload["tenant_fingerprint"] != "fp-1" {
		t.Errorf("payload = %v, want the tenant fingerprint", fact.Payload)
	}
}

// TestDedupeFactsKeepsTheLastOccurrence pins the dedupe contract: facts
// sharing kind and stable key collapse with the last winning, distinct
// keys survive.
func TestDedupeFactsKeepsTheLastOccurrence(t *testing.T) {
	t.Parallel()

	first := sdk.Fact{Kind: FactKindCoverageWarning, StableKey: "k", Payload: map[string]any{"n": 1}}
	second := sdk.Fact{Kind: FactKindCoverageWarning, StableKey: "k", Payload: map[string]any{"n": 2}}
	other := sdk.Fact{Kind: FactKindCoverageWarning, StableKey: "other", Payload: map[string]any{}}
	got := dedupeFacts([]sdk.Fact{first, other, second})
	if len(got) != 2 {
		t.Fatalf("len(dedupeFacts) = %d, want 2", len(got))
	}
	if got[0].Payload["n"] != 2 {
		t.Errorf("dedupeFacts[0] payload = %v, want the last occurrence to win", got[0].Payload)
	}
	if got[1].StableKey != "other" {
		t.Errorf("dedupeFacts[1] key = %q, want the distinct fact kept", got[1].StableKey)
	}
}

// TestMergeContractPayloadSurfacesEncodeFailures pins the error path: a
// failed contract encode fails the fact, never emits a half payload.
func TestMergeContractPayloadSurfacesEncodeFailures(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"outcome": OutcomeObserved}
	boom := errors.New("boom")
	err := mergeContractPayload(payload, func() (map[string]any, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Errorf("mergeContractPayload() error = %v, want the encode failure", err)
	}
}

// TestCheckGrantsFailsClosed pins the grant seam: nil authorizes for
// first-party replay, an attached denial fails the claim.
func TestCheckGrantsFailsClosed(t *testing.T) {
	t.Parallel()

	if err := checkGrants(nil, "s", FactKinds()); err != nil {
		t.Errorf("checkGrants(nil) error = %v, want nil", err)
	}
	if err := checkGrants(FailClosedAuthorizer{}, "s", FactKinds()); err == nil {
		t.Error("checkGrants(deny-all) error = nil, want fail-closed")
	}
}
