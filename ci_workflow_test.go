// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	pinnedAction     = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+@[0-9a-f]{40}$`)
	secretUsePattern = regexp.MustCompile(`secrets\.([A-Za-z0-9_]+)`)
)

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	// If holds the step's `if:` condition; ContinueOnError holds
	// `continue-on-error:`. Both default to the zero value when absent.
	// The govulncheck/gosec reachability tests read them; other tests
	// ignore them.
	If              string `yaml:"if"`
	ContinueOnError bool   `yaml:"continue-on-error"`
}

// needsList accepts the scalar (`needs: changes`) and sequence
// (`needs: [a, b]`) forms GitHub allows.
type needsList []string

func (n *needsList) UnmarshalYAML(value *yaml.Node) error {
	var single string
	if err := value.Decode(&single); err == nil {
		*n = []string{single}
		return nil
	}
	var multi []string
	if err := value.Decode(&multi); err != nil {
		return err
	}
	*n = multi
	return nil
}

type workflowFile struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency map[string]any    `yaml:"concurrency"`
	Jobs        map[string]struct {
		Needs       needsList         `yaml:"needs"`
		Uses        string            `yaml:"uses"`
		With        map[string]any    `yaml:"with"`
		Permissions map[string]string `yaml:"permissions"`
		Timeout     int               `yaml:"timeout-minutes"`
		Env         map[string]string `yaml:"env"`
		Steps       []workflowStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestCIWorkflowPinsActionsByCommitSHA keeps a moved tag from changing what
// pull-request CI executes. CI holds only contents: read, but it still runs
// third-party action code on every pull request.
func TestCIWorkflowPinsActionsByCommitSHA(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatal("ci.yml defines no jobs")
	}
	for name, job := range workflow.Jobs {
		// Job-level uses (the lanes reusable call) carries the same pin
		// contract as step-level action refs: a full commit SHA, never a
		// mutable tag.
		if job.Uses != "" && !pinnedAction.MatchString(job.Uses) {
			t.Errorf("job %s uses %q, want a full commit SHA pin", name, job.Uses)
		}
		for _, step := range job.Steps {
			if step.Uses != "" && !pinnedAction.MatchString(step.Uses) {
				t.Errorf("job %s uses %q, want a full commit SHA pin", name, step.Uses)
			}
		}
	}
}

// callerJobs are the only jobs the thin caller may define: the classifier,
// the reusable lane bodies, and the required aggregator. The org lane names
// (lint, test, race, shell, boundary, govulncheck, gosec, actionlint) now
// live inside the pinned reusable at an exact SHA, so they cannot churn
// here; ci-complete keeps its name so the ruleset never changes.
var callerJobs = []string{"changes", "lanes", "ci-complete"}

// effectiveLanes pins every run gate to the exact expression the retired
// inline lanes used. A proof test must fail when the behavior it claims
// breaks: flipping any gate to false (or widening it) turns the suite red,
// so lane selection cannot go dark unnoticed. run-image builds the shipped
// Dockerfile on every non-PR event and on PRs touching image inputs, Go, or
// workflows (issue #11; the image-only skip matrix is in ci_image_lane_test.go).

// wantUses is the single source of truth for the pinned reusable: the v2
// merge commit whose body was verified out-of-band (fail-loud govulncheck,
// schedule-gated gosec failure, inner timeouts). Tests that reason about
// reusable behavior derive from this pin so a pin move fails them until
// the new body is re-verified.
const wantUses = "eshu-hq/.github/.github/workflows/go-collector-ci.yml@4d01ab90baf5dd77c21cc1ee6bdf701332613449"

var effectiveLanes = map[string]string{
	"run-lint":               "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.misc == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-test":               "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.fixtures == 'true' || needs.changes.outputs.misc == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-race":               "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.misc == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-shell":              "${{ github.event_name != 'pull_request' || needs.changes.outputs.shell == 'true' }}",
	"run-image":              "${{ github.event_name != 'pull_request' || needs.changes.outputs.image == 'true' || needs.changes.outputs.go == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-boundary":           "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.misc == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-govulncheck":        "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.deps == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-gosec":              "${{ github.event_name != 'pull_request' || needs.changes.outputs.go == 'true' || needs.changes.outputs.workflows == 'true' }}",
	"run-actionlint":         "${{ github.event_name != 'pull_request' || needs.changes.outputs.workflows == 'true' }}",
	"boundary-command":       "go test -count=1 -run TestImportBoundaryRejectsCoreAndCrossCollectorImports .",
	"gosec-fail-on-findings": "${{ github.event_name == 'schedule' }}",
}

// TestCIWorkflowKeepsFrozenJobNames keeps the org lane contract across the
// reusable move (#15): the caller holds exactly its three jobs, the lanes
// job calls the pinned v2 reusable, and every effective lane stays gated.
func TestCIWorkflowKeepsFrozenJobNames(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	if len(workflow.Jobs) != len(callerJobs) {
		names := make([]string, 0, len(workflow.Jobs))
		for name := range workflow.Jobs {
			names = append(names, name)
		}
		t.Errorf("ci.yml jobs = %v, want exactly %v: lane bodies live in the reusable", names, callerJobs)
	}
	for _, name := range callerJobs {
		if _, ok := workflow.Jobs[name]; !ok {
			t.Errorf("ci.yml has no %q job, want the caller jobs %v", name, callerJobs)
		}
	}
	if _, ok := workflow.Jobs["vet"]; ok {
		t.Errorf("ci.yml still has a %q job, want it folded into the lint lane", "vet")
	}
	lanes, ok := workflow.Jobs["lanes"]
	if !ok {
		t.Fatal("ci.yml has no lanes job")
	}
	if lanes.Uses != wantUses {
		t.Errorf("lanes uses %q, want the pinned v2 reusable %q", lanes.Uses, wantUses)
	}
	for input, want := range effectiveLanes {
		val, ok := lanes.With[input]
		if !ok || fmt.Sprint(val) != want {
			t.Errorf("lanes input %q = %v, want the exact retired gate %q", input, val, want)
		}
	}
	complete, ok := workflow.Jobs["ci-complete"]
	if !ok {
		t.Fatal("ci.yml has no ci-complete job")
	}
	needed := make(map[string]bool, len(complete.Needs))
	for _, name := range complete.Needs {
		needed[name] = true
	}
	for _, name := range []string{"changes", "lanes"} {
		if !needed[name] {
			t.Errorf("ci-complete needs %v, want it to aggregate changes and lanes", complete.Needs)
		}
	}
}

// TestCIWorkflowSetsJobTimeouts keeps a hung runner from holding a PR open
// forever: every job declares a timeout, except the lanes reusable call —
// the workflow schema forbids timeout-minutes there, and each inner lane
// carries its own timeout, which bounds the call. Inner timeouts were
// verified out-of-band, not by this suite: direct fetch of the pinned
// reusable (go-collector-ci.yml at 4d01ab90, 2026-10-29; the pin
// rev-parses to the v2 tag) shows timeout-minutes on lint:141, test:174,
// race:190, shell:217, image:229, boundary:246, govulncheck:259,
// gosec:274, actionlint:311, ci-complete:329. If the pin moves, re-verify
// before trusting this comment.
func TestCIWorkflowSetsJobTimeouts(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	for name, job := range workflow.Jobs {
		if name == "lanes" {
			if job.Timeout != 0 {
				t.Errorf("job lanes sets timeout-minutes: the schema forbids it on reusable calls")
			}
			continue
		}
		if job.Timeout <= 0 {
			t.Errorf("job %s sets no timeout-minutes", name)
		}
	}
}

// TestCIWorkflowCancelsSupersededRuns keeps stale pushes from consuming
// runners: one concurrency group per PR (or ref) with cancellation.
func TestCIWorkflowCancelsSupersededRuns(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	if group := fmt.Sprint(workflow.Concurrency["group"]); !strings.Contains(group, "github.event.pull_request.number") {
		t.Errorf("concurrency group = %q, want one group per PR", group)
	}
	if cancel := fmt.Sprint(workflow.Concurrency["cancel-in-progress"]); cancel != "true" {
		t.Errorf("cancel-in-progress = %q, want true", cancel)
	}
}

// TestCIWorkflowPRLeanesNeedNoConfiguredSecrets keeps fork PRs working: the
// only secret any lane may touch is the automatic GITHUB_TOKEN (used solely
// by the schedule-only issue step), and the workflow holds only
// contents: read.
func TestCIWorkflowPRLeanesNeedNoConfiguredSecrets(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	if got := workflow.Permissions["contents"]; got != "read" {
		t.Errorf("top-level contents permission = %q, want read", got)
	}
	for name, job := range workflow.Jobs {
		var texts []string
		for key, value := range job.Env {
			texts = append(texts, key+"="+value)
		}
		// Job-level reusable inputs (the lanes gates) carry the same
		// no-secret contract as step bodies.
		for key, value := range job.With {
			texts = append(texts, key+"="+fmt.Sprint(value))
		}
		for _, step := range job.Steps {
			texts = append(texts, step.Run)
			for key, value := range step.Env {
				texts = append(texts, key+"="+value)
			}
			for key, value := range step.With {
				texts = append(texts, key+"="+fmt.Sprint(value))
			}
		}
		for _, text := range texts {
			for _, match := range secretUsePattern.FindAllStringSubmatch(text, -1) {
				if match[1] != "GITHUB_TOKEN" {
					t.Errorf("job %s uses configured secret %q, want no secrets but GITHUB_TOKEN", name, match[1])
				}
			}
		}
	}
}

// TestChangesDefinesFixtureAndMiscClasses keeps the skip matrix honest: the
// changes job must define the fixtures class (fixture-only PRs run test
// only) and the misc catch-all (unclassified files run the full Go lanes),
// with the some-with-excludes quantifier the misc negations need.
func TestChangesDefinesFixtureAndMiscClasses(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"fixtures:", "misc:", "predicate-quantifier: 'some-with-excludes'"} {
		if !strings.Contains(text, want) {
			t.Errorf("ci.yml changes job lacks %q", want)
		}
	}
	// config.example.yaml must NOT be a fixtures entry: no test reads it
	// (rg for config.example under Go files finds nothing), so it stays in
	// misc and runs the full Go lanes, fail closed.
	fixturesIdx := strings.Index(text, "fixtures:")
	miscIdx := strings.Index(text, "misc:")
	if fixturesIdx < 0 || miscIdx < 0 || fixturesIdx > miscIdx {
		t.Fatal("ci.yml has no fixtures/misc classes in order")
	}
	fixtures := text[fixturesIdx:miscIdx]
	if strings.Contains(fixtures, "config.example.yaml") {
		t.Error("fixtures class names config.example.yaml, but no test reads it: leave it in misc")
	}
	// Every fixtures entry must be negated in misc, or a fixture-only PR
	// also trips misc and pays for lint and race.
	misc := text[miscIdx:]
	for _, entry := range []string{"manifest.yaml", "grant.example.json", "**/*.json", "**/*.sh", "testdata/**"} {
		if !strings.Contains(misc, "'!"+entry+"'") {
			t.Errorf("misc class does not negate %q: a narrow-lane PR would also run the full lanes", entry)
		}
	}
}

// aggregatorProblems returns every way run breaks the ci-complete contract:
// it must compare each job's result (whole-object comparison never matches,
// a bare needs.*.result array breaks green runs), fail failed and cancelled
// lanes, and say skips are allowed.
func aggregatorProblems(run string) []string {
	var problems []string
	if !strings.Contains(run, `v["result"]`) && !strings.Contains(run, `.get("result")`) {
		problems = append(problems, `aggregator must compare v["result"] per job, not whole job objects (false-green)`)
	}
	if strings.Contains(run, "join(needs.*.result") {
		problems = append(problems, "aggregator must not expand needs.*.result (bare array breaks green runs)")
	}
	for _, want := range []string{`"failure"`, `"cancelled"`, "skip"} {
		if !strings.Contains(run, want) {
			problems = append(problems, "aggregator never mentions "+want)
		}
	}
	return problems
}

// stripCommentLines drops full-line comments (optional leading whitespace
// followed by #) from a joined run text. Explanatory comments in the live
// ci.yml text must not satisfy the executable-shape substring checks in
// aggregatorProblems: only executable lines pin the aggregator.
func stripCommentLines(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestCICompleteAggregatesPerJobResults pins the aggregator shape against
// its two historical false modes: the pre-gates shell loop over
// join(needs.*.result) and a whole-object comparison. The live ci.yml text
// must pass; both mutants must fail.
func TestCICompleteAggregatesPerJobResults(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	complete, ok := workflow.Jobs["ci-complete"]
	if !ok {
		t.Fatal("ci.yml has no ci-complete job")
	}
	var runs []string
	for _, step := range complete.Steps {
		runs = append(runs, step.Run)
	}
	if problems := aggregatorProblems(stripCommentLines(strings.Join(runs, "\n"))); len(problems) > 0 {
		t.Errorf("ci-complete aggregator breaks its contract:\n%s", strings.Join(problems, "\n"))
	}

	for name, run := range map[string]string{
		// The pre-gates aggregator: a bare needs.*.result array joined
		// into shell words, blind to skips and cancelled lanes.
		"bare needs array": "RESULTS: ${{ join(needs.*.result, ' ') }}\nfor result in $RESULTS; do if [ \"$result\" != \"success\" ]; then exit 1; fi; done",
		// Whole job objects never equal "failure": every red run passed.
		"whole objects": `bad = {k: v for k, v in needs.items() if v == "failure"}`,
	} {
		if len(aggregatorProblems(run)) == 0 {
			t.Errorf("%s: the gate accepted a violating aggregator", name)
		}
	}
}

// coverageFloorPattern matches the Makefile floor literal
// (`COVERAGE_FLOOR ?= 81.9`). The CI side now lives in the lanes job's
// coverage-floor reusable input, not in a race-job env block.
var coverageFloorPattern = regexp.MustCompile(`COVERAGE_FLOOR\s*\??=\s*["']?([0-9.]+)["']?`)

// TestCoverageFloorParity keeps `make ci` honest: the Makefile floor and
// the lanes coverage-floor input must name the same number, so the next
// floor ratchet that touches one file and not the other fails this gate
// instead of letting local and CI disagree silently.
func TestCoverageFloorParity(t *testing.T) {
	t.Parallel()
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	workflow := readCIWorkflow(t)
	lanes, ok := workflow.Jobs["lanes"]
	if !ok {
		t.Fatal("ci.yml has no lanes job")
	}
	makeFloor := coverageFloorPattern.FindStringSubmatch(string(makefile))
	ciFloor, ok := lanes.With["coverage-floor"]
	if len(makeFloor) < 2 || !ok {
		t.Fatalf("COVERAGE_FLOOR literal missing (Makefile=%q lanes=%v)", makeFloor, lanes.With["coverage-floor"])
	}
	if makeFloor[1] != fmt.Sprint(ciFloor) {
		t.Errorf("COVERAGE_FLOOR drift: Makefile=%s lanes=%v, want one shared value", makeFloor[1], ciFloor)
	}
}

// scanFailureProblems returns every way a scan lane breaks its scheduled
// failure contract. The scan bodies now live in the pinned reusable, so the
// caller owns two things: the gate expression must run the scan on schedule
// (and on the paths that can newly expose findings), and nothing in the
// caller may quiet it (no always-false gate). Per-scan issue filing has no
// reusable hook yet and rides an upstream follow-up; what must never break
// is the loud failure itself.
func scanFailureProblems(gate string, scheduleFails bool) []string {
	var problems []string
	if !strings.Contains(gate, "github.event_name != 'pull_request'") {
		problems = append(problems, "scan gate never runs on schedule/push (missing the non-PR short-circuit)")
	}
	if !scheduleFails {
		problems = append(problems, "scheduled findings stay green: triage has nothing loud to ride")
	}
	return problems
}

// TestGovulncheckFailsLoudOnSchedule keeps the govulncheck promise: dep, Go,
// and workflow changes scan (a Go-only change can newly reach a vulnerable
// symbol), scheduled runs always scan, and the reusable scan fails loud by
// construction (go-collector-ci.yml at 4d01ab90 runs
// `govulncheck -scan package ./...` with no `|| true`, lines 256-268;
// no quieting input exists).
func TestGovulncheckFailsLoudOnSchedule(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	lanes, ok := workflow.Jobs["lanes"]
	if !ok {
		t.Fatal("ci.yml has no lanes job")
	}
	gate := fmt.Sprint(lanes.With["run-govulncheck"])
	// scheduleFails is derived, not assumed: it holds only while the
	// caller points at the out-of-band-verified pin (fail-loud body read
	// on 2026-10-29; the pin rev-parses to the v2 tag). A pin move flips
	// this to false and fails the test until the new body is re-verified.
	scheduleFails := lanes.Uses == wantUses
	if problems := scanFailureProblems(gate, scheduleFails); len(problems) > 0 {
		t.Errorf("govulncheck schedule gate is broken:\n%s", strings.Join(problems, "\n"))
	}
	for _, want := range []string{"needs.changes.outputs.deps", "needs.changes.outputs.go"} {
		if !strings.Contains(gate, want) {
			t.Errorf("govulncheck gate %q lacks %s: findings from those paths would go unscanned", gate, want)
		}
	}

	if len(scanFailureProblems("needs.changes.outputs.deps == 'true'", true)) == 0 {
		t.Error("PR-only gate: the gate accepted a scan that never runs on schedule")
	}
}

// TestGosecFailsLoudOnSchedule keeps the gosec promise: Go and workflow
// changes scan, scheduled runs with findings fail red via the
// gosec-fail-on-findings schedule gate, and PR runs stay advisory green.
// Verified against the pinned reusable (go-collector-ci.yml at 4d01ab90):
// the lane fails only under `if: inputs.gosec-fail-on-findings`
// (lines 295-299), and the caller passes the schedule-only expression,
// so PR runs cannot fail while scheduled findings fail red.
func TestGosecFailsLoudOnSchedule(t *testing.T) {
	t.Parallel()

	workflow := readCIWorkflow(t)
	lanes, ok := workflow.Jobs["lanes"]
	if !ok {
		t.Fatal("ci.yml has no lanes job")
	}
	gate := fmt.Sprint(lanes.With["run-gosec"])
	failGate := fmt.Sprint(lanes.With["gosec-fail-on-findings"])
	scheduleFails := strings.Contains(failGate, "schedule") && !strings.Contains(failGate, "false")
	if problems := scanFailureProblems(gate, scheduleFails); len(problems) > 0 {
		t.Errorf("gosec schedule gate is broken:\n%s", strings.Join(problems, "\n"))
	}

	if len(scanFailureProblems(gate, false)) == 0 {
		t.Error("advisory-only schedule: the gate accepted findings that stay green on the sweep")
	}
}

func readCIWorkflow(t *testing.T) workflowFile {
	t.Helper()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatal("ci.yml defines no jobs")
	}
	return workflow
}
