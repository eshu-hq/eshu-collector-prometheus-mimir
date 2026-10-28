// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"

	collector "github.com/eshu-hq/eshu-collector-prometheus-mimir"
)

// stubPrometheus serves one healthy Prometheus target and one rule group: the
// smallest provider surface that still yields source-instance, target, and
// rule facts through runClaim.
func stubPrometheus(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/targets":
			_, _ = w.Write([]byte(`{"status":"success","data":{"activeTargets":[{` +
				`"discoveredLabels":{"__address__":"10.0.0.1:9100"},` +
				`"labels":{"instance":"node-1","job":"node"},` +
				`"scrapePool":"nodes",` +
				`"scrapeUrl":"http://10.0.0.1:9100/metrics",` +
				`"health":"up",` +
				`"lastScrape":"2026-09-30T12:00:00Z"}]}}`))
		case "/api/v1/rules":
			_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[{` +
				`"name":"smoke.rules","file":"rules/smoke.yaml","rules":[{` +
				`"name":"SmokeBurn","type":"alerting","health":"ok",` +
				`"query":"sum(rate(smoke_total[5m])) > 7",` +
				`"lastEvaluation":"2026-09-30T12:00:00Z"}]}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func smokeClaim() sdk.Claim {
	return sdk.Claim{
		ComponentID:   collector.ComponentID,
		InstanceID:    "collector-prometheus-smoke",
		CollectorKind: collector.CollectorKind,
		SourceSystem:  collector.SourceSystem,
		Scope:         sdk.Scope{ID: "prometheus:smoke:prod", Kind: collector.ScopeKindMetricSource},
		SourceRunID:   "smoke-run",
		GenerationID:  "prometheus-mimir:smoke:1",
		WorkItemID:    "smoke-work",
		FencingToken:  "7",
		Attempt:       1,
		Deadline:      time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		ConfigHandle:  "smoke-config",
	}
}

func smokeConfig(baseURL string) map[string]any {
	return map[string]any{"targets": []any{map[string]any{
		"provider":       collector.ProviderPrometheus,
		"scope_id":       "prometheus:smoke:prod",
		"instance_id":    "prometheus-smoke",
		"base_url":       baseURL,
		"resource_limit": 50,
		"enabled":        true,
	}}}
}

func smokeRequest(t *testing.T, claim sdk.Claim, config map[string]any) []byte {
	t.Helper()
	request := collector.Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        collector.Contract(),
		Config:          config,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal(request) error = %v", err)
	}
	return raw
}

func fixedNow() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}

func noEnv(string) string { return "" }

// TestRunClaimServesOneRequestAsOneResult proves the stdin/stdout contract end
// to end through the command: one Request in, one Result out with facts, and
// the claim echoed back for the host identity check. Both arg shapes (bare
// and --sdk-stdio) behave the same.
func TestRunClaimServesOneRequestAsOneResult(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"--sdk-stdio"}} {
		server := stubPrometheus(t)
		var stdout, stderr bytes.Buffer
		err := runClaim(context.Background(), args,
			bytes.NewReader(smokeRequest(t, smokeClaim(), smokeConfig(server.URL))),
			&stdout, &stderr, noEnv, fixedNow)
		if err != nil {
			t.Fatalf("runClaim(%v) error = %v, stderr = %s", args, err, stderr.String())
		}
		var result sdk.Result
		decoder := json.NewDecoder(&stdout)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if result.State != sdk.ResultComplete && result.State != sdk.ResultPartial {
			t.Fatalf("result state = %q, want complete or partial", result.State)
		}
		if result.ProtocolVersion != sdk.ProtocolVersionV1Alpha1 {
			t.Fatalf("protocol version = %q, want %q", result.ProtocolVersion, sdk.ProtocolVersionV1Alpha1)
		}
		if result.Claim.GenerationID != "prometheus-mimir:smoke:1" {
			t.Fatalf("claim echo generation = %q, want the request claim", result.Claim.GenerationID)
		}
		byKind := map[string]int{}
		for _, fact := range result.Facts {
			byKind[fact.Kind]++
		}
		for _, kind := range []string{
			collector.FactKindSourceInstance,
			collector.FactKindObservedTarget,
			collector.FactKindObservedRule,
		} {
			if byKind[kind] == 0 {
				t.Fatalf("result facts = %v, want a %s fact", result.Facts, kind)
			}
		}
	}
}

// TestRunClaimRejectsBadInput proves malformed host input fails the command
// before any provider contact: no partial result is written to stdout.
func TestRunClaimRejectsBadInput(t *testing.T) {
	t.Parallel()

	claim := smokeClaim()
	valid, err := json.Marshal(collector.Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        collector.Contract(),
		Config:          smokeConfig("http://127.0.0.1:1"),
	})
	if err != nil {
		t.Fatalf("json.Marshal(request) error = %v", err)
	}
	badClaim := claim
	badClaim.GenerationID = ""
	badClaimJSON, err := json.Marshal(collector.Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           badClaim,
		Contract:        collector.Contract(),
		Config:          smokeConfig("http://127.0.0.1:1"),
	})
	if err != nil {
		t.Fatalf("json.Marshal(bad request) error = %v", err)
	}

	for name, input := range map[string]struct {
		args  []string
		stdin string
	}{
		"malformed JSON":     {nil, "{not json"},
		"trailing JSON":      {nil, string(valid) + " {}"},
		"unknown field":      {nil, `{"protocol_version":"collector-sdk/v1alpha1","bogus":true}`},
		"wrong protocol":     {nil, `{"protocol_version":"v9"}`},
		"missing claim data": {nil, string(badClaimJSON)},
		"extra argument":     {[]string{"surprise"}, string(valid)},
		"bad flag":           {[]string{"--nope"}, string(valid)},
	} {
		var stdout, stderr bytes.Buffer
		err := runClaim(context.Background(), input.args,
			strings.NewReader(input.stdin), &stdout, &stderr, noEnv, fixedNow)
		if err == nil {
			t.Errorf("%s: runClaim error = nil, want a failure", name)
		}
		if stdout.Len() != 0 {
			t.Errorf("%s: stdout = %d bytes, want nothing written on input failure", name, stdout.Len())
		}
	}
}

// TestRunCoversTheCommandWrapper proves run delegates to the claim path with
// the wall clock: one valid Request in still yields one Result out.
func TestRunCoversTheCommandWrapper(t *testing.T) {
	t.Parallel()

	server := stubPrometheus(t)
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil,
		bytes.NewReader(smokeRequest(t, smokeClaim(), smokeConfig(server.URL))),
		&stdout, &stderr, noEnv)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	var result sdk.Result
	if err := json.NewDecoder(&stdout).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.State != sdk.ResultComplete && result.State != sdk.ResultPartial {
		t.Fatalf("result state = %q, want complete or partial", result.State)
	}
}

// TestRunClaimRejectsCollectorKindMismatch proves a claim naming another
// collector kind fails closed at the parse boundary: the command exits
// non-zero with no partial result, so one producer can never emit under a
// sibling's authority. (The Handler-level terminal Result for the same case
// is pinned by TestPrometheusMimir8864ParityCollectorKindRejected.)
func TestRunClaimRejectsCollectorKindMismatch(t *testing.T) {
	t.Parallel()

	claim := smokeClaim()
	claim.CollectorKind = "loki"
	var stdout, stderr bytes.Buffer
	err := runClaim(context.Background(), nil,
		bytes.NewReader(smokeRequest(t, claim, smokeConfig("http://127.0.0.1:1"))),
		&stdout, &stderr, noEnv, fixedNow)
	if err == nil {
		t.Fatal("runClaim error = nil, want collector-kind rejection")
	}
	if !strings.Contains(err.Error(), "collector_kind") {
		t.Fatalf("runClaim error = %v, want it to name collector_kind", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %d bytes, want nothing written on kind rejection", stdout.Len())
	}
}
