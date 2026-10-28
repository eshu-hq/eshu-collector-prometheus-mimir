// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"errors"
	"strings"
	"testing"
)

// TestProviderAPIErrorText pins the bounded error vocabulary: no URL, token,
// or body ever renders into an error, only the API status and error type.
func TestProviderAPIErrorText(t *testing.T) {
	t.Parallel()

	if got := (ProviderAPIError{Status: "error", ErrorType: "bad_data"}).Error(); !strings.Contains(got, `"error"`) || !strings.Contains(got, "bad_data") {
		t.Fatalf("Error() = %q, want status and error type", got)
	}
	if got := (ProviderAPIError{ErrorType: "bad_data"}).Error(); !strings.Contains(got, `"error"`) {
		t.Fatalf("Error() = %q, want blank status to read as error", got)
	}
	if got := (ProviderAPIError{Status: "error"}).Error(); strings.Contains(got, ":") {
		t.Fatalf("Error() = %q, want no separator without an error type", got)
	}
	if got := (ProviderAPIError{Status: "error", Operation: "rules"}).FailureOperation(); got != "rules" {
		t.Fatalf("FailureOperation() = %q, want the bounded endpoint token", got)
	}
	if got := (ProviderAPIError{}).FailureOperation(); got != "" {
		t.Fatalf("FailureOperation() = %q, want blank without an operation", got)
	}
}

// TestDecodeReasonNamesTheConfigFault pins the config error vocabulary:
// wrong-type fields and unknown fields name the fault class without quoting
// the input.
func TestDecodeReasonNamesTheConfigFault(t *testing.T) {
	t.Parallel()

	if _, err := ParseSettings(map[string]any{"targets": "nope"}, "", nil); err == nil {
		t.Fatal("ParseSettings(wrong-type targets) error = nil, want a failure")
	} else if !strings.Contains(err.Error(), "wrong type") {
		t.Fatalf("ParseSettings error = %q, want the wrong-type reason", err)
	}
	if _, err := ParseSettings(map[string]any{"targets": []any{}, "bogus": true}, "", nil); err == nil {
		t.Fatal("ParseSettings(unknown field) error = nil, want a failure")
	} else if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ParseSettings error = %q, want the unknown-field reason", err)
	}
	if got := decodeReason(errors.New("boom")); got != "invalid JSON" {
		t.Fatalf("decodeReason = %q, want the fallback", got)
	}
}

// TestSensitiveQueryKeyMatchesCredentialNames pins the redaction vocabulary:
// known credential parameter names redact, ordinary names do not.
func TestSensitiveQueryKeyMatchesCredentialNames(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"token", " Token ", "API_KEY", "password", "secret", "auth"} {
		if !sensitiveQueryKey(key) {
			t.Errorf("sensitiveQueryKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"job", "instance", "", "totals"} {
		if sensitiveQueryKey(key) {
			t.Errorf("sensitiveQueryKey(%q) = true, want false", key)
		}
	}
}

// TestTargetConfigStringRedactsCredentials pins the anti-leak contract: both
// verbs name the scope and neither renders the token, tenant, or URL.
func TestTargetConfigStringRedactsCredentials(t *testing.T) {
	t.Parallel()

	target := TargetConfig{
		ScopeID: "prometheus:smoke:prod", BaseURL: "http://secret-host:9090",
		Token: "super-secret", TenantID: "tenant-acme",
	}
	for _, rendered := range []string{target.String(), target.GoString()} {
		if !strings.Contains(rendered, "prometheus:smoke:prod") {
			t.Fatalf("rendered = %q, want the scope", rendered)
		}
		for _, leaked := range []string{"super-secret", "tenant-acme", "secret-host"} {
			if strings.Contains(rendered, leaked) {
				t.Fatalf("rendered = %q, must not contain %q", rendered, leaked)
			}
		}
	}
}
