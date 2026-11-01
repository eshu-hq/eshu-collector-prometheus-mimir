package collector

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestImageLaneBuildsOnImageInputs pins the PR-time image lane (issue #11):
// the changes job must classify Dockerfile and .dockerignore as the image
// class, the misc catch-all must negate both so an image-only PR runs the
// image lane and nothing else, and the lanes call must build the image with
// a digest-pin proof instead of leaving run-image false.
func TestImageLaneBuildsOnImageInputs(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"image: ${{ steps.filter.outputs.image }}",
		"- 'Dockerfile'",
		"- '.dockerignore'",
		"- '!Dockerfile'",
		"- '!.dockerignore'",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ci.yml lacks %q: an image-only PR would fall through to misc", want)
		}
	}

	lanes := readCIWorkflow(t).Jobs["lanes"]
	if got := fmt.Sprint(lanes.With["image-tag"]); got != "eshu-collector-prometheus-mimir:ci" {
		t.Errorf("lanes image-tag = %q, want eshu-collector-prometheus-mimir:ci", got)
	}
	if got := fmt.Sprint(lanes.With["image-context"]); got != "." {
		t.Errorf("lanes image-context = %q, want .", got)
	}
	imageCmd := fmt.Sprint(lanes.With["image-command"])
	for _, marker := range []string{"digest-pinned", "65532:65532", "Dockerfile bases"} {
		if !strings.Contains(imageCmd, marker) {
			t.Errorf("lanes image-command lacks %q: the image proof is gone", marker)
		}
	}
}
