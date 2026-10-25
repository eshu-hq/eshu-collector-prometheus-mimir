// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"errors"

	"github.com/eshu-hq/eshu-sdk/collector/transport"
)

// classifiedProviderFailure maps a collection error to its bounded failure
// class. A bad_data API status is terminal; everything else follows the
// transport policy with auth-denied at claim scope staying terminal.
func classifiedProviderFailure(err error) transport.ProviderFailure {
	var apiErr ProviderAPIError
	if errors.As(err, &apiErr) {
		class := transport.FailureRetryable
		terminal := false
		if apiErr.ErrorType == "bad_data" {
			class = transport.FailureTerminal
			terminal = true
		}
		return transport.NewProviderFailure("prometheus_mimir", class, terminal, err).WithOperation(apiErr.FailureOperation())
	}
	return transport.ClassifyProviderFailure("prometheus_mimir", err, transport.StatusPolicy{
		AuthDeniedClass: transport.FailureAuthDenied,
		NotFoundClass:   transport.FailureTerminal,
	}, transport.FailureRetryable)
}
