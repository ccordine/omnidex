package queue

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type cognitionProjectionFitInput struct {
	Episode            CognitionEpisode
	Current            cognition.Obligation
	Attempt            cognition.AttemptRef
	Budget             cognition.RuntimeBudget
	CompletionEvidence []cognition.EvidenceRef
	FactSources        cognitionFactProjectionSources
	Set                *workingset.Set
	WorkID             string
	Spec               contextbuilder.ContextSpec
	Materials          []contextbuilder.Material
	Initial            contextbuilder.Projection
}

type cognitionProjectionFit struct {
	Projection contextbuilder.Projection
	Snapshot   cognition.RuntimeSnapshot
	Envelope   cognitionpolicy.RenderedEnvelope
}

func fitCognitionPolicyProjection(input cognitionProjectionFitInput) (cognitionProjectionFit, error) {
	spec := input.Spec
	projection := input.Initial
	for {
		candidate, envelope, err := measureCognitionProjection(input, projection)
		if err == nil {
			if cognitionEnvelopeFits(envelope, input.Budget) {
				return cognitionProjectionFit{
					Projection: projection, Snapshot: candidate, Envelope: envelope,
				}, nil
			}
		} else if !errors.Is(err, cognitionpolicy.ErrEnvelopeLimit) {
			return cognitionProjectionFit{}, err
		}

		trimmed, trimErr := trimLowestPriorityOptionalSelection(&spec, projection)
		if trimErr != nil {
			return cognitionProjectionFit{}, trimErr
		}
		if !trimmed {
			if err != nil {
				return cognitionProjectionFit{}, fmt.Errorf(
					"%w: required context cannot produce a policy envelope within the hard renderer limit: %v",
					ErrCognitionEnvelopeBudget, err,
				)
			}
			return cognitionProjectionFit{}, fmt.Errorf(
				"%w: required context needs %d bytes/%d estimated tokens; limits are %d bytes/%d estimated tokens",
				ErrCognitionEnvelopeBudget, envelope.Bytes, envelope.EstimatedTokens,
				input.Budget.MaxInputBytes, input.Budget.MaxInputTokens,
			)
		}
		projection, err = contextbuilder.Build(contextbuilder.BuildInput{
			WorkID: input.WorkID, Spec: spec, WorkingSet: input.Set, Materials: input.Materials,
		})
		if err != nil {
			return cognitionProjectionFit{}, fmt.Errorf("fit cognition context projection: %w", err)
		}
	}
}

func measureCognitionProjection(
	input cognitionProjectionFitInput,
	projection contextbuilder.Projection,
) (cognition.RuntimeSnapshot, cognitionpolicy.RenderedEnvelope, error) {
	ref := cognitionProjectionReference(projection)
	modelEvidence, err := projectedCognitionEvidenceRefs(
		projection, input.CompletionEvidence, input.FactSources,
		input.Episode.CurrentRevision, input.Current,
	)
	if err != nil {
		return cognition.RuntimeSnapshot{}, cognitionpolicy.RenderedEnvelope{}, err
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		input.Episode.Goal, input.Episode.CurrentRevision, input.Current,
		input.Episode.ActionCatalog, input.Attempt, ref, input.Budget, modelEvidence,
	)
	if err != nil {
		return cognition.RuntimeSnapshot{}, cognitionpolicy.RenderedEnvelope{}, err
	}
	envelope, err := cognitionpolicy.MeasureEnvelope(snapshot, projection)
	return snapshot, envelope, err
}

func projectedCognitionEvidenceRefs(
	projection contextbuilder.Projection,
	completion []cognition.EvidenceRef,
	factSources cognitionFactProjectionSources,
	currentRevision cognition.WorldRevision,
	current cognition.Obligation,
) ([]cognition.EvidenceRef, error) {
	available := make(map[taskstate.Ref]cognition.EvidenceRef, len(completion))
	for _, ref := range completion {
		key := taskstate.Ref{
			URI: cognitionEvidenceTaskRef(ref), Version: strconv.FormatUint(ref.Revision.Number, 10),
			Hash: ref.SHA256, Relation: taskstate.RefEvidence,
		}
		available[key] = ref
	}
	model := make([]cognition.EvidenceRef, 0)
	projected := make(map[cognition.EvidenceRef]struct{})
	appendEvidence := func(ref cognition.EvidenceRef) {
		if _, exists := projected[ref]; exists {
			return
		}
		projected[ref] = struct{}{}
		model = append(model, ref)
	}
	for _, selected := range projection.Selected {
		switch selected.Role {
		case workingset.RoleEvidence:
			ref, exists := available[selected.Ref]
			if !exists {
				return nil, fmt.Errorf(
					"%w: projected evidence %q has no exact completion packet source",
					ErrCognitionConflict, selected.ItemID,
				)
			}
			appendEvidence(ref)
		case workingset.RoleFact:
			sources, accepted := factSources[selected.Ref]
			if !accepted || len(sources) == 0 || len(selected.SourceRefs) != len(sources) {
				return nil, fmt.Errorf(
					"%w: projected fact %q lacks exact accepted-fact authority",
					ErrCognitionConflict, selected.ItemID,
				)
			}
			for index, source := range sources {
				key := cognitionEvidenceTaskRefs([]cognition.EvidenceRef{source})[0]
				if selected.SourceRefs[index] != key || source.Revision == currentRevision {
					return nil, fmt.Errorf(
						"%w: projected fact %q has changed or current-revision lineage",
						ErrCognitionConflict, selected.ItemID,
					)
				}
				if exact, exists := available[key]; !exists || exact != source {
					return nil, fmt.Errorf(
						"%w: projected fact %q source is outside completion evidence",
						ErrCognitionConflict, selected.ItemID,
					)
				}
				appendEvidence(source)
			}
		}
	}
	for _, required := range current.SupportingRefs {
		if _, exists := projected[required]; !exists {
			return nil, fmt.Errorf(
				"%w: active obligation supporting evidence was omitted from model context",
				ErrCognitionConflict,
			)
		}
	}
	return model, nil
}

func cognitionEnvelopeFits(envelope cognitionpolicy.RenderedEnvelope, budget cognition.RuntimeBudget) bool {
	return envelope.Bytes <= budget.MaxInputBytes &&
		envelope.EstimatedTokens <= budget.MaxInputTokens
}

func cognitionProjectionReference(projection contextbuilder.Projection) cognition.ContextProjectionRef {
	return cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.ID), SHA256: projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.WorkingSetID),
		WorkingSetVersion: projection.WorkingSetVersion, RendererVersion: projection.RendererVersion,
	}
}

func trimLowestPriorityOptionalSelection(
	spec *contextbuilder.ContextSpec,
	projection contextbuilder.Projection,
) (bool, error) {
	selectedByRole := make(map[workingset.Role]int)
	minimumByRole := make(map[workingset.Role]int)
	for _, selected := range projection.Selected {
		selectedByRole[selected.Role]++
	}
	for _, selector := range spec.Required {
		minimumByRole[selector.Role] = selector.MinItems
	}
	for index := len(projection.Selected) - 1; index >= 0; index-- {
		role := projection.Selected[index].Role
		selected := selectedByRole[role]
		if selected <= minimumByRole[role] {
			continue
		}
		for selectorIndex := range spec.Required {
			if spec.Required[selectorIndex].Role == role {
				spec.Required[selectorIndex].MaxItems = selected - 1
				return true, nil
			}
		}
		for selectorIndex := range spec.Optional {
			if spec.Optional[selectorIndex].Role != role {
				continue
			}
			if selected == 1 {
				spec.Optional = append(spec.Optional[:selectorIndex], spec.Optional[selectorIndex+1:]...)
			} else {
				spec.Optional[selectorIndex].MaxItems = selected - 1
			}
			return true, nil
		}
		return false, fmt.Errorf(
			"%w: selected role %q has no registered context selector", ErrCognitionConflict, role,
		)
	}
	return false, nil
}
