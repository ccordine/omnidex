package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestOfflineResumeReceiptRequiresAllSchedulesAndFiveStaleWriteClasses(t *testing.T) {
	registration := testResumeRegistration(t)
	baseline := testResumeBaseline(t, 7)
	runs := make([]OfflineResumeRunReceipt, len(registration.Schedules))
	for index, schedule := range registration.Schedules {
		runs[index] = testResumeRun(t, schedule, baseline, index)
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	lastInference, firstEvaluator, completedAt, err := resumeAggregateChronology(runs)
	if err != nil {
		t.Fatal(err)
	}
	receipt := OfflineResumeReceipt{
		Schema: OfflineResumeReceiptSchemaV1, PreregistrationSHA256: registrationSHA,
		BaselineArtifactSHA256: strings.Repeat("8", 64), Runs: runs,
		LastInferenceExitedAt: lastInference, FirstEvaluatorStartedAt: firstEvaluator,
		CompletedAt: completedAt,
	}
	receipt.Gate = deriveOfflineResumeGate(receipt.Runs, baseline)
	receipt.GateEvidenceQualified = receipt.Gate.Passed
	receipt.PromotionEligible = false
	if err := receipt.Validate(registration, baseline); err != nil {
		t.Fatal(err)
	}
	if receipt.PromotionEligible || !receipt.Gate.Passed || receipt.Gate.StaleWriteClasses != 5 {
		t.Fatalf("Resume gate=%+v", receipt.Gate)
	}
	claimed := receipt
	claimed.PromotionEligible = true
	if err := claimed.Validate(registration, baseline); err == nil {
		t.Fatal("isolated Resume receipt claimed global promotion")
	}

	changed := receipt
	changed.Runs = append([]OfflineResumeRunReceipt{}, receipt.Runs...)
	changed.Runs[1].Semantics.ActionSequenceSHA256 = strings.Repeat("9", 64)
	changed.Gate = deriveOfflineResumeGate(changed.Runs, baseline)
	changed.GateEvidenceQualified = changed.Gate.Passed
	changed.PromotionEligible = false
	if err := changed.Validate(registration, baseline); err == nil {
		t.Fatal("Resume receipt accepted an interrupted action sequence that differs from baseline")
	}

	changed = receipt
	changed.Runs = append([]OfflineResumeRunReceipt{}, receipt.Runs...)
	live := *changed.Runs[4].LiveStaleProbe
	live.Probes = append([]LiveStalePortProof{}, live.Probes...)
	live.Probes[3].StateAfter.WorkingSetVersion++
	changed.Runs[4].LiveStaleProbe = &live
	changed.Gate = deriveOfflineResumeGate(changed.Runs, baseline)
	changed.GateEvidenceQualified = changed.Gate.Passed
	changed.PromotionEligible = false
	if err := changed.Validate(registration, baseline); err == nil {
		t.Fatal("Resume receipt accepted an omitted stale completion-write proof")
	}
	for name, mutate := range map[string]func(*OfflineResumeReceipt){
		"last inference": func(value *OfflineResumeReceipt) {
			value.LastInferenceExitedAt = value.LastInferenceExitedAt.Add(time.Second)
		},
		"first evaluator": func(value *OfflineResumeReceipt) {
			value.FirstEvaluatorStartedAt = value.FirstEvaluatorStartedAt.Add(time.Second)
		},
		"completion": func(value *OfflineResumeReceipt) {
			value.CompletedAt = value.CompletedAt.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			mutate(&changed)
			if err := changed.Validate(registration, baseline); err == nil {
				t.Fatal("Resume accepted caller-authored aggregate chronology")
			}
		})
	}
	verified := VerifiedOfflineResumeReceipt{receipt: receipt}
	copy := verified.Receipt()
	copy.Runs[1].Schedule.DecisionBoundaries[0]++
	copy.Runs[1].Interruptions[0].DecisionBoundary++
	copy.Runs[4].LiveStaleProbe.Probes[0].DatabaseSchema = "changed"
	if verified.receipt.Runs[1].Schedule.DecisionBoundaries[0] ==
		copy.Runs[1].Schedule.DecisionBoundaries[0] ||
		verified.receipt.Runs[1].Interruptions[0].DecisionBoundary ==
			copy.Runs[1].Interruptions[0].DecisionBoundary ||
		verified.receipt.Runs[4].LiveStaleProbe.Probes[0].DatabaseSchema == "changed" {
		t.Fatal("verified Resume receipt exposed mutable nested state")
	}
}

func testResumeRegistration(t *testing.T) OfflineResumePreregistration {
	t.Helper()
	request := validOfflineResumeRequest(t)
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	discovery, provider, host := offlinePrepareTestEvidence(t)
	prepared, err := prepareOfflineExperiment(
		request.baseExperiment(), discovery, provider, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := resumeConfigFromBase(request, prepared.promotion)
	registration, err := NewOfflineResumePreregistration(config.Plan, config.fixedAuthority())
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func testResumeBaseline(t *testing.T, decisions int) ResumeBaselineArtifact {
	t.Helper()
	checkpoints := make([]ResumeBaselineCheckpoint, decisions)
	for index := range checkpoints {
		checkpoints[index] = ResumeBaselineCheckpoint{
			DecisionBoundary: uint32(index),
			PreCall: testSemanticPreCallCheckpoint(
				1, "baseline-worker", cognition.ContextProjectionID(fmt.Sprintf("baseline-%d", index)),
				"baseline-snapshot",
			),
		}
	}
	artifact := ResumeBaselineArtifact{
		Schema:                   ResumeBaselineArtifactSchemaV1,
		PublicRunAuthoritySHA256: strings.Repeat("1", 64),
		EpisodeSealSHA256:        strings.Repeat("2", 64),
		Semantics: ResumeEpisodeSemantics{
			Schema:                   ResumeEpisodeSemanticsSchemaV2,
			ProjectionSequenceSHA256: strings.Repeat("3", 64),
			LogicalProjectionSHA256:  strings.Repeat("3", 64),
			ActionSequenceSHA256:     strings.Repeat("4", 64),
			FinalRevision: cognition.WorldRevision{
				EpisodeID: "episode-takeover", Number: 7, SHA256: strings.Repeat("5", 64),
			},
			Outcome:    Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "complete"},
			ModelCalls: decisions, ModelDecisions: decisions, EnvironmentActions: decisions,
			ProjectionCount: decisions, LogicalProjectionCount: decisions,
		},
		Checkpoints: checkpoints,
	}
	if err := artifact.Validate(); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func testResumeRun(
	t *testing.T,
	schedule OfflineResumeSchedule,
	baseline ResumeBaselineArtifact,
	index int,
) OfflineResumeRunReceipt {
	t.Helper()
	boundaries := schedule.DecisionBoundaries
	if schedule.Kind == ResumeEveryDecision {
		boundaries = []uint32{1, 2, 3, 4, 5, 6}
	} else if schedule.Kind == ResumeLiveInferenceExpiry {
		boundaries = []uint32{0}
	}
	interruptions := make([]OfflineResumeInterruptionReceipt, len(boundaries))
	for current, boundary := range boundaries {
		interruptions[current] = testResumeInterruption(t, boundary, int64(current+1), baseline)
	}
	now := time.Now().UTC().Add(time.Duration(index) * time.Second)
	run := OfflineResumeRunReceipt{
		Schedule: schedule, ScheduleEvidenceSHA256: strings.Repeat("5", 64),
		PromotionReceiptSHA256:   strings.Repeat("6", 64),
		PublicRunAuthoritySHA256: strings.Repeat("1", 64),
		EpisodeSealSHA256:        strings.Repeat("7", 64),
		EvaluationArtifactSHA256: strings.Repeat("8", 64),
		Semantics:                baseline.Semantics, Interruptions: interruptions,
		GoalSuccess: true, ValidTerminalState: true,
		CausalAdmissionComplete: true, CleanDeskQualified: true,
		Recovery:           RecoveryMetrics{Restarts: len(interruptions)},
		InferenceStartedAt: now, InferenceExitedAt: now.Add(time.Second),
		EvaluatorStartedAt:   now.Add(100 * time.Second),
		EvaluatorCompletedAt: now.Add(101 * time.Second),
	}
	if schedule.Kind == ResumeLiveInferenceExpiry {
		run.Semantics.ProjectionCount++
		run.Semantics.ProjectionSequenceSHA256 = strings.Repeat("d", 64)
		probe := testLiveStaleProbe(t, now)
		run.LiveStaleProbe = &probe
		if probe.CompletedAt.After(run.EvaluatorCompletedAt) {
			run.EvaluatorCompletedAt = probe.CompletedAt
		}
		run.LiveStaleProbeSHA256 = strings.Repeat("9", 64)
		run.Recovery.StaleAttemptRejections = 5
	}
	return run
}
