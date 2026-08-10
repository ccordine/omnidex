package cognitiongauntlet

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
)

func BuildLabyrinthProjectionRelevance(
	fixture MicrogauntletCase,
	episode SealedEpisode,
	surfaceVersion string,
) (ProjectionRelevanceEvidence, error) {
	if _, err := ValidateCausalAcquisitionTrace(fixture, episode, surfaceVersion); err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	return buildLabyrinthProjectionRelevance(fixture, episode, surfaceVersion)
}

func BuildPartialLabyrinthProjectionRelevance(
	fixture MicrogauntletCase,
	episode SealedEpisode,
	surfaceVersion string,
) (ProjectionRelevanceEvidence, error) {
	if _, err := MeasureCausalAcquisitionTrace(fixture, episode, surfaceVersion); err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	return buildLabyrinthProjectionRelevance(fixture, episode, surfaceVersion)
}

func buildLabyrinthProjectionRelevance(
	fixture MicrogauntletCase,
	episode SealedEpisode,
	surfaceVersion string,
) (ProjectionRelevanceEvidence, error) {
	oracle := fixture.generated.PrivateOracle()
	oracleManifest, err := fixture.oracleManifest()
	if err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	observations, err := acquisitionObservations(episode)
	if err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	bindings, err := tracedActionBindings(episode)
	if err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	relevant := make(map[ProjectionReferenceIdentity]struct{})
	critical := make(map[string]CriticalProjectionUse)
	for index, use := range oracle.EvidenceUses {
		witnessAcquisition, err := privateWitnessAction(oracle, use.AcquisitionActionID)
		if err != nil {
			return ProjectionRelevanceEvidence{}, err
		}
		witnessConsumer, err := privateWitnessAction(oracle, use.RequiredByActionID)
		if err != nil {
			return ProjectionRelevanceEvidence{}, err
		}
		actualAcquisition, acquired, err := optionalActionForRequest(bindings, witnessAcquisition.Request)
		if err != nil {
			return ProjectionRelevanceEvidence{}, fmt.Errorf("relevance acquisition %d: %w", index+1, err)
		}
		actualConsumer, consumed, err := optionalActionForRequest(bindings, witnessConsumer.Request)
		if err != nil {
			return ProjectionRelevanceEvidence{}, fmt.Errorf("relevance consumer %d: %w", index+1, err)
		}
		if !acquired {
			continue
		}
		observation, exists := observations[actualAcquisition.Action.ID]
		if !exists {
			continue
		}
		if err := observationContainsEvidence(observation, surfaceVersion, use.Evidence); err != nil {
			return ProjectionRelevanceEvidence{}, fmt.Errorf("relevance acquisition %d: %w", index+1, err)
		}
		ref := observationProjectionRef(observation)
		relevant[ref] = struct{}{}
		if !consumed {
			continue
		}
		if actualConsumer.ProjectionID == "" {
			return ProjectionRelevanceEvidence{}, fmt.Errorf("relevance consumer %d has no model-call projection", index+1)
		}
		key := actualConsumer.ProjectionID + "\x00" + projectionReferenceKey(ref)
		critical[key] = CriticalProjectionUse{
			ProjectionID: actualConsumer.ProjectionID, Ref: ref,
			RequiredBytes: int64(len([]byte(observation.Content))),
		}
	}
	relevantRefs := make([]ProjectionReferenceIdentity, 0, len(relevant))
	for ref := range relevant {
		relevantRefs = append(relevantRefs, ref)
	}
	sort.Slice(relevantRefs, func(left, right int) bool {
		return projectionReferenceKey(relevantRefs[left]) < projectionReferenceKey(relevantRefs[right])
	})
	criticalUses := make([]CriticalProjectionUse, 0, len(critical))
	for _, use := range critical {
		criticalUses = append(criticalUses, use)
	}
	sort.Slice(criticalUses, func(left, right int) bool {
		leftKey := criticalUses[left].ProjectionID + "\x00" + projectionReferenceKey(criticalUses[left].Ref)
		rightKey := criticalUses[right].ProjectionID + "\x00" + projectionReferenceKey(criticalUses[right].Ref)
		return leftKey < rightKey
	})
	evidence := ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256, RelevantRefs: relevantRefs, CriticalUses: criticalUses,
	}
	if _, _, err := validateProjectionRelevance(evidence, episode, oracleManifest); err != nil {
		return ProjectionRelevanceEvidence{}, err
	}
	return evidence, nil
}

func observationProjectionRef(observation cognition.Observation) ProjectionReferenceIdentity {
	return ProjectionReferenceIdentity{
		URI: "cognition:episode/" + string(observation.Revision.EpisodeID) +
			"/observation/" + string(observation.ID),
		Version:       strconv.FormatUint(observation.Revision.Number, 10),
		ContentSHA256: observation.ContentSHA256,
	}
}

func projectionReferenceKey(ref ProjectionReferenceIdentity) string {
	return ref.URI + "\x00" + ref.Version + "\x00" + ref.ContentSHA256
}
