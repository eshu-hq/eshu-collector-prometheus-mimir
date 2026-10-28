// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// marshalSmokeRequest returns one valid stdio request document.
func marshalSmokeRequest() (string, error) {
	raw, err := json.Marshal(Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim: sdk.Claim{
			ComponentID:   ComponentID,
			InstanceID:    "collector-prometheus-smoke",
			CollectorKind: CollectorKind,
			SourceSystem:  SourceSystem,
			Scope:         sdk.Scope{ID: "prometheus:smoke:prod", Kind: ScopeKindMetricSource},
			SourceRunID:   "smoke-run",
			GenerationID:  "prometheus-mimir:smoke:1",
			WorkItemID:    "smoke-work",
			FencingToken:  "7",
			Attempt:       1,
			Deadline:      time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
			ConfigHandle:  "smoke-config",
		},
		Contract: Contract(),
		Config:   map[string]any{},
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// TestParseRequestAcceptsAValidRequest proves the documented wire shape
// decodes and echoes the claim identity the result must carry back.
func TestParseRequestAcceptsAValidRequest(t *testing.T) {
	t.Parallel()

	base, err := marshalSmokeRequest()
	if err != nil {
		t.Fatal(err)
	}
	request, err := ParseRequest(strings.NewReader(base))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if request.Claim.GenerationID != "prometheus-mimir:smoke:1" {
		t.Fatalf("GenerationID = %q, want the request claim", request.Claim.GenerationID)
	}
}

// TestParseRequestRejectsFaults proves each malformed shape fails with a
// bounded reason that never quotes the input.
func TestParseRequestRejectsFaults(t *testing.T) {
	t.Parallel()

	base, err := marshalSmokeRequest()
	if err != nil {
		t.Fatal(err)
	}
	badKind := strings.Replace(base, `"collector_kind":"prometheus_mimir"`, `"collector_kind":"loki"`, 1)
	noGeneration := strings.Replace(base, `"generation_id":"prometheus-mimir:smoke:1",`, ``, 1)

	for name, input := range map[string]string{
		"malformed JSON":  "{not json",
		"trailing JSON":   base + " {}",
		"unknown field":   strings.Replace(base, `"config_handle"`, `"bogus_field"`, 1),
		"wrong protocol":  strings.Replace(base, `"protocol_version":"collector-sdk/v1alpha1"`, `"protocol_version":"v9"`, 1),
		"wrong kind":      badKind,
		"missing field":   noGeneration,
		"empty collector": `{"protocol_version":"collector-sdk/v1alpha1","claim":{},"contract":{"protocol_version":"collector-sdk/v1alpha1"}}`,
	} {
		request, err := ParseRequest(strings.NewReader(input))
		if err == nil {
			t.Errorf("%s: ParseRequest() error = nil, want a failure", name)
		}
		if err != nil && strings.Contains(err.Error(), "smoke") {
			t.Errorf("%s: ParseRequest() error = %q, must not quote the input", name, err)
		}
		_ = request
	}
}
