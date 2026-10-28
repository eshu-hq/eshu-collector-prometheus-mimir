// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"strings"
	"testing"
	"time"
)

// coverageEnvelopeContext returns a valid envelope every fact builder
// accepts, so each test below faults exactly one field.
func coverageEnvelopeContext() EnvelopeContext {
	return EnvelopeContext{
		ScopeID:             "scope-1",
		GenerationID:        "gen-1",
		CollectorInstanceID: "collector-1",
		SourceInstanceID:    "source-1",
		SourceKind:          SourceKindPrometheus,
		SourceSystem:        SourceSystem,
		SourceURI:           "http://127.0.0.1:9090",
		ObservedAt:          time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	}
}

// TestNormalizedOutcomeBounds every provider outcome to the contract
// vocabulary: known values pass through, anything else reads observed.
func TestNormalizedOutcomeBounds(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]string{
		OutcomePartial:          OutcomePartial,
		OutcomePermissionHidden: OutcomePermissionHidden,
		OutcomeUnsupported:      OutcomeUnsupported,
		OutcomeRejected:         OutcomeRejected,
		OutcomeStale:            OutcomeStale,
		"":                      OutcomeObserved,
		"  observed  ":          OutcomeObserved,
		"something-new":         OutcomeObserved,
	} {
		if got := normalizedOutcome(value); got != want {
			t.Errorf("normalizedOutcome(%q) = %q, want %q", value, got, want)
		}
	}
}

// TestNormalizedFreshnessBounds every freshness value: known states pass
// through, anything else reads current.
func TestNormalizedFreshnessBounds(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]string{
		FreshnessUnknown:          FreshnessUnknown,
		FreshnessStale:            FreshnessStale,
		FreshnessPermissionHidden: FreshnessPermissionHidden,
		"":                        FreshnessCurrent,
		"current":                 FreshnessCurrent,
		"something-new":           FreshnessCurrent,
	} {
		if got := normalizedFreshness(value); got != want {
			t.Errorf("normalizedFreshness(%q) = %q, want %q", value, got, want)
		}
	}
}

// TestWarningOutcomeBounds every warning reason to its outcome, including
// the default that keeps an unknown reason partial, never terminal.
func TestWarningOutcomeBounds(t *testing.T) {
	t.Parallel()

	for reason, want := range map[string]string{
		WarningPermissionHidden:       OutcomePermissionHidden,
		WarningUnsupported:            OutcomeUnsupported,
		WarningManualProviderResource: OutcomeObserved,
		WarningStale:                  OutcomeStale,
		WarningPartial:                OutcomePartial,
		"something-new":               OutcomePartial,
	} {
		if got := warningOutcome(reason); got != want {
			t.Errorf("warningOutcome(%q) = %q, want %q", reason, got, want)
		}
	}
}

// TestValidateEnvelopeContextRejectsEachBlank pins the four identity
// rejections so a fact can never lose its scope, generation, collector, or
// source binding silently.
func TestValidateEnvelopeContextRejectsEachBlank(t *testing.T) {
	t.Parallel()

	if err := validateEnvelopeContext(coverageEnvelopeContext()); err != nil {
		t.Fatalf("validateEnvelopeContext(valid) error = %v, want nil", err)
	}
	cases := map[string]func(*EnvelopeContext){
		"scope":      func(ctx *EnvelopeContext) { ctx.ScopeID = "  " },
		"generation": func(ctx *EnvelopeContext) { ctx.GenerationID = "" },
		"collector":  func(ctx *EnvelopeContext) { ctx.CollectorInstanceID = "" },
		"source":     func(ctx *EnvelopeContext) { ctx.SourceInstanceID = "" },
	}
	for name, fault := range cases {
		ctx := coverageEnvelopeContext()
		fault(&ctx)
		if err := validateEnvelopeContext(ctx); err == nil {
			t.Errorf("validateEnvelopeContext(blank %s) error = nil, want a failure", name)
		}
	}
}

// TestSanitizeURLShapes pins the credential-stripping contract: userinfo,
// sensitive query keys, and fragments go; ordinary parameters stay.
func TestSanitizeURLShapes(t *testing.T) {
	t.Parallel()

	if got := sanitizeURL(""); got != "" {
		t.Errorf("sanitizeURL(blank) = %q, want empty", got)
	}
	if got := sanitizeURL("/api/v1/targets"); got != "" {
		t.Errorf("sanitizeURL(relative) = %q, want empty", got)
	}
	got := sanitizeURL("https://user:pass@metrics:9090/api?token=secret&job=node#frag")
	if strings.Contains(got, "user") || strings.Contains(got, "secret") || strings.Contains(got, "frag") {
		t.Errorf("sanitizeURL() = %q, must drop credentials, secrets, and fragments", got)
	}
	if !strings.Contains(got, "job=node") {
		t.Errorf("sanitizeURL() = %q, want the ordinary parameter kept", got)
	}
	if !strings.HasPrefix(got, "https://metrics:9090/api") {
		t.Errorf("sanitizeURL() = %q, want scheme, host, and path kept", got)
	}
}

// TestNormalizedObservedAtKeepsClockHonest pins both branches: a zero
// observation reads the wall clock, a carried timestamp converts to UTC.
func TestNormalizedObservedAtKeepsClockHonest(t *testing.T) {
	t.Parallel()

	if got := normalizedObservedAt(time.Time{}); got.IsZero() {
		t.Error("normalizedObservedAt(zero) is zero, want the wall clock")
	}
	fixed := time.Date(2026, 9, 30, 12, 0, 0, 0, time.FixedZone("CET", 3600))
	if got := normalizedObservedAt(fixed); !got.Equal(fixed) || got.Location() != time.UTC {
		t.Errorf("normalizedObservedAt() = %v, want the same instant in UTC", got)
	}
}

// TestSetURLRedactionShapes pins the three presence branches: absent and
// unredacted writes nothing, a carried value fingerprints, and a redacted
// blank still marks the field redacted without a fingerprint.
func TestSetURLRedactionShapes(t *testing.T) {
	t.Parallel()

	absent := map[string]any{}
	setURLRedaction(absent, "scrape_url", "", false)
	if len(absent) != 0 {
		t.Errorf("setURLRedaction(blank, false) wrote %v, want nothing", absent)
	}
	present := map[string]any{}
	setURLRedaction(present, "scrape_url", "http://metrics:9090/metrics", false)
	if present["scrape_url_present"] != true || present["scrape_url_redacted"] != true {
		t.Errorf("setURLRedaction(value) = %v, want present and redacted", present)
	}
	if present["scrape_url_fingerprint"] != fingerprint("http://metrics:9090/metrics") {
		t.Errorf("setURLRedaction(value) = %v, want the value fingerprint", present)
	}
	blankRedacted := map[string]any{}
	setURLRedaction(blankRedacted, "scrape_url", "  ", true)
	if blankRedacted["scrape_url_present"] != false || blankRedacted["scrape_url_redacted"] != true {
		t.Errorf("setURLRedaction(blank, true) = %v, want absent but redacted", blankRedacted)
	}
	if _, ok := blankRedacted["scrape_url_fingerprint"]; ok {
		t.Errorf("setURLRedaction(blank, true) = %v, want no fingerprint for a blank", blankRedacted)
	}
}

// TestSetRedactionStateShapes pins the state derivation: a true redaction
// flag wins, a bare fingerprint also marks applied, and a clean payload
// reads none.
func TestSetRedactionStateShapes(t *testing.T) {
	t.Parallel()

	flagged := map[string]any{"last_error_redacted": true}
	setRedactionState(flagged)
	if flagged["redaction_state"] != "applied" {
		t.Errorf("redaction_state = %v, want applied for a redaction flag", flagged)
	}
	fingerprinted := map[string]any{"tenant_fingerprint": "sha256:abc"}
	setRedactionState(fingerprinted)
	if fingerprinted["redaction_state"] != "applied" {
		t.Errorf("redaction_state = %v, want applied for a fingerprint", fingerprinted)
	}
	clean := map[string]any{"health": "up"}
	setRedactionState(clean)
	if clean["redaction_state"] != "none" {
		t.Errorf("redaction_state = %v, want none for a clean payload", clean)
	}
}

// TestFirstNonBlankShapes pins the fallback chain the envelope uses for
// collector identity.
func TestFirstNonBlankShapes(t *testing.T) {
	t.Parallel()

	if got := firstNonBlank("", "  ", "instance-1", "component-1"); got != "instance-1" {
		t.Errorf("firstNonBlank() = %q, want the first non-blank", got)
	}
	if got := firstNonBlank("", "   "); got != "" {
		t.Errorf("firstNonBlank(all blank) = %q, want empty", got)
	}
}

// TestConfigFailureClassDefaults pins the rejection class: a bare config
// error reads invalid_config, an explicit class survives.
func TestConfigFailureClassDefaults(t *testing.T) {
	t.Parallel()

	if got := (ConfigError{}).FailureClass(); got != FailureInvalidConfig {
		t.Errorf("FailureClass() = %q, want %q", got, FailureInvalidConfig)
	}
	if got := (ConfigError{class: "custom"}).FailureClass(); got != "custom" {
		t.Errorf("FailureClass() = %q, want the explicit class", got)
	}
}
