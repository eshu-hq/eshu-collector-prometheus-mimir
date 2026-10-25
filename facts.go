// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/factschema"
	observabilityv1 "github.com/eshu-hq/eshu-sdk/factschema/observability/v1"
	"github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// newSourceInstanceFact converts one live metric target snapshot into a
// source-instance SDK fact.
func newSourceInstanceFact(ctx EnvelopeContext, source SourceInstance, stats CollectionStats) (sdk.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return sdk.Fact{}, err
	}
	payload := basePayload(ctx, OutcomeObserved, FreshnessCurrent)
	payload["resource_count"] = stats.Targets
	payload["rule_count"] = stats.Rules
	payload["warning_count"] = stats.Warnings
	payload["pages_fetched"] = stats.PagesFetched
	payload["partial"] = stats.Partial
	payload["truncated"] = stats.Truncated
	if source.TenantPresent || source.TenantRedacted {
		payload["tenant_scope_state"] = "configured"
		payload["tenant_redacted"] = true
		if source.TenantFingerprint != "" {
			payload["tenant_fingerprint"] = source.TenantFingerprint
		}
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilitySourceInstance(observabilityv1.SourceInstance{
			SourceInstanceID: ctx.SourceInstanceID,
			Provider:         stringPtr(sourceKind(ctx)),
			SourceKind:       stringPtr(sourceKind(ctx)),
			SourceClass:      stringPtr(SourceClassObserved),
			Outcome:          stringPtr(OutcomeObserved),
			FreshnessState:   stringPtr(FreshnessCurrent),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}
	stableKey := stableFactKey(FactKindSourceInstance, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"source_kind":        sourceKind(ctx),
	})
	return observabilityFact(ctx, FactKindSourceInstance, stableKey, payload, ctx.SourceInstanceID), nil
}

// newObservedTargetFact converts one bounded live target into an
// observed-target SDK fact.
func newObservedTargetFact(ctx EnvelopeContext, target Target) (sdk.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return sdk.Fact{}, err
	}
	recordID := strings.TrimSpace(target.ProviderObjectID)
	if recordID == "" {
		return sdk.Fact{}, fmt.Errorf("metric target identity must not be blank")
	}
	payload := basePayload(ctx, normalizedOutcome(target.Outcome), normalizedFreshness(target.FreshnessState))
	payload["resource_class"] = ResourceClassTarget
	payload["provider_object_uid"] = recordID
	if value := strings.TrimSpace(target.ScrapePool); value != "" {
		payload["scrape_pool"] = value
	}
	if value := strings.TrimSpace(target.Health); value != "" {
		payload["health"] = value
	}
	if value := strings.TrimSpace(target.DeclaredMatchState); value != "" {
		payload["declared_match_state"] = value
	} else {
		payload["declared_match_state"] = MatchStateNotCompared
	}
	setStringSlice(payload, "label_keys", target.LabelKeys)
	setStringSlice(payload, "discovered_label_keys", target.DiscoveredKeys)
	setURLRedaction(payload, "scrape_url", target.ScrapeURL, target.ScrapeURLRedacted)
	setTimePayload(payload, "last_scrape_at", target.LastScrapeAt)
	if target.LastErrorRedacted {
		payload["last_error_redacted"] = true
	}
	if target.ManuallyCreated {
		payload["manually_created"] = true
		payload["drift_candidate_reason"] = WarningManualProviderResource
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilityObservedTarget(observabilityv1.ObservedTarget{
			SourceInstanceID:     ctx.SourceInstanceID,
			ProviderObjectUID:    recordID,
			Provider:             stringPtr(sourceKind(ctx)),
			SourceKind:           stringPtr(sourceKind(ctx)),
			SourceClass:          stringPtr(SourceClassObserved),
			ResourceClass:        stringPtr(ResourceClassTarget),
			Outcome:              stringPtr(normalizedOutcome(target.Outcome)),
			FreshnessState:       stringPtr(normalizedFreshness(target.FreshnessState)),
			DriftCandidateReason: optionalStringPtr(payloadString(payload, "drift_candidate_reason")),
			DeclaredMatchState:   stringPtr(firstNonBlank(target.DeclaredMatchState, MatchStateNotCompared)),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}
	stableKey := stableFactKey(FactKindObservedTarget, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"target_uid":         recordID,
	})
	return observabilityFact(ctx, FactKindObservedTarget, stableKey, payload, recordID), nil
}

// newObservedRuleFact converts one bounded live rule into an observed-rule
// SDK fact.
func newObservedRuleFact(ctx EnvelopeContext, rule Rule) (sdk.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return sdk.Fact{}, err
	}
	recordID := firstNonBlank(rule.ProviderObjectID, rule.GroupName+":"+rule.RuleName)
	if recordID == "" || recordID == ":" {
		return sdk.Fact{}, fmt.Errorf("metric rule identity must not be blank")
	}
	payload := basePayload(ctx, normalizedOutcome(rule.Outcome), normalizedFreshness(rule.FreshnessState))
	payload["resource_class"] = ResourceClassRule
	payload["provider_object_uid"] = recordID
	if value := strings.TrimSpace(rule.GroupName); value != "" {
		payload["rule_group"] = value
	}
	if value := strings.TrimSpace(rule.RuleName); value != "" {
		payload["rule_name"] = value
	}
	if value := strings.TrimSpace(rule.RuleType); value != "" {
		payload["rule_type"] = value
	}
	if value := strings.TrimSpace(rule.Health); value != "" {
		payload["health"] = value
	}
	if value := strings.TrimSpace(rule.DeclaredMatchState); value != "" {
		payload["declared_match_state"] = value
	} else {
		payload["declared_match_state"] = MatchStateNotCompared
	}
	setStringSlice(payload, "label_keys", rule.LabelKeys)
	setStringSlice(payload, "annotation_keys", rule.AnnotationKeys)
	setTimePayload(payload, "last_evaluation_at", rule.LastEvaluationAt)
	if strings.TrimSpace(rule.Query) != "" || rule.QueryRedacted {
		payload["query_redacted"] = true
	}
	if rule.ManuallyCreated {
		payload["manually_created"] = true
		payload["drift_candidate_reason"] = WarningManualProviderResource
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilityObservedRule(observabilityv1.ObservedRule{
			SourceInstanceID:     ctx.SourceInstanceID,
			ProviderObjectUID:    stringPtr(recordID),
			RuleGroup:            optionalStringPtr(rule.GroupName),
			RuleName:             optionalStringPtr(rule.RuleName),
			Provider:             stringPtr(sourceKind(ctx)),
			SourceKind:           stringPtr(sourceKind(ctx)),
			SourceClass:          stringPtr(SourceClassObserved),
			ResourceClass:        stringPtr(ResourceClassRule),
			Outcome:              stringPtr(normalizedOutcome(rule.Outcome)),
			FreshnessState:       stringPtr(normalizedFreshness(rule.FreshnessState)),
			DriftCandidateReason: optionalStringPtr(payloadString(payload, "drift_candidate_reason")),
			DeclaredMatchState:   stringPtr(firstNonBlank(rule.DeclaredMatchState, MatchStateNotCompared)),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}
	stableKey := stableFactKey(FactKindObservedRule, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"rule_uid":           recordID,
	})
	return observabilityFact(ctx, FactKindObservedRule, stableKey, payload, recordID), nil
}

// newCoverageWarningFact converts a source-local warning into an SDK
// coverage-warning fact.
func newCoverageWarningFact(ctx EnvelopeContext, warning Warning) (sdk.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return sdk.Fact{}, err
	}
	reason := strings.TrimSpace(warning.Reason)
	if reason == "" {
		reason = WarningPartial
	}
	resourceClass := strings.TrimSpace(warning.ResourceClass)
	if resourceClass == "" {
		resourceClass = "unknown"
	}
	resourceID := strings.TrimSpace(warning.ResourceID)
	payload := basePayload(ctx, warningOutcome(reason), warningFreshness(reason))
	payload["warning_kind"] = reason
	payload["resource_class"] = resourceClass
	if resourceID != "" {
		payload["provider_object_uid"] = resourceID
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilityCoverageWarning(observabilityv1.CoverageWarning{
			SourceInstanceID:  ctx.SourceInstanceID,
			ProviderObjectUID: optionalStringPtr(resourceID),
			Provider:          stringPtr(sourceKind(ctx)),
			SourceKind:        stringPtr(sourceKind(ctx)),
			SourceClass:       stringPtr(SourceClassObserved),
			ResourceClass:     stringPtr(resourceClass),
			Outcome:           stringPtr(warningOutcome(reason)),
			FreshnessState:    stringPtr(warningFreshness(reason)),
			WarningKind:       stringPtr(reason),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}
	stableKey := stableFactKey(FactKindCoverageWarning, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"resource_class":     resourceClass,
		"resource_id":        resourceID,
		"reason":             reason,
	})
	return observabilityFact(ctx, FactKindCoverageWarning, stableKey, payload, firstNonBlank(resourceID, reason)), nil
}

func basePayload(ctx EnvelopeContext, outcome string, freshness string) map[string]any {
	payload := map[string]any{
		"collector_instance_id": ctx.CollectorInstanceID,
		"provider":              sourceKind(ctx),
		"source_class":          SourceClassObserved,
		"source_kind":           sourceKind(ctx),
		"source_instance_id":    ctx.SourceInstanceID,
		"scope_id":              ctx.ScopeID,
		"generation_id":         ctx.GenerationID,
		"observed_at":           normalizedObservedAt(ctx.ObservedAt).Format(time.RFC3339Nano),
		"freshness_state":       freshness,
		"redaction_version":     RedactionVersion,
		"outcome":               outcome,
	}
	payload["provenance"] = map[string]any{
		"provider":           sourceKind(ctx),
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
	}
	return payload
}

func mergeContractPayload(payload map[string]any, encode func() (map[string]any, error)) error {
	encoded, err := encode()
	if err != nil {
		return err
	}
	for key, value := range encoded {
		payload[key] = value
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func payloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func observabilityFact(
	ctx EnvelopeContext,
	kind string,
	stableKey string,
	payload map[string]any,
	recordID string,
) sdk.Fact {
	return sdk.Fact{
		Kind:             kind,
		SchemaVersion:    SchemaVersion,
		StableKey:        stableKey,
		SourceConfidence: sdk.SourceConfidenceReported,
		ObservedAt:       normalizedObservedAt(ctx.ObservedAt),
		SourceRef: sdk.SourceRef{
			SourceSystem: ctx.SourceSystem,
			ScopeID:      ctx.ScopeID,
			GenerationID: ctx.GenerationID,
			FactKey:      stableKey,
			URI:          sanitizeURL(ctx.SourceURI),
			RecordID:     strings.TrimSpace(recordID),
		},
		Payload:    payload,
		Redactions: redactionsFor(payload),
	}
}

// redactionsFor records every payload field the collector intentionally
// blanked while the source carried a value. The list is sorted for
// byte-stable output and every entry carries the redaction policy version.
func redactionsFor(payload map[string]any) []sdk.Redaction {
	fields := make([]string, 0, 4)
	for key, value := range payload {
		if value == true && strings.HasSuffix(key, "_redacted") {
			fields = append(fields, key)
			continue
		}
		if text, ok := value.(string); ok && text != "" && strings.HasSuffix(key, "_fingerprint") {
			fields = append(fields, key)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	sort.Strings(fields)
	redactions := make([]sdk.Redaction, 0, len(fields))
	for _, field := range fields {
		redactions = append(redactions, sdk.Redaction{Field: field, Reason: RedactionVersion})
	}
	return redactions
}

// sanitizeURL drops credentials, sensitive query keys, and fragments. A value
// that is not an absolute URL sanitizes to the empty string.
func sanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveQueryKey(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

func sensitiveQueryKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "access_token", "api_key", "apikey", "auth", "authorization", "jwt",
		"key", "password", "passwd", "secret", "sig", "signature", "token":
		return true
	default:
		return false
	}
}

func validateEnvelopeContext(ctx EnvelopeContext) error {
	if strings.TrimSpace(ctx.ScopeID) == "" {
		return fmt.Errorf("metric fact scope_id must not be blank")
	}
	if strings.TrimSpace(ctx.GenerationID) == "" {
		return fmt.Errorf("metric fact generation_id must not be blank")
	}
	if strings.TrimSpace(ctx.CollectorInstanceID) == "" {
		return fmt.Errorf("metric fact collector_instance_id must not be blank")
	}
	if strings.TrimSpace(ctx.SourceInstanceID) == "" {
		return fmt.Errorf("metric fact source_instance_id must not be blank")
	}
	return nil
}

func stableFactKey(kind string, generationID string, id map[string]any) string {
	return strings.Join([]string{kind, generationID, identity.StableID(kind, id)}, ":")
}

func normalizedObservedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func setTimePayload(payload map[string]any, key string, value time.Time) {
	if !value.IsZero() {
		payload[key] = value.UTC().Format(time.RFC3339Nano)
	}
}

func warningOutcome(reason string) string {
	switch reason {
	case WarningPermissionHidden:
		return OutcomePermissionHidden
	case WarningUnsupported:
		return OutcomeUnsupported
	case WarningManualProviderResource:
		return OutcomeObserved
	case WarningStale:
		return OutcomeStale
	default:
		return OutcomePartial
	}
}

func warningFreshness(reason string) string {
	switch reason {
	case WarningPermissionHidden:
		return FreshnessPermissionHidden
	case WarningStale:
		return FreshnessStale
	default:
		return FreshnessUnknown
	}
}

func normalizedOutcome(value string) string {
	switch strings.TrimSpace(value) {
	case OutcomePartial:
		return OutcomePartial
	case OutcomePermissionHidden:
		return OutcomePermissionHidden
	case OutcomeUnsupported:
		return OutcomeUnsupported
	case OutcomeRejected:
		return OutcomeRejected
	case OutcomeStale:
		return OutcomeStale
	default:
		return OutcomeObserved
	}
}

func normalizedFreshness(value string) string {
	switch strings.TrimSpace(value) {
	case FreshnessUnknown:
		return FreshnessUnknown
	case FreshnessStale:
		return FreshnessStale
	case FreshnessPermissionHidden:
		return FreshnessPermissionHidden
	default:
		return FreshnessCurrent
	}
}

func sourceKind(ctx EnvelopeContext) string {
	switch strings.TrimSpace(ctx.SourceKind) {
	case SourceKindMimir:
		return SourceKindMimir
	default:
		return SourceKindPrometheus
	}
}
