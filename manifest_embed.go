// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import _ "embed"

// ManifestYAML is the component manifest shipped with this binary.
//
//go:embed manifest.yaml
var ManifestYAML []byte
