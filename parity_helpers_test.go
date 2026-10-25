// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"encoding/json"
	"testing"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// warningReasons counts coverage-warning facts by warning_kind.
func warningReasons(facts []sdk.Fact) map[string]int {
	reasons := map[string]int{}
	for _, fact := range facts {
		if fact.Kind != FactKindCoverageWarning {
			continue
		}
		if reason, ok := fact.Payload["warning_kind"].(string); ok {
			reasons[reason]++
		}
	}
	return reasons
}

// anyParityJSON renders a value as JSON for redaction assertions.
func anyParityJSON(t *testing.T, value any) string {
	t.Helper()
	rendered, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload error = %v, want nil", err)
	}
	return string(rendered)
}
