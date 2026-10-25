// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// TestPrometheusMimir8864ParityDuplicateDeliveryIsStable pins duplicate
// safety: two identical claims emit identical stable-key sets, so a
// redelivered generation commits idempotently at the host boundary.
func TestPrometheusMimir8864ParityDuplicateDeliveryIsStable(t *testing.T) {
	t.Parallel()

	scopes := parityScopes()
	first, _ := parityLiveResult(t, scopes[0])
	second, _ := parityLiveResult(t, scopes[0])
	if first.State != sdk.ResultComplete && first.State != sdk.ResultPartial {
		t.Fatalf("first state = %q, want complete or partial", first.State)
	}
	if !slices.Equal(parityStableKeys(first.Facts), parityStableKeys(second.Facts)) {
		t.Fatal("duplicate claims emit different stable keys; redelivery would not be idempotent")
	}
}

// TestPrometheusMimir8864ParityCollectorKindRejected pins kind scoping: a
// claim naming another collector kind never collects, so one producer
// cannot emit under a sibling's authority.
func TestPrometheusMimir8864ParityCollectorKindRejected(t *testing.T) {
	t.Parallel()

	ps := parityScopes()[0]
	mux := http.NewServeMux()
	ps.serve(t, mux, &paritySeen{})
	server := httptest.NewServer(mux)
	defer server.Close()

	claim := parityClaim(ps)
	claim.CollectorKind = "loki"
	handler := Handler{}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          parityConfig(ps, server.URL),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("state = %q, want terminal for collector-kind rejection", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts = %d, want 0 for collector-kind rejection", len(result.Facts))
	}
}
