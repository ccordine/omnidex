package worker

import (
	"os/exec"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type desiredStateProductCase struct {
	name, instruction, target string
	present                   bool
	wantKinds                 []assemblyline.WorkKind
	wantGenerationCalls       int
}

// This is deliberately labeled production plumbing, not Gate D autonomy. The
// fixture-aware provider makes the run contaminated while still proving the
// unchanged channel/job/station/stage/journal/apply/reindex completion route.
func TestPostgresOrdinaryChannelDesiredStateContaminatedProductionPlumbing(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap is required for the production desired-state product vertical")
	}
	t.Setenv("GOMODCACHE", t.TempDir())
	cases := []desiredStateProductCase{
		{
			name:        "create",
			instruction: "An independently owned Go artifact declaring func Added() int that returns 2 must exist.",
			target:      "omni_added_artifact.go", present: true, wantGenerationCalls: 1,
			wantKinds: []assemblyline.WorkKind{
				assemblyline.WorkConversationObjectiveKind,
				assemblyline.WorkApplicationContextNeedCoverage,
				assemblyline.WorkRepositoryRequirement,
				assemblyline.WorkRepositoryRequirementCoverage,
				assemblyline.WorkRepositoryArtifactAbsence,
				assemblyline.WorkDeclarationArtifactBoundary,
				assemblyline.WorkFragmentGeneration,
			},
		},
		{
			name:        "delete-path-free",
			instruction: "The known Go artifact declaring func Obsolete() int and all behavior it owns must no longer exist.",
			target:      "obsolete.go", present: false, wantGenerationCalls: 0,
			wantKinds: []assemblyline.WorkKind{
				assemblyline.WorkConversationObjectiveKind,
				assemblyline.WorkApplicationContextNeedCoverage,
				assemblyline.WorkRepositoryRequirement,
				assemblyline.WorkRepositoryRequirementCoverage,
				assemblyline.WorkRepositoryArtifactAbsence,
				assemblyline.WorkArtifactCandidateSelection,
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runDesiredStateProductVertical(t, test)
		})
	}
}

func TestPostgresOrdinaryChannelDeletionProductionPlumbingKeepsTargetPathModelBlind(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap is required for the production desired-state product vertical")
	}
	t.Setenv("GOMODCACHE", t.TempDir())
	test := desiredStateProductCase{
		name: "delete-red", instruction: "obsolete.go must no longer exist.",
		target: "obsolete.go", present: false, wantGenerationCalls: 0,
		wantKinds: []assemblyline.WorkKind{
			assemblyline.WorkConversationObjectiveKind,
			assemblyline.WorkApplicationContextNeedCoverage,
			assemblyline.WorkArtifactHandling,
			assemblyline.WorkRepositoryRequirement,
			assemblyline.WorkRepositoryRequirementCoverage,
		},
	}
	// The physical filename is redacted before every semantic boundary and is
	// restored only by code when compiling the verified repository transition.
	runDesiredStateProductVertical(t, test)
}

func runDesiredStateProductVertical(t *testing.T, test desiredStateProductCase) {
	t.Helper()
	root := desiredStateProductRepository(t, !test.present)
	before, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, repository, pool := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: model.ChannelID("desired-state-product-" + test.name), Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "Desired state product " + test.name, WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	message, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, test.instruction)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != test.instruction || job.Instruction != test.instruction ||
		job.Pipeline != model.PipelineChat {
		t.Fatalf("ordinary channel authority was rewritten: message=%q job=%q pipeline=%q", message.Content, job.Instruction, job.Pipeline)
	}
	claim, err := repository.ClaimNextStep(ctx, "desired-state-product-worker-"+test.name)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "objective_resolve" {
		t.Fatalf("ordinary production claim=%+v want job=%d objective_resolve", claim, job.ID)
	}
	provider := &desiredStateProductProvider{}
	service := desiredStateProductService(t, repository, provider, root)
	if err := service.processStep(ctx, claim); err != nil {
		t.Fatal(err)
	}
	after, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertDesiredStateInventoryDelta(t, before, after, test.target, test.present)
	assertDesiredStateProductFilesystem(t, root, test)
	summary := assertDesiredStateProductCompletion(t, repository, channel, job, test)
	assertDesiredStateProductStationEvidence(t, pool, job.ID, provider.Calls(), test)
	assertDesiredStateProductMutationEvidence(t, pool, channel.ProjectID, job.ID, before, after, test)
	assertDesiredStateProductSummary(t, summary, test)
}
