package cognitionenv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (environment *Environment) startRevision() (cognition.WorldRevision, error) {
	digest, err := exactDigest(struct {
		Schema   string
		Scenario cognition.ScenarioRef
		Episode  cognition.EpisodeRef
		Stage    investigationStage
	}{ScenarioSchemaV2, environment.investigation.ref, environment.episode, stageStart})
	if err != nil {
		return cognition.WorldRevision{}, err
	}
	return cognition.NewWorldRevision(environment.episode.ID, 1, digest)
}

func (environment *Environment) nextRevision(
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
	pack repositoryretrieval.EvidencePack,
	state investigationState,
) (cognition.WorldRevision, error) {
	digest, err := exactDigest(struct {
		Schema   string
		Scenario cognition.ScenarioRef
		Episode  cognition.EpisodeRef
		Previous cognition.WorldRevision
		Action   cognition.RegisteredAction
		PackID   string
		State    investigationState
	}{
		ScenarioSchemaV2, environment.investigation.ref, environment.episode,
		expected, action, pack.ID, state,
	})
	if err != nil {
		return cognition.WorldRevision{}, err
	}
	return cognition.NewWorldRevision(environment.episode.ID, expected.Number+1, digest)
}

func (environment *Environment) startTransition() (cognition.Transition, error) {
	revision, err := environment.startRevision()
	if err != nil {
		return cognition.Transition{}, err
	}
	contentRaw, err := json.Marshal(struct {
		Schema       string                        `json:"schema"`
		Need         NeedAuthority                 `json:"accepted_need"`
		Operation    repositoryretrieval.Operation `json:"terminal_evidence_operation"`
		QueryBinding string                        `json:"query_binding"`
	}{
		ScenarioSchemaV2, environment.investigation.need, environment.investigation.operation,
		environment.investigation.queryBinding,
	})
	if err != nil {
		return cognition.Transition{}, err
	}
	observation, err := cognition.NewObservation(
		observationID("need", environment.investigation.queryBinding), revision,
		ObservationNeed, string(contentRaw),
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	transition := cognition.Transition{
		Current: revision, Observations: []cognition.Observation{observation},
		Effects: []cognition.Effect{}, Cost: 0,
	}
	if err := transition.ValidateStart(); err != nil {
		return cognition.Transition{}, err
	}
	return transition, nil
}

func (environment *Environment) actionTransition(
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
	pack repositoryretrieval.EvidencePack,
	state investigationState,
	terminal bool,
) (cognition.Transition, error) {
	current, err := environment.nextRevision(expected, action, pack, state)
	if err != nil {
		return cognition.Transition{}, err
	}
	packRaw, err := json.Marshal(pack)
	if err != nil {
		return cognition.Transition{}, fmt.Errorf("encode repository evidence observation: %w", err)
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return cognition.Transition{}, fmt.Errorf("encode repository investigation state: %w", err)
	}
	evidence, err := cognition.NewActionObservation(
		observationID("evidence", pack.ID), action.ID, current,
		ObservationEvidence, string(packRaw),
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	stateObservation, err := cognition.NewActionObservation(
		observationID("state", current.SHA256), action.ID, current,
		ObservationState, string(stateRaw),
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	effect, err := cognition.NewEffect(
		action.ID, current, cognition.EffectObservationProduced,
		"one bounded repository evidence pack and opaque investigation state were acquired",
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	transition := cognition.Transition{
		ActionID: action.ID, Previous: &expected, Current: current,
		Observations: []cognition.Observation{evidence, stateObservation},
		Effects:      []cognition.Effect{effect}, Cost: 1, Terminal: terminal,
	}
	if terminal {
		transition.PublicOutcome = PublicOutcomeEvidenceAcquired
	}
	if err := transition.ValidateApply(environment.episode, expected, action); err != nil {
		return cognition.Transition{}, err
	}
	return transition, nil
}

func latestStateEvidence(
	journal cognition.EnvironmentJournalState,
) (cognition.EvidenceRef, error) {
	if journal.Current == journal.Start.Current {
		return journal.Start.Observations[0].EvidenceRef(), nil
	}
	if journal.CurrentReceipt == nil || journal.CurrentReceipt.Transition == nil {
		return cognition.EvidenceRef{}, fmt.Errorf("repository investigation has no current receipt")
	}
	for _, observation := range journal.CurrentReceipt.Transition.Observations {
		if observation.Kind == ObservationState {
			return observation.EvidenceRef(), nil
		}
	}
	return cognition.EvidenceRef{}, fmt.Errorf("repository investigation current receipt has no state evidence")
}

func (environment *Environment) buildEvidence(
	ctx context.Context,
	request repositoryretrieval.Request,
) (repositoryretrieval.EvidencePack, error) {
	pack, err := environment.builder.Build(ctx, request)
	if err != nil {
		return repositoryretrieval.EvidencePack{}, err
	}
	if err := pack.ValidateForRequest(request.Operation, request.Query); err != nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository cognition evidence: %w", err)
	}
	if pack.SnapshotID != environment.investigation.snapshot.ID ||
		pack.AnalysisID != environment.investigation.analysis.ID {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"repository cognition evidence changed its exact snapshot or analysis authority",
		)
	}
	if err := environment.rejectWholeFileEvidence(pack); err != nil {
		return repositoryretrieval.EvidencePack{}, err
	}
	return pack, nil
}

func (environment *Environment) rejectWholeFileEvidence(
	pack repositoryretrieval.EvidencePack,
) error {
	files := make(map[string]int64, len(environment.investigation.snapshot.Files))
	for _, file := range environment.investigation.snapshot.Files {
		files[file.ID] = file.Size
	}
	symbols := make(map[string]struct {
		fileID             string
		startByte, endByte int64
	}, len(environment.investigation.analysis.Symbols))
	for _, symbol := range environment.investigation.analysis.Symbols {
		symbols[symbol.ID] = struct {
			fileID             string
			startByte, endByte int64
		}{symbol.FileID, symbol.StartByte, symbol.EndByte}
	}
	for _, evidence := range pack.Symbols {
		symbol, exists := symbols[evidence.ID]
		if !exists {
			return fmt.Errorf("repository evidence contains an unknown symbol")
		}
		fileSize := files[symbol.fileID]
		if evidence.Source != "" && (int64(len(evidence.Source)) >= fileSize ||
			(symbol.startByte == 0 && symbol.endByte == fileSize)) {
			return fmt.Errorf("repository cognition refuses to expose a whole file as symbol evidence")
		}
	}
	return nil
}

func observationID(kind, identity string) cognition.ObservationID {
	digest, err := exactDigest(struct{ Kind, Identity string }{kind, identity})
	if err != nil {
		panic(err)
	}
	return cognition.ObservationID("repository-observation-" + digest)
}
