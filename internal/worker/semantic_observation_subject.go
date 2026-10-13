package worker

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// The registered renderer has already validated the job's source grammar.
// Project its compared values verbatim, without new interpretation or authority.
func semanticObservationSubjects(job assemblyline.PortableJob) (string, string, error) {
	switch job.Kind {
	case assemblyline.WorkCapabilityRelation:
		var input assemblyline.CapabilityRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.LeftNeed, input.RightNeed, nil
	case assemblyline.WorkRoleplayCanonFactCandidateRelation:
		var input assemblyline.RoleplayCanonFactCandidateRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, input.AcceptedFact, nil
	case assemblyline.WorkDatabaseQueryPurposeRelation:
		var input assemblyline.DatabaseQueryPurposeRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, input.AcceptedPurpose, nil
	default:
		return semanticObservationCandidateSubjects(job.Payload)
	}
}

func semanticObservationCandidateSubjects(payload json.RawMessage) (string, string, error) {
	var focus struct {
		Candidate json.RawMessage `json:"candidate"`
		Against   string          `json:"accepted_requirement"`
	}
	if err := json.Unmarshal(payload, &focus); err != nil {
		return "", "", err
	}
	subject := ""
	if len(focus.Candidate) > 0 && focus.Candidate[0] == '"' {
		if err := json.Unmarshal(focus.Candidate, &subject); err != nil {
			return "", "", err
		}
	} else if len(focus.Candidate) > 0 && focus.Candidate[0] == '{' {
		var source struct {
			Namespace   string `json:"namespace"`
			CandidateID string `json:"candidate_id"`
		}
		if err := json.Unmarshal(focus.Candidate, &source); err != nil {
			return "", "", err
		}
		subject = source.CandidateID
		if source.Namespace != "" && subject != "" {
			subject = source.Namespace + "/" + subject
		}
	}
	return subject, focus.Against, nil
}
