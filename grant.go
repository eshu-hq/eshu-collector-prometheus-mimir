// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"fmt"
	"strings"
)

// GrantAuthorizer decides whether an emission is authorized for one fact
// kind, schema version, and scope. The host owns the live grant registry;
// this interface is the seam the extracted producer checks before emitting.
// See PRODUCER_GRANT_DRAFT semantics in eshu-hq/eshu#8864: admit only when
// producer identity, version, kind, schema version, and scope all match a
// live, unexpired, unrevoked stored grant; anything else fails closed.
type GrantAuthorizer interface {
	// Authorize reports whether the producer may emit one fact kind at one
	// schema version under one scope. A nil error authorizes; any error
	// denies and fails the claim closed.
	Authorize(kind string, schemaVersion string, scope string) error
}

// FailClosedAuthorizer denies every emission. It is the safe default until
// the host wires the live grant registry: with no grants configured, the
// producer emits nothing rather than emitting under unknown authorization.
// The parity suite runs authorizer-free (like the in-tree first-party path);
// any Handler with this authorizer attached must fail terminal with no facts.
type FailClosedAuthorizer struct{}

// Authorize always denies with the reason the operator must fix.
func (FailClosedAuthorizer) Authorize(kind string, schemaVersion string, scope string) error {
	return fmt.Errorf(
		"producer grant not configured for kind %q schema %q scope %q (producer %s version %s)",
		strings.TrimSpace(kind),
		strings.TrimSpace(schemaVersion),
		strings.TrimSpace(scope),
		ComponentID,
		GrantVersion,
	)
}

// checkGrants fails closed when an authorizer is attached: every emitted
// kind must authorize at the declared schema version under the claim scope.
// A nil authorizer preserves the in-tree first-party behavior (emission
// without a grant lookup) for parity and credential-free replay.
func checkGrants(authorizer GrantAuthorizer, scope string, kinds []string) error {
	if authorizer == nil {
		return nil
	}
	for _, kind := range kinds {
		if err := authorizer.Authorize(kind, SchemaVersion, scope); err != nil {
			return err
		}
	}
	return nil
}
