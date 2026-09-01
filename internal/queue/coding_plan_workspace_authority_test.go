package queue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestLifecycleWorkspaceAuthorityIsConditionalAndExact(t *testing.T) {
	t.Parallel()
	const root = "/tmp/lifecycle-workspace-authority"
	const identity = "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const otherIdentity = "directory_identity_v1_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	chatMetadata, err := marshalChannelTurnMetadata(
		model.ChannelID("lifecycle-workspace-authority"),
		1,
		root,
		"",
		"",
		model.ChannelModeAssistant,
		modelconfig.Config{},
		model.CodingScopeModeNormal,
		nil,
		identity,
	)
	if err != nil {
		t.Fatal(err)
	}
	codingMetadata, err := json.Marshal(map[string]string{
		"client_cwd": root, "client_workspace_identity": identity,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, job := range map[string]model.Job{
		"CLI chat": {ID: 41, Pipeline: model.PipelineChat, Metadata: chatMetadata},
		"coding":   {ID: 42, Pipeline: model.PipelineCoding, Metadata: codingMetadata},
	} {
		job := job
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := requireLifecycleWorkspaceAuthority(job, root, identity); err != nil {
				t.Fatalf("exact authority: %v", err)
			}
			if err := requireLifecycleWorkspaceAuthority(job, "", ""); err == nil ||
				!strings.Contains(err.Error(), "required") {
				t.Fatalf("omitted authority error = %v", err)
			}
			if err := requireLifecycleWorkspaceAuthority(job, root, ""); err == nil ||
				!strings.Contains(err.Error(), "required") {
				t.Fatalf("partial authority error = %v", err)
			}
			if err := requireLifecycleWorkspaceAuthority(job, root, otherIdentity); !errors.Is(
				err, ErrChannelSessionWorkspace,
			) {
				t.Fatalf("wrong authority error = %v", err)
			}
		})
	}

	unboundChatMetadata, err := marshalChannelTurnMetadata(
		model.ChannelID("unbound-lifecycle-authority"),
		1,
		root,
		"",
		"",
		model.ChannelModeAssistant,
		modelconfig.Config{},
		model.CodingScopeModeNormal,
		nil,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, job := range map[string]model.Job{
		"unbound chat": {ID: 43, Pipeline: model.PipelineChat, Metadata: unboundChatMetadata},
		"Scrum":        {ID: 44, Pipeline: model.PipelineScrum},
	} {
		job := job
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := requireLifecycleWorkspaceAuthority(job, "", ""); err != nil {
				t.Fatalf("omitted non-workspace authority: %v", err)
			}
			if err := requireLifecycleWorkspaceAuthority(job, root, identity); !errors.Is(
				err, ErrChannelSessionWorkspace,
			) {
				t.Fatalf("phantom workspace authority error = %v", err)
			}
		})
	}
}

func TestCodingPlanWorkspaceAuthorityIsRequiredAndExact(t *testing.T) {
	t.Parallel()
	const root = "/tmp/coding-plan-authority"
	const identity = "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	metadata, err := json.Marshal(map[string]string{
		"client_cwd": root, "client_workspace_identity": identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	job := model.Job{ID: 41, Pipeline: model.PipelineCoding, Metadata: metadata}
	if err := requireCodingPlanWorkspaceAuthority(job, root, identity); err != nil {
		t.Fatalf("accept exact coding-plan workspace authority: %v", err)
	}
	if err := requireCodingPlanWorkspaceAuthority(job, "", ""); err == nil ||
		!strings.Contains(err.Error(), "required") {
		t.Fatalf("omitted coding-plan workspace authority error = %v", err)
	}
	if err := requireCodingPlanWorkspaceAuthority(job, "/tmp/other", identity); !errors.Is(
		err, ErrChannelSessionWorkspace,
	) {
		t.Fatalf("cross-workspace coding-plan authority error = %v", err)
	}
}

func TestCodingPlanCommandsRejectOmittedWorkspaceAuthority(t *testing.T) {
	t.Parallel()
	operationID, err := NewLifecycleOperationID("coding-plan-required-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeApplyCodingPlanDecisionsCommand(ApplyCodingPlanDecisionsCommand{
		OperationID: operationID, JobID: 1, Generation: 1, Revision: 1,
	}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("omitted decision workspace authority error = %v", err)
	}
	if _, err := normalizeFreezeCodingPlanCommand(FreezeCodingPlanCommand{
		OperationID: operationID, JobID: 1, Generation: 1, Revision: 1,
	}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("omitted freeze workspace authority error = %v", err)
	}
}
