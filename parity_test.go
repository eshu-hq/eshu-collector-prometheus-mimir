// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// parityObservedAt is the fixed generation timestamp recorded in the
// cassette scopes. Live runs use the real clock for observed_at, which the
// comparison excludes; everything identity-bearing is deterministic.
var parityObservedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// parityScope binds one live claim to its recorded cassette scope.
type parityScope struct {
	name           string
	provider       string
	scopeID        string
	instanceID     string
	generationID   string
	fencingToken   string
	resourceLimit  int
	staleAfter     time.Duration
	declaredIDs    map[string]struct{}
	observedOnly   bool
	tenantID       string
	serve          func(t *testing.T, mux *http.ServeMux, seen *paritySeen)
	expectNoTarget bool
}

// paritySeen records cross-cutting server observations.
type paritySeen struct {
	targetPaths []string
	tenant      []string
}

func parityPrometheusTargets() map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"activeTargets": []map[string]any{
				{
					"discoveredLabels": map[string]any{
						"__address__":      "10.0.0.1:9100",
						"__metrics_path__": "/metrics",
					},
					"labels": map[string]any{
						"instance": "node-1",
						"job":      "node",
					},
					"scrapePool": "nodes",
					"scrapeUrl":  "http://10.0.0.1:9100/metrics",
					"health":     "up",
					"lastScrape": "2026-09-30T12:00:00Z",
				},
				{
					"discoveredLabels": map[string]any{
						"__address__": "10.0.0.2:9100",
					},
					"labels": map[string]any{
						"instance": "node-2",
						"job":      "node",
					},
					"scrapePool": "nodes",
					"scrapeUrl":  "http://10.0.0.2:9100/metrics",
					"health":     "down",
					"lastScrape": "2026-01-01T00:00:00Z",
					"lastError":  "connection refused",
				},
			},
		},
	}
}

func parityPrometheusRules() map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"groups": []map[string]any{
				{
					"name": "parity.rules",
					"file": "rules/parity.yaml",
					"rules": []map[string]any{
						{
							"name":           "ParityBurn",
							"type":           "alerting",
							"health":         "ok",
							"query":          "sum(rate(secret_total[5m])) > 7",
							"lastEvaluation": "2026-09-30T12:00:00Z",
							"labels":         map[string]any{"service": "parity"},
							"annotations":    map[string]any{"summary": "parity alert"},
						},
						{
							"name":           "ParityRecord",
							"type":           "recording",
							"health":         "ok",
							"query":          "sum(secret_bytes)",
							"lastEvaluation": "2026-01-01T00:00:00Z",
							"labels":         map[string]any{"service": "parity"},
							"annotations":    map[string]any{"summary": "parity record"},
						},
					},
				},
			},
		},
	}
}

// parityScopes mirrors the three recorded cassette scopes: a full
// Prometheus scope (targets, rules, stale and manual signals), a Mimir
// tenant scope (rules only, tenant redacted), and a truncated Prometheus
// scope (resource_limit cut with an in-band warning).
func parityScopes() []parityScope {
	return []parityScope{
		{
			name:          "prometheus full",
			provider:      ProviderPrometheus,
			scopeID:       "prometheus:parity:prod",
			instanceID:    "prometheus-parity",
			generationID:  "prometheus-mimir:8864:prod:1",
			fencingToken:  "7",
			resourceLimit: 50,
			staleAfter:    time.Hour,
			declaredIDs:   map[string]struct{}{"parity.rules:ParityBurn": {}},
			observedOnly:  true,
			serve: func(t *testing.T, mux *http.ServeMux, seen *paritySeen) {
				t.Helper()
				mux.HandleFunc("/api/v1/targets", func(w http.ResponseWriter, r *http.Request) {
					seen.targetPaths = append(seen.targetPaths, r.URL.String())
					writeParityJSON(t, w, parityPrometheusTargets())
				})
				mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
					writeParityJSON(t, w, parityPrometheusRules())
				})
			},
		},
		{
			name:          "mimir tenant",
			provider:      ProviderMimir,
			scopeID:       "mimir:parity:tenant",
			instanceID:    "mimir-parity",
			generationID:  "prometheus-mimir:8864:mimir:1",
			fencingToken:  "9",
			resourceLimit: 50,
			tenantID:      "tenant-acme",
			serve: func(t *testing.T, mux *http.ServeMux, seen *paritySeen) {
				t.Helper()
				mux.HandleFunc("/api/v1/targets", func(w http.ResponseWriter, r *http.Request) {
					seen.targetPaths = append(seen.targetPaths, r.URL.String())
					t.Error("mimir must not request targets")
				})
				mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
					seen.tenant = append(seen.tenant, r.Header.Get("X-Scope-OrgID"))
					writeParityJSON(t, w, map[string]any{
						"status": "success",
						"data": map[string]any{
							"groups": []map[string]any{
								{
									"name": "mimir.rules",
									"file": "rules/mimir.yaml",
									"rules": []map[string]any{
										{
											"name":           "MimirBurn",
											"type":           "alerting",
											"health":         "ok",
											"query":          "sum(rate(secret_total[5m])) > 3",
											"lastEvaluation": "2026-09-30T12:00:00Z",
											"labels":         map[string]any{"service": "mimir"},
											"annotations":    map[string]any{"summary": "mimir alert"},
										},
									},
								},
							},
						},
					})
				})
			},
			expectNoTarget: true,
		},
		{
			name:          "prometheus truncated",
			provider:      ProviderPrometheus,
			scopeID:       "prometheus:parity:truncated",
			instanceID:    "prometheus-parity-trunc",
			generationID:  "prometheus-mimir:8864:trunc:1",
			fencingToken:  "11",
			resourceLimit: 1,
			staleAfter:    time.Hour,
			serve: func(t *testing.T, mux *http.ServeMux, seen *paritySeen) {
				t.Helper()
				mux.HandleFunc("/api/v1/targets", func(w http.ResponseWriter, r *http.Request) {
					writeParityJSON(t, w, parityPrometheusTargets())
				})
				mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
					writeParityJSON(t, w, map[string]any{
						"status": "success",
						"data": map[string]any{
							"groups": []map[string]any{
								{
									"name": "trunc.rules",
									"file": "rules/trunc.yaml",
									"rules": []map[string]any{
										{
											"name":           "TruncBurn",
											"type":           "alerting",
											"health":         "ok",
											"query":          "up == 0",
											"lastEvaluation": "2026-09-30T12:00:00Z",
										},
									},
								},
							},
						},
					})
				})
			},
		},
	}
}

// parityClaim builds the SDK claim for one parity scope.
func parityClaim(ps parityScope) sdk.Claim {
	return sdk.Claim{
		ComponentID:   ComponentID,
		InstanceID:    "collector-prometheus-parity",
		CollectorKind: CollectorKind,
		SourceSystem:  SourceSystem,
		Scope:         sdk.Scope{ID: ps.scopeID, Kind: ScopeKindMetricSource},
		SourceRunID:   "parity-run",
		GenerationID:  ps.generationID,
		WorkItemID:    "parity-work-" + ps.scopeID,
		FencingToken:  ps.fencingToken,
		Attempt:       1,
		Deadline:      time.Now().Add(time.Minute),
		ConfigHandle:  "parity-config",
	}
}

// parityConfig builds the activation config map for one parity scope against
// a live httptest server URL.
func parityConfig(ps parityScope, baseURL string) map[string]any {
	declared := make([]string, 0, len(ps.declaredIDs))
	for id := range ps.declaredIDs {
		declared = append(declared, id)
	}
	slices.Sort(declared)
	target := map[string]any{
		"provider":           ps.provider,
		"scope_id":           ps.scopeID,
		"instance_id":        ps.instanceID,
		"base_url":           baseURL,
		"resource_limit":     ps.resourceLimit,
		"enabled":            true,
		"declared_ids":       declared,
		"observed_only_hint": ps.observedOnly,
	}
	if ps.staleAfter != 0 {
		target["stale_after"] = ps.staleAfter.String()
	}
	if ps.tenantID != "" {
		target["tenant_id"] = ps.tenantID
	}
	return map[string]any{"targets": []any{target}}
}

// parityLiveResult runs one scope through the extracted producer against its
// httptest server and returns the SDK result plus server observations.
func parityLiveResult(t *testing.T, ps parityScope) (sdk.Result, *paritySeen) {
	t.Helper()
	seen := &paritySeen{}
	mux := http.NewServeMux()
	ps.serve(t, mux, seen)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           parityClaim(ps),
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	return result, seen
}

func writeParityJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode parity response: %v", err)
	}
}

// cassetteScope is one recorded scope from the 8864 fixture pack.
type cassetteScope struct {
	ScopeID      string `json:"scope_id"`
	GenerationID string `json:"generation_id"`
	ObservedAt   string `json:"observed_at"`
	Facts        []struct {
		FactKind      string         `json:"fact_kind"`
		StableFactKey string         `json:"stable_fact_key"`
		SchemaVersion string         `json:"schema_version"`
		Payload       map[string]any `json:"payload"`
		SourceRecord  string         `json:"source_record_id"`
		FencingToken  int64          `json:"fencing_token"`
	} `json:"facts"`
}

func loadCassetteScopes(t *testing.T) []cassetteScope {
	t.Helper()
	raw, err := os.ReadFile("testdata/8864-parity-prometheus-mimir.json")
	if err != nil {
		t.Fatalf("read cassette fixture: %v", err)
	}
	var pack struct {
		Scopes []cassetteScope `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatalf("decode cassette fixture: %v", err)
	}
	return pack.Scopes
}

func parityKindMultiset(facts []sdk.Fact) map[string]int {
	counts := map[string]int{}
	for _, fact := range facts {
		counts[fact.Kind]++
	}
	return counts
}

func parityStableKeys(facts []sdk.Fact) []string {
	keys := make([]string, 0, len(facts))
	for _, fact := range facts {
		keys = append(keys, fact.StableKey)
	}
	slices.Sort(keys)
	return keys
}

// parityCanonicalPayload renders a payload with clock-owned fields removed so
// live runs and the recorded cassette compare on identity-bearing content.
func parityCanonicalPayload(t *testing.T, payload map[string]any) string {
	t.Helper()
	clean := maps.Clone(payload)
	delete(clean, "observed_at")
	rendered, err := json.Marshal(clean)
	if err != nil {
		t.Fatalf("marshal payload error = %v, want nil", err)
	}
	return string(rendered)
}

// TestPrometheusMimir8864ParityLiveAgreesWithCassette is the AC3
// source-parity pin: the extracted live claim and the checked-in cassette
// agree scope by scope on scope ID, generation ID, fact-kind multiset,
// stable keys, and payloads. The cassette was cut from a live HTTP run with
// the same fixed generations; the live side is the oracle.
func TestPrometheusMimir8864ParityLiveAgreesWithCassette(t *testing.T) {
	t.Parallel()

	recorded := loadCassetteScopes(t)
	if len(recorded) != 3 {
		t.Fatalf("cassette scopes = %d, want 3", len(recorded))
	}

	scopes := parityScopes()
	for i, want := range recorded {
		ps := scopes[i]
		if want.ScopeID != ps.scopeID {
			t.Fatalf("cassette scope[%d] = %q, want %q", i, want.ScopeID, ps.scopeID)
		}
		if want.ObservedAt != parityObservedAt.Format(time.RFC3339) &&
			want.ObservedAt != "2026-10-01T12:00:00Z" {
			t.Fatalf("cassette scope %q observed_at = %q, want fixed cut time", want.ScopeID, want.ObservedAt)
		}
		result, seen := parityLiveResult(t, ps)
		if result.State != sdk.ResultComplete && result.State != sdk.ResultPartial {
			t.Fatalf("live state = %q, want complete or partial for scope %q", result.State, want.ScopeID)
		}
		if result.Generation.ID != want.GenerationID {
			t.Fatalf("live GenerationID = %q, want %q", result.Generation.ID, want.GenerationID)
		}
		if result.Claim.Scope.ID != want.ScopeID {
			t.Fatalf("live ScopeID = %q, want %q", result.Claim.Scope.ID, want.ScopeID)
		}
		wantKinds := map[string]int{}
		wantKeys := make([]string, 0, len(want.Facts))
		wantPayloads := map[string]string{}
		for _, fact := range want.Facts {
			wantKinds[fact.FactKind]++
			wantKeys = append(wantKeys, fact.StableFactKey)
			wantPayloads[fact.StableFactKey] = parityCanonicalPayload(t, fact.Payload)
		}
		slices.Sort(wantKeys)
		if !maps.Equal(parityKindMultiset(result.Facts), wantKinds) {
			t.Fatalf("live kinds = %v, want %v for scope %q", parityKindMultiset(result.Facts), wantKinds, want.ScopeID)
		}
		if !slices.Equal(parityStableKeys(result.Facts), wantKeys) {
			t.Fatalf("live stable keys differ from cassette for scope %q", want.ScopeID)
		}
		gotPayloads := map[string]string{}
		for _, fact := range result.Facts {
			gotPayloads[fact.StableKey] = parityCanonicalPayload(t, fact.Payload)
			if fact.SchemaVersion != "1.0.0" {
				t.Fatalf("SchemaVersion = %q, want 1.0.0 for kind %q", fact.SchemaVersion, fact.Kind)
			}
			if fact.SourceConfidence != sdk.SourceConfidenceReported {
				t.Fatalf("SourceConfidence = %q, want reported", fact.SourceConfidence)
			}
			if fact.SourceRef.ScopeID != want.ScopeID {
				t.Fatalf("SourceRef.ScopeID = %q, want %q", fact.SourceRef.ScopeID, want.ScopeID)
			}
			if fact.SourceRef.GenerationID != want.GenerationID {
				t.Fatalf("SourceRef.GenerationID = %q, want %q", fact.SourceRef.GenerationID, want.GenerationID)
			}
			if strings.TrimSpace(fact.SourceRef.URI) == "" {
				t.Fatalf("SourceRef.URI is blank for kind %q", fact.Kind)
			}
		}
		if !maps.Equal(gotPayloads, wantPayloads) {
			t.Fatalf("live payloads differ from cassette for scope %q", want.ScopeID)
		}
		if ps.expectNoTarget && len(seen.targetPaths) != 0 {
			t.Fatalf("mimir requested targets %v, want no target calls", seen.targetPaths)
		}
		if ps.tenantID != "" {
			if len(seen.tenant) == 0 {
				t.Fatal("mimir rules request is missing, want tenant header observation")
			}
			for _, got := range seen.tenant {
				if got == "" {
					t.Fatal("mimir rules request omitted X-Scope-OrgID, want tenant header")
				}
			}
		}
	}

	union := map[string]bool{}
	for _, s := range recorded {
		for _, fact := range s.Facts {
			union[fact.FactKind] = true
		}
	}
	for _, kind := range []string{
		FactKindSourceInstance,
		FactKindObservedTarget,
		FactKindObservedRule,
		FactKindCoverageWarning,
	} {
		if !union[kind] {
			t.Fatalf("cassette covers no %q fact; parity pack must span all 4 live kinds", kind)
		}
	}
}
