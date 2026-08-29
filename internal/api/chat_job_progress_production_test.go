package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestProductionStepEventFormsHaveTypedGUIProjections(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	forms := []string{
		"step_start phase=coding action=v3_coding worker=worker-1",
		"step_complete action=v3_coding worker=worker-1",
		"step_canceled action=v3_coding worker=worker-1",
		"step_error exact deterministic failure",
		"operation_heartbeat operation=web research elapsed=30s",
		"coding_phase_changed phase=assembling detail=compiling deterministic source assembly",
		"coding_specification_accepted surface=browser requirements=3 product_bytes=920",
		"coding_assembly_ready adapter=typescript files=5 blocks=9 waves=3",
		"coding_artifact_sieve_passed stack=typescript_browser_v1 files=5",
		"coding_workload_frozen tasks=3 sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"coding_stage_started attempt=1 generated_blocks=9",
		"coding_stage_passed attempt=1 generated_blocks=9",
		"coding_fragment_repair_guidance_started block=feature.render exact_failure=one exact failure",
		"coding_fragment_correction_started block=feature.render guidance_bytes=128",
		"coding_compiler_repair_applied block=feature.render mechanism=deterministic_primitive_nullish_narrowing",
		"coding_target_tree_validation_failed diagnostic=one exact tree failure",
		"application_evidence_need_opened need=evidence_001 source=repository stop=one exact fact",
		"application_evidence_need_resolved need=evidence_001 facts=2 stop=one exact fact",
		"coding_skill_bound requirement=requirement_001 skill=skill-1 version=2 source=registry status=active",
		"repository_snapshot_started authority=server",
		"repository_snapshot_failed exact snapshot failure",
		"repository_snapshot_ready snapshot=sha256:abc files=7",
		"repository_analysis_started snapshot=sha256:abc adapter=golang_v1",
		"repository_analysis_failed exact analysis failure",
		"repository_analysis_ready snapshot=sha256:abc adapter=golang_v1 analysis=sha256:def",
		"repository_change_staged contract=contract-1 files=2",
		"repository_change_completed contract=contract-1 files=2 snapshot=sha256:def",
		"repository_desired_state_staged graph=desired-1 files=1",
		"repository_desired_state_verified graph=desired-1 files=1 snapshot=sha256:ghi",
		"repository_verification_command_passed scope=staged command=go_test_./...",
		"repository_verification_baseline_accepted scope=baseline plan=sha256:baseline",
		"repository_verification_plan_accepted scope=staged plan=sha256:plan",
		"workspace_mutation_recovery_started stage=stage-1 source=workspace-state-1",
		"workspace_mutation_recovered operation=operation-1 expected=workspace-state-2",
		"coding_portable_dispatched kind=fragment_generation work=abcdef123456 payload=420B model=local",
		"coding_worker_started kind=fragment subject=feature.render model=local attempt=1/3 context=prompt:420B,capabilities:20B,current:0B,correction:0B",
		"coding_worker_completed kind=fragment subject=feature.render model=local attempt=1/3",
		"objective_worker_rejected kind=semantic subject=conversation_response model=local attempt=1/3 error=invalid typed leaf",
		"web_research_worker_failed kind=semantic subject=web_relevance model=local attempt=3/3 error=exact station failure",
	}
	progress := queue.JobProgressPage{JobID: 7, Generation: 1}
	for index, form := range forms {
		contextID := int64(index + 1)
		progress.Items = append(progress.Items, queue.JobProgressContext{
			Context: model.StepContext{
				ID: contextID, StepID: 11, Key: "event",
				Value:     fmt.Sprintf("time=%s event=%s", now.Format(time.RFC3339), form),
				CreatedAt: now,
			},
			Generation: 1, StepAction: "v3_coding",
		})
		progress.LatestContextID = contextID
		if _, err := projectChatProgress(queue.JobProgressPage{
			JobID: 7, Generation: 1, LatestContextID: contextID,
			Items: progress.Items[len(progress.Items)-1:],
		}); err != nil {
			t.Errorf("production event %q has no valid projection: %v", form, err)
		}
	}
}

func TestChatProgressRejectsRemovedFixedCorrectionCounts(t *testing.T) {
	t.Parallel()

	for _, event := range []parsedChatStepEvent{
		{Type: "coding_task_verified", Message: "task=application_task_001 corrections_remaining=2"},
		{Type: "coding_file_written", Message: "path=main.go bytes=12 operation=create result=accepted"},
		{Type: "coding_verification_failed", Message: "command=go_test diagnostic=failure"},
		{Type: "coding_fragment_correction_started", Message: "block=feature.render correction=1 exact_failure=failure"},
	} {
		if _, _, err := summarizeChatStepEvent(event, "v3_coding"); err == nil {
			t.Fatalf("obsolete fixed-count event was accepted: %#v", event)
		}
	}
}

func TestChatProgressRejectsRemovedAggregateRepositoryIndexEvents(t *testing.T) {
	t.Parallel()

	for _, event := range []parsedChatStepEvent{
		{Type: "repository_index_started", Message: "authority=server"},
		{Type: "repository_index_failed", Message: "exact indexing failure"},
		{Type: "repository_index_ready", Message: "snapshot=sha256:abc files=7 analyses=2"},
	} {
		if _, _, err := summarizeChatStepEvent(event, "v3_coding"); err == nil {
			t.Fatalf("obsolete aggregate repository event was accepted: %#v", event)
		}
	}
}
