// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// manifestDoc mirrors the component manifest shape for tests only. The
// binary embeds manifest.yaml as bytes; parsing stays in tests so the
// runtime never takes a yaml dependency.
type manifestDoc struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		ID      string `yaml:"id"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		CompatibleCore string   `yaml:"compatibleCore"`
		CollectorKinds []string `yaml:"collectorKinds"`
		Runtime        struct {
			SDKProtocol string `yaml:"sdkProtocol"`
		} `yaml:"runtime"`
		Artifacts []struct {
			Image string `yaml:"image"`
		} `yaml:"artifacts"`
		EmittedFacts []struct {
			Kind             string   `yaml:"kind"`
			SchemaVersions   []string `yaml:"schemaVersions"`
			SourceConfidence []string `yaml:"sourceConfidence"`
		} `yaml:"emittedFacts"`
		Telemetry struct {
			MetricsPrefix string `yaml:"metricsPrefix"`
		} `yaml:"telemetry"`
	} `yaml:"spec"`
}

func loadManifest(t *testing.T) manifestDoc {
	t.Helper()
	raw, err := os.ReadFile("manifest.yaml")
	if err != nil {
		t.Fatalf("read manifest.yaml: %v", err)
	}
	var doc manifestDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode manifest.yaml: %v", err)
	}
	return doc
}

// TestManifestDeclaresTheFourObservabilityKinds keeps the manifest, the
// contract the handler emits against, and the pinned SDK in step: every kind
// is declared once at schema 1.0.0 with reported confidence.
func TestManifestDeclaresTheFourObservabilityKinds(t *testing.T) {
	t.Parallel()

	manifest := loadManifest(t)
	if manifest.Metadata.ID != ComponentID {
		t.Fatalf("manifest id = %q, want %q", manifest.Metadata.ID, ComponentID)
	}
	if manifest.Metadata.Version != GrantVersion {
		t.Fatalf("manifest version = %q, want %q", manifest.Metadata.Version, GrantVersion)
	}
	if len(manifest.Spec.CollectorKinds) != 1 || manifest.Spec.CollectorKinds[0] != CollectorKind {
		t.Fatalf("collectorKinds = %v, want [%q]", manifest.Spec.CollectorKinds, CollectorKind)
	}
	want := FactKinds()
	var got []string
	for _, family := range manifest.Spec.EmittedFacts {
		got = append(got, family.Kind)
		if strings.Join(family.SchemaVersions, ",") != SchemaVersion {
			t.Errorf("%s: schemaVersions=%v, want [%s]", family.Kind, family.SchemaVersions, SchemaVersion)
		}
		if strings.Join(family.SourceConfidence, ",") != "reported" {
			t.Errorf("%s: sourceConfidence=%v, want [reported]", family.Kind, family.SourceConfidence)
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest kinds = %v, want %v", got, want)
	}
	if manifest.Spec.Telemetry.MetricsPrefix != MetricsPrefix {
		t.Fatalf("metricsPrefix = %q, want %q", manifest.Spec.Telemetry.MetricsPrefix, MetricsPrefix)
	}
	for _, artifact := range manifest.Spec.Artifacts {
		if !strings.Contains(artifact.Image, "@sha256:") {
			t.Fatalf("artifact image %q is not digest-pinned", artifact.Image)
		}
		if strings.Contains(artifact.Image, ":latest") {
			t.Fatalf("artifact image %q uses a mutable tag", artifact.Image)
		}
		if !isPlaceholderImage(artifact.Image) {
			t.Fatalf("artifact image %q is not the placeholder digest: do not commit a real digest "+
				"(the manifest is embedded in the image, so it would belong to an older build); "+
				"the release workflow stamps the deployed digest when it lands", artifact.Image)
		}
	}
}

// placeholderDigest is the artifact digest the repository manifest carries
// until a release workflow lands. manifest.yaml is go:embed'ed into the
// binary, so a digest written into it would belong to an older build than
// the image that embeds it. This is the explicit disposition of the
// manifest.yaml:18 zero-digest: it stays until release stamping exists,
// and the artifact loop above fails loudly if anyone commits a real digest
// thinking it deploys.
const placeholderDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// isPlaceholderImage reports whether image names the placeholder digest.
func isPlaceholderImage(image string) bool {
	return strings.HasSuffix(image, "@"+placeholderDigest)
}

// TestPlaceholderImageGate pins the gate on both sides: the placeholder passes
// and a real-looking digest fails.
func TestPlaceholderImageGate(t *testing.T) {
	t.Parallel()

	const repo = "ghcr.io/eshu-hq/eshu-collector-prometheus-mimir@"
	if !isPlaceholderImage(repo + placeholderDigest) {
		t.Fatal("the placeholder digest must pass the gate")
	}
	if isPlaceholderImage(repo + "sha256:" + strings.Repeat("a", 64)) {
		t.Fatal("a real-looking digest must fail the gate")
	}
}

// producerGrant is the shape of a core producer grant request. Core issues the
// grant; this repository stores only the request an operator submits.
type producerGrant struct {
	ProducerID     string    `json:"producer_id"`
	Version        string    `json:"version"`
	Kind           string    `json:"kind"`
	SchemaVersions []string  `json:"schema_versions"`
	Scope          string    `json:"scope"`
	ExpiresAt      time.Time `json:"expires_at"`
	Revoked        bool      `json:"revoked"`
}

// TestGrantExampleCoversExactlyTheManifestKinds proves the grant request an
// operator submits matches the manifest: one grant per declared core kind,
// bound to this producer and version, scoped to the collector kind, and
// covering every declared schema version.
func TestGrantExampleCoversExactlyTheManifestKinds(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("grant.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var grants []producerGrant
	if err := json.Unmarshal(raw, &grants); err != nil {
		t.Fatalf("grant.example.json: %v", err)
	}
	manifest := loadManifest(t)
	declared := map[string][]string{}
	for _, family := range manifest.Spec.EmittedFacts {
		declared[family.Kind] = family.SchemaVersions
	}
	got := map[string][]string{}
	for _, grant := range grants {
		if grant.ProducerID != ComponentID || grant.Version != manifest.Metadata.Version {
			t.Fatalf("grant %+v is not bound to %s@%s", grant, ComponentID, manifest.Metadata.Version)
		}
		if grant.Scope != CollectorKind || grant.Revoked || grant.ExpiresAt.IsZero() {
			t.Fatalf("grant %+v must be live and scoped to the collector kind %q", grant, CollectorKind)
		}
		if _, dup := got[grant.Kind]; dup {
			t.Fatalf("grant kind %s appears twice", grant.Kind)
		}
		got[grant.Kind] = grant.SchemaVersions
	}
	if !reflect.DeepEqual(got, declared) {
		t.Fatalf("grants cover %v, manifest declares %v", got, declared)
	}
}
