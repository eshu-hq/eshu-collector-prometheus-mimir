// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector/transport"
)

// coverageTargetMap returns a valid activation target entry for scope.
func coverageTargetMap(scope string) map[string]any {
	return map[string]any{
		"provider":    "prometheus",
		"scope_id":    scope,
		"instance_id": "inst-" + scope,
		"base_url":    "http://127.0.0.1:9090",
	}
}

// TestOptionalEnvSeams pins the credential seams: blank names resolve
// empty, a missing getter or blank value fails closed, a carried value
// trims.
func TestOptionalEnvSeams(t *testing.T) {
	t.Parallel()

	if value, err := optionalEnvValue("  ", nil, "token_env"); err != nil || value != "" {
		t.Errorf("optionalEnvValue(blank) = %q, %v; want empty, nil", value, err)
	}
	if _, err := optionalEnvValue("TOKEN", nil, "token_env"); err == nil {
		t.Error("optionalEnvValue(nil getter) error = nil, want fail-closed")
	}
	blank := func(string) string { return "  " }
	if _, err := optionalEnvValue("TOKEN", blank, "token_env"); err == nil {
		t.Error("optionalEnvValue(blank value) error = nil, want fail-closed")
	}
	full := func(string) string { return "  bearer-1  " }
	if value, err := optionalEnvValue("TOKEN", full, "token_env"); err != nil || value != "bearer-1" {
		t.Errorf("optionalEnvValue(value) = %q, %v; want trimmed, nil", value, err)
	}

	if value, err := optionalEnvOverride("  raw-1 ", "", nil, "tenant_id_env"); err != nil || value != "raw-1" {
		t.Errorf("optionalEnvOverride(raw) = %q, %v; want trimmed raw, nil", value, err)
	}
	if _, err := optionalEnvOverride("raw-1", "TENANT", nil, "tenant_id_env"); err == nil {
		t.Error("optionalEnvOverride(nil getter) error = nil, want fail-closed")
	}
	if _, err := optionalEnvOverride("raw-1", "TENANT", blank, "tenant_id_env"); err == nil {
		t.Error("optionalEnvOverride(blank value) error = nil, want fail-closed")
	}
	if value, err := optionalEnvOverride("raw-1", "TENANT", full, "tenant_id_env"); err != nil || value != "bearer-1" {
		t.Errorf("optionalEnvOverride(env) = %q, %v; want the env value, nil", value, err)
	}
}

// TestParseOptionalDurationShapes pins the stale_after seam: blank means
// no staleness window, garbage fails, a Go duration parses.
func TestParseOptionalDurationShapes(t *testing.T) {
	t.Parallel()

	if value, err := parseOptionalDuration("  ", "stale_after"); err != nil || value != 0 {
		t.Errorf("parseOptionalDuration(blank) = %v, %v; want 0, nil", value, err)
	}
	if _, err := parseOptionalDuration("bogus", "stale_after"); err == nil {
		t.Error("parseOptionalDuration(bogus) error = nil, want a failure")
	}
	if value, err := parseOptionalDuration("90s", "stale_after"); err != nil || value != 90*time.Second {
		t.Errorf("parseOptionalDuration(90s) = %v, %v; want 90s, nil", value, err)
	}
}

// TestValidateTargetRejectsFaults pins every target rejection so a bad
// activation fails at parse time, never at collection time.
func TestValidateTargetRejectsFaults(t *testing.T) {
	t.Parallel()

	valid := TargetConfig{
		Provider: "prometheus", ScopeID: "s", InstanceID: "i",
		BaseURL: "http://127.0.0.1:9090",
	}
	if _, err := validateTarget(valid); err != nil {
		t.Fatalf("validateTarget(valid) error = %v, want nil", err)
	}
	badProvider := valid
	badProvider.Provider = "datadog"
	if _, err := validateTarget(badProvider); err == nil {
		t.Error("validateTarget(bad provider) error = nil, want a failure")
	}
	for name, fault := range map[string]func(*TargetConfig){
		"blank scope":    func(target *TargetConfig) { target.ScopeID = " " },
		"blank instance": func(target *TargetConfig) { target.InstanceID = "" },
		"blank base URL": func(target *TargetConfig) { target.BaseURL = " " },
		"negative limit": func(target *TargetConfig) { target.ResourceLimit = -1 },
		"huge limit":     func(target *TargetConfig) { target.ResourceLimit = maxResourceLimit + 1 },
	} {
		broken := valid
		fault(&broken)
		if _, err := validateTarget(broken); err == nil {
			t.Errorf("validateTarget(%s) error = nil, want a failure", name)
		}
	}
	unlimited := valid
	mapped, err := validateTarget(unlimited)
	if err != nil {
		t.Fatalf("validateTarget(zero limit) error = %v, want nil", err)
	}
	if mapped.ResourceLimit != defaultResourceLimit {
		t.Errorf("ResourceLimit = %d, want the default %d", mapped.ResourceLimit, defaultResourceLimit)
	}
}

// TestMapActivationTargetEnvFaults pins the env failure paths: a named
// variable that resolves nothing fails the target, as does a bad duration.
func TestMapActivationTargetEnvFaults(t *testing.T) {
	t.Parallel()

	tokenItem := activationTargetJSON{
		Provider: "prometheus", ScopeID: "s", InstanceID: "i",
		BaseURL: "http://127.0.0.1:9090", TokenEnv: "TOKEN",
	}
	if _, err := mapActivationTarget(tokenItem, nil); err == nil {
		t.Error("mapActivationTarget(unresolved token_env) error = nil, want a failure")
	}
	tenantItem := activationTargetJSON{
		Provider: "prometheus", ScopeID: "s", InstanceID: "i",
		BaseURL: "http://127.0.0.1:9090", TenantIDEnv: "TENANT",
	}
	if _, err := mapActivationTarget(tenantItem, nil); err == nil {
		t.Error("mapActivationTarget(unresolved tenant_id_env) error = nil, want a failure")
	}
	durationItem := activationTargetJSON{
		Provider: "prometheus", ScopeID: "s", InstanceID: "i",
		BaseURL: "http://127.0.0.1:9090", StaleAfter: "bogus",
	}
	if _, err := mapActivationTarget(durationItem, nil); err == nil {
		t.Error("mapActivationTarget(bad stale_after) error = nil, want a failure")
	}
}

// TestParseSettingsBoundsScopeSelection pins the scope gate: no targets,
// no enabled targets, duplicates, and an unconfigured claim scope all fail.
func TestParseSettingsBoundsScopeSelection(t *testing.T) {
	t.Parallel()

	if _, err := ParseSettings(map[string]any{}, "", nil); err == nil {
		t.Error("ParseSettings(no targets) error = nil, want a failure")
	}
	disabled := coverageTargetMap("s1")
	disabled["enabled"] = false
	if _, err := ParseSettings(map[string]any{"targets": []any{disabled}}, "", nil); err == nil {
		t.Error("ParseSettings(all disabled) error = nil, want a failure")
	}
	duplicates := map[string]any{"targets": []any{coverageTargetMap("s1"), coverageTargetMap("s1")}}
	if _, err := ParseSettings(duplicates, "", nil); err == nil {
		t.Error("ParseSettings(duplicate scope) error = nil, want a failure")
	}
	single := map[string]any{"targets": []any{coverageTargetMap("s1")}}
	if _, err := ParseSettings(single, "nope", nil); err == nil {
		t.Error("ParseSettings(unconfigured claim scope) error = nil, want a failure")
	}
	if _, err := ParseSettings(single, "s1", nil); err != nil {
		t.Errorf("ParseSettings(matching scope) error = %v, want nil", err)
	}
}

// TestClassifiedProviderFailureBounds pins the failure taxonomy: bad_data
// is terminal, other API errors stay retryable with their operation token,
// auth-denied keeps its class, and unknown errors fall back to retryable.
func TestClassifiedProviderFailureBounds(t *testing.T) {
	t.Parallel()

	badData := classifiedProviderFailure(ProviderAPIError{Status: "error", ErrorType: "bad_data", Operation: "list_rules"})
	if !badData.TerminalFailure() || badData.FailureClass() != FailureTerminal {
		t.Errorf("bad_data = class %q terminal %v, want terminal %q",
			badData.FailureClass(), badData.TerminalFailure(), FailureTerminal)
	}
	transient := classifiedProviderFailure(ProviderAPIError{Status: "error", ErrorType: "timeout", Operation: "list_targets"})
	if transient.TerminalFailure() || transient.FailureClass() != FailureRetryable {
		t.Errorf("timeout = class %q terminal %v, want retryable non-terminal",
			transient.FailureClass(), transient.TerminalFailure())
	}
	if got := transient.FailureOperation(); got != "list_targets" {
		t.Errorf("FailureOperation() = %q, want the bounded endpoint token", got)
	}
	denied := classifiedProviderFailure(transport.HTTPError{Provider: "metric", StatusCode: 401})
	if denied.FailureClass() != FailureAuthDenied {
		t.Errorf("401 = class %q, want %q", denied.FailureClass(), FailureAuthDenied)
	}
	generic := classifiedProviderFailure(errors.New("boom"))
	if generic.TerminalFailure() || generic.FailureClass() != FailureRetryable {
		t.Errorf("generic = class %q terminal %v, want retryable non-terminal",
			generic.FailureClass(), generic.TerminalFailure())
	}
}

// TestPrometheusOperationForEndpointBounds the failure operation token:
// known endpoints map, unknown endpoints stay blank for the stage fallback.
func TestPrometheusOperationForEndpointBounds(t *testing.T) {
	t.Parallel()

	if got := prometheusOperationForEndpoint(targetsEndpoint); got != "list_targets" {
		t.Errorf("operation(targets) = %q, want list_targets", got)
	}
	if got := prometheusOperationForEndpoint(rulesEndpoint); got != "list_rules" {
		t.Errorf("operation(rules) = %q, want list_rules", got)
	}
	if got := prometheusOperationForEndpoint("/api/v1/new"); got != "" {
		t.Errorf("operation(unknown) = %q, want blank", got)
	}
}

// TestNormalizedProviderBounds the provider vocabulary: blank reads
// Prometheus, Mimir survives casing and padding, anything else passes
// through for the caller to reject.
func TestNormalizedProviderBounds(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]string{
		"":           ProviderPrometheus,
		"prometheus": ProviderPrometheus,
		"PROMETHEUS": ProviderPrometheus,
		"mimir":      ProviderMimir,
		" Mimir ":    ProviderMimir,
		"datadog":    "datadog",
	} {
		if got := normalizedProvider(value); got != want {
			t.Errorf("normalizedProvider(%q) = %q, want %q", value, got, want)
		}
	}
}

// TestNormalizedResourceLimitBounds the client-side cap: non-positive
// reads the default, over-max clamps, in-range survives.
func TestNormalizedResourceLimitBounds(t *testing.T) {
	t.Parallel()

	for value, want := range map[int]int{
		0:                    defaultResourceLimit,
		-5:                   defaultResourceLimit,
		maxResourceLimit + 1: maxResourceLimit,
		50:                   50,
	} {
		if got := normalizedResourceLimit(value); got != want {
			t.Errorf("normalizedResourceLimit(%d) = %d, want %d", value, got, want)
		}
	}
}

// TestParseProviderTimeShapes pins the timestamp seam: blank and garbage
// read zero, RFC3339Nano parses to UTC.
func TestParseProviderTimeShapes(t *testing.T) {
	t.Parallel()

	if got := parseProviderTime("  "); !got.IsZero() {
		t.Errorf("parseProviderTime(blank) = %v, want zero", got)
	}
	if got := parseProviderTime("not-a-time"); !got.IsZero() {
		t.Errorf("parseProviderTime(garbage) = %v, want zero", got)
	}
	got := parseProviderTime("2026-09-30T12:00:00Z")
	want := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseProviderTime() = %v, want %v", got, want)
	}
}

// TestAPIStatusShapes pins the status reader: nil reads blank, values
// trim.
func TestAPIStatusShapes(t *testing.T) {
	t.Parallel()

	var missing *apiResponse[targetsData]
	if status, errorType := missing.apiStatus(); status != "" || errorType != "" {
		t.Errorf("nil apiStatus() = %q, %q; want blanks", status, errorType)
	}
	response := &apiResponse[targetsData]{Status: " success ", ErrorType: " bad_data "}
	if status, errorType := response.apiStatus(); status != "success" || errorType != "bad_data" {
		t.Errorf("apiStatus() = %q, %q; want trimmed values", status, errorType)
	}
}

// TestIdentitiesBoundBlankInputs pins the identity floors: fully blank
// target and rule inputs carry no identity so the collector drops them,
// and a query-only rule fingerprints instead of leaking PromQL.
func TestIdentitiesBoundBlankInputs(t *testing.T) {
	t.Parallel()

	if got := targetIdentity(targetResource{}, nil, nil); got != "" {
		t.Errorf("targetIdentity(blank) = %q, want empty", got)
	}
	identified := targetIdentity(targetResource{ScrapePool: "node"}, []string{"job"}, nil)
	if identified == "" {
		t.Error("targetIdentity(pool) is empty, want a fingerprint")
	}
	for name, got := range map[string]string{
		"group and rule": ruleIdentity("g", "r", "q"),
		"group only":     ruleIdentity("g", "", ""),
		"rule only":      ruleIdentity("", "r", ""),
	} {
		if got == "" {
			t.Errorf("ruleIdentity(%s) is empty, want an identity", name)
		}
	}
	if got, want := ruleIdentity("g", "r", "q"), "g:r"; got != want {
		t.Errorf("ruleIdentity(group, rule) = %q, want %q", got, want)
	}
	if got := ruleIdentity("g", "", ""); got != "group:g" {
		t.Errorf("ruleIdentity(group only) = %q, want group:g", got)
	}
	if got := ruleIdentity("", "r", ""); got != "rule:r" {
		t.Errorf("ruleIdentity(rule only) = %q, want rule:r", got)
	}
	if got, want := ruleIdentity("", "", "up == 0"), fingerprint("up == 0"); got != want {
		t.Errorf("ruleIdentity(query only) = %q, want the query fingerprint %q", got, want)
	}
	if got := ruleIdentity("", "", "  "); got != "" {
		t.Errorf("ruleIdentity(blank) = %q, want empty", got)
	}
}

// TestHandlerSeamsDefaultHonestly pins the zero-value Handler: nil seams
// read the process environment and wall clock, an explicit factory wins,
// and latency clamps both ways.
func TestHandlerSeamsDefaultHonestly(t *testing.T) {
	t.Parallel()

	var zero Handler
	if got := zero.getenv("ESHU_TEST_MISSING_SEAM"); got != "" {
		t.Errorf("getenv(nil) = %q, want empty", got)
	}
	stubbed := Handler{Getenv: func(string) string { return "v" }}
	if got := stubbed.getenv("K"); got != "v" {
		t.Errorf("getenv(stub) = %q, want v", got)
	}
	before := time.Now().Add(-time.Minute)
	if got := zero.now(); got.Before(before) || got.Location() != time.UTC {
		t.Errorf("now(nil) = %v, want a recent UTC time", got)
	}
	fixed := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	pinned := Handler{Now: func() time.Time { return fixed }}
	if got := pinned.now(); !got.Equal(fixed) {
		t.Errorf("now(stub) = %v, want %v", got, fixed)
	}
	if client, err := zero.newClient(TargetConfig{BaseURL: "http://127.0.0.1:9"}); err != nil || client == (HTTPClient{}) {
		t.Errorf("newClient(nil factory) = %v, %v; want the HTTP client", client, err)
	}
	for duration, want := range map[time.Duration]int{
		-time.Second:            0,
		0:                       0,
		1500 * time.Millisecond: 1500,
		time.Duration(1 << 62):  1<<31 - 1,
	} {
		if got := latencyMillis(duration); got != want {
			t.Errorf("latencyMillis(%v) = %d, want %d", duration, got, want)
		}
	}
}

// TestParseRequestRejectsOversize pins the stdin cap: a document past
// MaxRequestBytes fails without decoding.
func TestParseRequestRejectsOversize(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Repeat("x", MaxRequestBytes+10))
	if _, err := ParseRequest(input); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("ParseRequest(oversize) error = %v, want the size rejection", err)
	}
}
