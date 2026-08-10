package labyrinth

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateCausalActionEvidence(
	action cognition.RegisteredAction,
	facts factSet,
	observations map[cognition.ObservationID]cognition.Observation,
	records []PublicRecord,
) error {
	setID := EntityID(actionArgument(action.Request, evidenceSetArg))
	if setID == "" {
		return nil
	}
	recordHashes := make(map[EntityID]string, len(records))
	for _, record := range records {
		recordHashes[record.ID] = record.ContentSHA256
	}
	required := make(map[EntityID]string)
	for _, fact := range facts {
		if fact.Name != "evidence.member" || len(fact.Args) != 2 || EntityID(fact.Args[0]) != setID {
			continue
		}
		id := EntityID(fact.Args[1])
		hash, exists := recordHashes[id]
		if !exists {
			return cognition.ErrInvalidEvidence
		}
		required[id] = hash
	}
	if len(required) == 0 {
		return cognition.ErrInvalidEvidence
	}
	grounded := make(map[EntityID]string)
	for _, ref := range action.EvidenceRefs {
		observation := observations[ref.ObservationID]
		for id, hash := range observedEvidenceRecords(observation.Content) {
			grounded[id] = hash
		}
	}
	for id, hash := range required {
		if grounded[id] != hash {
			return cognition.ErrInvalidEvidence
		}
	}
	return nil
}

func observedEvidenceRecords(content string) map[EntityID]string {
	result := make(map[EntityID]string)
	var symbolic publicObservationPayload
	if json.Unmarshal([]byte(content), &symbolic) == nil && symbolic.Format == "symbolic-observation.v1" {
		for _, record := range symbolic.Records {
			if record.Content != "" && textSHA256(record.Content) == record.ContentSHA256 {
				result[record.ID] = record.ContentSHA256
			}
		}
		return result
	}
	var surface struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(content), &surface) != nil || len(surface.Result) == 0 {
		return result
	}
	var payload struct {
		Records []struct {
			ID            EntityID `json:"id"`
			Content       string   `json:"content"`
			SHA256        string   `json:"sha256"`
			ContentSHA256 string   `json:"content_sha256"`
		} `json:"records"`
		Matches []struct {
			ID            EntityID `json:"id"`
			Content       string   `json:"content"`
			ContentSHA256 string   `json:"content_sha256"`
		} `json:"matches"`
	}
	if json.Unmarshal(surface.Result, &payload) != nil {
		return result
	}
	for _, record := range payload.Records {
		hash := record.ContentSHA256
		if hash == "" {
			hash = record.SHA256
		}
		if record.ID != "" && record.Content != "" && textSHA256(record.Content) == hash {
			result[record.ID] = hash
		}
	}
	for _, match := range payload.Matches {
		if match.ID != "" && match.Content != "" && textSHA256(match.Content) == match.ContentSHA256 {
			result[match.ID] = match.ContentSHA256
		}
	}
	return result
}

func generationEvidenceRefs(schema cognition.ActionSchema) []cognition.EvidenceRef {
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		return nil
	}
	digest := textSHA256("labyrinth-hidden-generation-evidence")
	return []cognition.EvidenceRef{{
		ObservationID: "hidden-generation-evidence",
		Revision: cognition.WorldRevision{
			EpisodeID: "hidden-generation-episode", Number: 1, SHA256: digest,
		},
		SHA256: digest,
	}}
}

func registeredWitnessEvidence(
	schema cognition.ActionSchema,
	observations []cognition.Observation,
) ([]cognition.EvidenceRef, error) {
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		return nil, nil
	}
	if len(observations) == 0 || len(observations) > cognition.MaxEvidenceRefs {
		return nil, fmt.Errorf("%w: required witness evidence is unavailable", ErrGeneration)
	}
	refs := make([]cognition.EvidenceRef, len(observations))
	for index, observation := range observations {
		refs[index] = observation.EvidenceRef()
	}
	return refs, nil
}
