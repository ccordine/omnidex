package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const SemanticPreCallCheckpointSchemaV1 = "omnidex.cognition-semantic-pre-call.v1"

type PreCallBoundAuthority struct {
	Attempt        cognition.AttemptRef           `json:"attempt"`
	Projection     cognition.ContextProjectionRef `json:"projection"`
	SnapshotSHA256 string                         `json:"snapshot_sha256"`
}

type SemanticPreCallCheckpoint struct {
	Schema                   string                `json:"schema"`
	Bound                    PreCallBoundAuthority `json:"bound_authority"`
	SemanticSHA256           string                `json:"semantic_sha256"`
	ProjectionRenderedSHA256 string                `json:"projection_rendered_sha256"`
}

type semanticPreCallState struct {
	Goal                     cognition.GoalExpression          `json:"goal"`
	Revision                 cognition.WorldRevision           `json:"revision"`
	Graph                    cognition.ObligationGraphSnapshot `json:"obligation_graph"`
	GraphVersion             uint64                            `json:"graph_version"`
	Ledger                   taskstate.MaterializedState       `json:"task_ledger"`
	WorkingSet               workingset.Snapshot               `json:"working_set"`
	ActiveObligation         cognition.Obligation              `json:"active_obligation"`
	Catalog                  cognition.ActionCatalog           `json:"action_catalog"`
	Budget                   cognition.RuntimeBudget           `json:"runtime_budget"`
	ModelEvidence            []cognition.EvidenceRef           `json:"model_evidence"`
	CompletionEvidence       []cognition.EvidenceRef           `json:"completion_evidence"`
	EnvironmentTerminal      bool                              `json:"environment_terminal"`
	PublicOutcome            string                            `json:"public_outcome"`
	Brain                    cognitionpolicy.AttestedBrain     `json:"attested_brain"`
	ProjectionSpecName       string                            `json:"projection_spec_name"`
	ProjectionSpecVersion    string                            `json:"projection_spec_version"`
	ProjectionSpecSHA256     string                            `json:"projection_spec_sha256"`
	ProjectionRenderer       string                            `json:"projection_renderer"`
	ProjectionRendered       string                            `json:"projection_rendered"`
	ProjectionTokenEstimator string                            `json:"projection_token_estimator"`
}

func CaptureSemanticPreCallCheckpoint(
	ctx context.Context,
	repository *queue.Repository,
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
) (SemanticPreCallCheckpoint, error) {
	if ctx == nil || repository == nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call capture requires context and PostgreSQL")
	}
	record, err := repository.PrepareCognitionRuntimeSnapshot(ctx, queue.CognitionRuntimeSnapshotCommand{
		Authority: authority, EpisodeID: episodeID,
	})
	if err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("prepare semantic pre-call snapshot: %w", err)
	}
	projection, err := repository.GetContextProjection(
		ctx, string(record.Prepared.Snapshot.ContextProjection().ID),
	)
	if err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("load semantic pre-call projection: %w", err)
	}
	if projection.Authority.StepAttemptAuthority != authority {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call projection belongs to another attempt")
	}
	ledger, err := repository.TaskLedger(ctx, authority.JobID)
	if err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("load semantic pre-call Task Ledger: %w", err)
	}
	set, err := repository.CurrentWorkingSet(ctx, authority.JobID)
	if err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("load semantic pre-call Working Set: %w", err)
	}
	episode, err := repository.CognitionEpisode(ctx, episodeID)
	if err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("load semantic pre-call episode: %w", err)
	}
	return NewSemanticPreCallCheckpoint(
		record.Prepared, projection.Projection, ledger, set, episode.AttestedBrain,
	)
}

func NewSemanticPreCallCheckpoint(
	prepared cognitionruntime.PreparedSnapshot,
	projection contextbuilder.Projection,
	ledger taskstate.MaterializedState,
	set workingset.Snapshot,
	brain cognitionpolicy.AttestedBrain,
) (SemanticPreCallCheckpoint, error) {
	episode, err := cognition.NewEpisodeRef(prepared.Snapshot.CurrentRevision().EpisodeID)
	if err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	binding, err := cognitionruntime.NewBinding(episode, prepared.Snapshot.Attempt())
	if err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	if err := prepared.ValidateFor(binding); err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	if err := projection.Validate(); err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	if err := brain.Validate(); err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	if _, err := taskstate.RestoreLedger(ledger); err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call Task Ledger: %w", err)
	}
	if err := workingset.ValidateSnapshot(set); err != nil {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call Working Set: %w", err)
	}
	ref := prepared.Snapshot.ContextProjection()
	if ref.ID != cognition.ContextProjectionID(projection.ID) || ref.SHA256 != projection.RenderedSHA256 ||
		ref.WorkingSetID != cognition.WorkingSetID(projection.WorkingSetID) ||
		ref.WorkingSetVersion != projection.WorkingSetVersion || ref.RendererVersion != projection.RendererVersion {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call projection differs from its snapshot authority")
	}
	if ledger.Owner.JobID != prepared.Snapshot.Attempt().JobID ||
		set.Owner.JobID != prepared.Snapshot.Attempt().JobID ||
		set.Owner.Generation != prepared.Snapshot.Attempt().Generation ||
		string(set.ID) != string(ref.WorkingSetID) || set.Version != ref.WorkingSetVersion {
		return SemanticPreCallCheckpoint{}, fmt.Errorf("semantic pre-call durable state belongs to another authority")
	}
	state := semanticPreCallState{
		Goal: prepared.Snapshot.Goal(), Revision: prepared.Snapshot.CurrentRevision(),
		Graph: prepared.ObligationGraph.Clone(), GraphVersion: prepared.GraphVersion,
		Ledger: ledger, WorkingSet: set, ActiveObligation: prepared.Snapshot.CurrentObligation(),
		Catalog: prepared.Snapshot.ActionCatalog(), Budget: prepared.Snapshot.Budget(),
		ModelEvidence:       append([]cognition.EvidenceRef{}, prepared.Snapshot.EvidenceRefs()...),
		CompletionEvidence:  append([]cognition.EvidenceRef{}, prepared.CompletionEvidenceRefs...),
		EnvironmentTerminal: prepared.EnvironmentTerminal, PublicOutcome: prepared.PublicOutcome,
		Brain: brain, ProjectionSpecName: projection.SpecName,
		ProjectionSpecVersion: projection.SpecVersion, ProjectionSpecSHA256: projection.SpecSHA256,
		ProjectionRenderer: projection.RendererVersion, ProjectionRendered: projection.Rendered,
		ProjectionTokenEstimator: projection.TokenEstimator,
	}
	semanticSHA, err := digestJSON(state)
	if err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	checkpoint := SemanticPreCallCheckpoint{
		Schema: SemanticPreCallCheckpointSchemaV1,
		Bound: PreCallBoundAuthority{
			Attempt: prepared.Snapshot.Attempt(), Projection: ref,
			SnapshotSHA256: prepared.Snapshot.SHA256(),
		},
		SemanticSHA256: semanticSHA, ProjectionRenderedSHA256: projection.RenderedSHA256,
	}
	return checkpoint, checkpoint.Validate()
}

func (checkpoint SemanticPreCallCheckpoint) Validate() error {
	if checkpoint.Schema != SemanticPreCallCheckpointSchemaV1 ||
		checkpoint.Bound.Attempt.Validate() != nil || checkpoint.Bound.Projection.Validate() != nil ||
		!validDigest(checkpoint.Bound.SnapshotSHA256) || !validDigest(checkpoint.SemanticSHA256) ||
		!validDigest(checkpoint.ProjectionRenderedSHA256) ||
		checkpoint.Bound.Projection.SHA256 != checkpoint.ProjectionRenderedSHA256 {
		return fmt.Errorf("semantic pre-call checkpoint authority is invalid")
	}
	return nil
}
