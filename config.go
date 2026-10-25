// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// activationTargetJSON is the operator shape for one metric target. Secrets
// travel by env reference, never inline.
type activationTargetJSON struct {
	Provider         string   `json:"provider"`
	ScopeID          string   `json:"scope_id"`
	InstanceID       string   `json:"instance_id"`
	BaseURL          string   `json:"base_url"`
	PathPrefix       string   `json:"path_prefix"`
	TokenEnv         string   `json:"token_env"`
	TenantID         string   `json:"tenant_id"`
	TenantIDEnv      string   `json:"tenant_id_env"`
	ResourceLimit    int      `json:"resource_limit"`
	StaleAfter       string   `json:"stale_after"`
	Enabled          *bool    `json:"enabled"`
	DeclaredIDs      []string `json:"declared_ids"`
	ObservedOnlyHint bool     `json:"observed_only_hint"`
}

// Settings is the validated per-claim configuration.
type Settings struct {
	Targets []TargetConfig
}

// ParseSettings validates the activation config for the claimed scope.
// getenv resolves credential environment variables the config names. Unknown
// keys fail closed so a misspelled limit cannot be silently ignored.
func ParseSettings(raw map[string]any, claimScopeID string, getenv func(string) string) (Settings, error) {
	var decoded struct {
		Targets []activationTargetJSON `json:"targets"`
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Settings{}, configErrorf("prometheus/mimir collector config is not JSON serializable")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return Settings{}, configErrorf("decode prometheus/mimir collector config: %s", decodeReason(err))
	}
	if len(decoded.Targets) == 0 {
		return Settings{}, configErrorf("prometheus/mimir collector requires at least one target")
	}
	targets := make([]TargetConfig, 0, len(decoded.Targets))
	seen := map[string]struct{}{}
	for i, item := range decoded.Targets {
		mapped, err := mapActivationTarget(item, getenv)
		if err != nil {
			return Settings{}, configErrorf("targets[%d]: %s", i, err.Error())
		}
		if _, dup := seen[mapped.ScopeID]; dup {
			return Settings{}, configErrorf("duplicate metric target scope_id %q", mapped.ScopeID)
		}
		seen[mapped.ScopeID] = struct{}{}
		targets = append(targets, mapped)
	}
	if strings.TrimSpace(claimScopeID) != "" {
		found := false
		for _, target := range targets {
			if target.ScopeID == strings.TrimSpace(claimScopeID) {
				found = true
				break
			}
		}
		if !found {
			return Settings{}, configErrorf("claim scope %q is not configured", strings.TrimSpace(claimScopeID))
		}
	}
	return Settings{Targets: targets}, nil
}

func mapActivationTarget(item activationTargetJSON, getenv func(string) string) (TargetConfig, error) {
	token, err := optionalEnvValue(item.TokenEnv, getenv, "token_env")
	if err != nil {
		return TargetConfig{}, err
	}
	tenantID, err := optionalEnvOverride(item.TenantID, item.TenantIDEnv, getenv, "tenant_id_env")
	if err != nil {
		return TargetConfig{}, err
	}
	staleAfter, err := parseOptionalDuration(item.StaleAfter, "stale_after")
	if err != nil {
		return TargetConfig{}, err
	}
	enabled := true
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	target := TargetConfig{
		Provider:         strings.TrimSpace(item.Provider),
		ScopeID:          strings.TrimSpace(item.ScopeID),
		InstanceID:       strings.TrimSpace(item.InstanceID),
		BaseURL:          strings.TrimRight(strings.TrimSpace(item.BaseURL), "/"),
		PathPrefix:       strings.TrimSpace(item.PathPrefix),
		Token:            token,
		TenantID:         tenantID,
		ResourceLimit:    item.ResourceLimit,
		StaleAfter:       staleAfter,
		Enabled:          enabled,
		DeclaredIDs:      stringSet(item.DeclaredIDs),
		ObservedOnlyHint: item.ObservedOnlyHint,
	}
	return validateTarget(target)
}

func validateTarget(target TargetConfig) (TargetConfig, error) {
	target.Provider = normalizedProvider(target.Provider)
	if target.Provider != ProviderPrometheus && target.Provider != ProviderMimir {
		return TargetConfig{}, fmt.Errorf("provider must be %q or %q", ProviderPrometheus, ProviderMimir)
	}
	target.ScopeID = strings.TrimSpace(target.ScopeID)
	target.InstanceID = strings.TrimSpace(target.InstanceID)
	target.BaseURL = strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	target.PathPrefix = "/" + strings.Trim(strings.TrimSpace(target.PathPrefix), "/")
	if target.PathPrefix == "/" {
		target.PathPrefix = ""
	}
	target.Token = strings.TrimSpace(target.Token)
	target.TenantID = strings.TrimSpace(target.TenantID)
	if target.ScopeID == "" {
		return TargetConfig{}, fmt.Errorf("scope_id is required")
	}
	if target.InstanceID == "" {
		return TargetConfig{}, fmt.Errorf("instance_id is required")
	}
	if target.BaseURL == "" {
		return TargetConfig{}, fmt.Errorf("base_url is required")
	}
	if target.ResourceLimit < 0 || target.ResourceLimit > maxResourceLimit {
		return TargetConfig{}, fmt.Errorf("resource_limit must be between 0 and %d", maxResourceLimit)
	}
	if target.ResourceLimit == 0 {
		target.ResourceLimit = defaultResourceLimit
	}
	return target, nil
}

func optionalEnvValue(envName string, getenv func(string) string, field string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return "", nil
	}
	if getenv == nil {
		return "", fmt.Errorf("%s %s did not resolve a value", field, envName)
	}
	value := strings.TrimSpace(getenv(envName))
	if value == "" {
		return "", fmt.Errorf("%s %s did not resolve a value", field, envName)
	}
	return value, nil
}

func optionalEnvOverride(raw string, envName string, getenv func(string) string, field string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return strings.TrimSpace(raw), nil
	}
	if getenv == nil {
		return "", fmt.Errorf("%s %s did not resolve a value", field, envName)
	}
	value := strings.TrimSpace(getenv(envName))
	if value == "" {
		return "", fmt.Errorf("%s %s did not resolve a value", field, envName)
	}
	return value, nil
}

func parseOptionalDuration(raw string, field string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s: %s", field, err.Error())
	}
	return value, nil
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

// ConfigError reports an invalid activation config. Its text names config
// keys only and never carries a resolved secret.
type ConfigError struct {
	msg   string
	class string
}

// Error returns the bounded configuration failure text.
func (e ConfigError) Error() string { return e.msg }

// FailureClass returns the bounded status class for this rejection.
func (e ConfigError) FailureClass() string {
	if e.class != "" {
		return e.class
	}
	return FailureInvalidConfig
}

func configErrorf(format string, args ...any) error {
	return ConfigError{msg: fmt.Sprintf(format, args...)}
}

func decodeReason(err error) string {
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &typeErr):
		return "field " + typeErr.Field + " has the wrong type"
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return "unknown field"
	default:
		return "invalid JSON"
	}
}
