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
		"coding_file_written path=src/main.ts bytes=42 operation=create result=accepted",
		"coding_file_deleted path=src/old.ts result=deleted",
		"coding_file_unchanged path=src/main.ts",
		"coding_file_delete_skipped path=src/old.ts reason=missing",
		"coding_verification_started commands=2",
		"coding_verification_failed command=npm_test diagnostic=one exact failure",
		"coding_verification_command_passed command=npm_test",
		"coding_static_validation_failed diagnostic=one exact failure",
		"coding_stage_started attempt=1 generated_blocks=9",
		"coding_stage_passed attempt=1 generated_blocks=9",
		"coding_fragment_correction_started block=feature.render correction=1 exact_failure=one exact failure",
		"coding_skill_bound requirement=requirement_001 skill=skill-1 version=2 source=registry status=active",
		"repository_index_started authority=server",
		"repository_index_failed exact indexing failure",
		"repository_index_ready snapshot=sha256:abc files=7 analyses=2",
		"repository_change_staged contract=contract-1 files=2",
		"repository_change_completed contract=contract-1 files=2 snapshot=sha256:def",
		"repository_desired_state_staged graph=desired-1 files=1",
		"repository_desired_state_verified graph=desired-1 files=1 snapshot=sha256:ghi",
		"repository_verification_command_passed scope=staged command=go_test_./...",
		"repository_verification_baseline_accepted scope=baseline plan=sha256:baseline",
		"repository_verification_plan_accepted scope=staged plan=sha256:plan",
		"repository_mutation_recovery_started stage=stage-1 snapshot=sha256:abc",
		"repository_mutation_recovered stage=stage-1 snapshot=sha256:abc",
		"external_agent_started codex_sdk",
		"external_agent_failed exact external failure",
		"external_agent_unavailable codex sdk is not configured",
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
