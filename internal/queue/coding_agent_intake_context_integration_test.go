package queue

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentIntakeRetainsOriginalArtifactBindingsAcrossChangesAndResume(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	root := t.TempDir()
	job, err := repository.EnqueueCodingJobMode(ctx, "Handle NaN and infinity values.", root, codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "retained-intake-planning-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner: %v %v", planner, err)
	}
	retained := &codingagent.IntakeContext{RedactedRequest: "Handle ARTIFACT_1 and ARTIFACT_2 values.", ArtifactBindings: []codingagent.ArtifactBinding{{Token: "ARTIFACT_1", Value: "NaN"}, {Token: "ARTIFACT_2", Value: "infinity"}}}
	leaf := codingPlanExecutableLeaf(t, "ARTIFACT_1 and ARTIFACT_2 are handled.", model.CodingPlanDecisionPending, 1)
	command := StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}, IntakeContext: retained}
	if _, err := repository.StoreCodingPlanReview(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StoreCodingPlanReview(ctx, command); err != nil {
		t.Fatalf("exact retained intake replay: %v", err)
	}
	changed := *retained
	changed.ArtifactBindings = []codingagent.ArtifactBinding{{Token: "ARTIFACT_1", Value: "A relabeled path"}}
	altered := command
	altered.IntakeContext = &changed
	if _, err := repository.StoreCodingPlanReview(ctx, altered); err == nil {
		t.Fatal("completed intake replay changed artifact provenance")
	}
	executor, err := repository.ClaimNextStep(ctx, "retained-intake-execution-fixture")
	if err != nil || executor == nil {
		t.Fatalf("executor: %v %v", executor, err)
	}
	if err := os.WriteFile(filepath.Join(root, "newly-created-path.txt"), []byte("changed workspace discovery"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.CodingAgentIntakeContext(ctx, executor.Authority)
	if err != nil || !reflect.DeepEqual(loaded, retained) {
		t.Fatalf("workspace changes relabeled retained intake: %#v %v", loaded, err)
	}
	if _, err := repository.CodingAgentIntakeContext(ctx, planner.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("planning authority read execution intake: %v", err)
	}
	questions := []string{"Choose an approved detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: questions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, questions); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CodingAgentIntakeContext(ctx, executor.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("terminal paused attempt retained projection authority: %v", err)
	}
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "retained-intake-answer", job.ID), JobID: job.ID, Feedback: "Use the approved detail.", WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity, ExpectedGeneration: &executor.Authority.Generation, ExpectedWaitingAttempt: &executor.Authority.Attempt}); err != nil {
		t.Fatal(err)
	}
	resumed, err := repository.ClaimNextStep(ctx, "retained-intake-resume-fixture")
	if err != nil || resumed == nil {
		t.Fatalf("resume: %v %v", resumed, err)
	}
	loaded, err = repository.CodingAgentIntakeContext(ctx, resumed.Authority)
	if err != nil || !reflect.DeepEqual(loaded, retained) {
		t.Fatalf("resumed work lost original redaction provenance: %#v %v", loaded, err)
	}
}

func TestCodingAgentIntakeAllowsAbsentSyntheticContext(t *testing.T) {
	_, repository, _, claim, _ := freshAgentCodingClaim(t)
	context, err := repository.CodingAgentIntakeContext(context.Background(), claim.Authority)
	if err != nil || context != nil {
		t.Fatalf("absent fixture intake became invented context: %#v %v", context, err)
	}
}
