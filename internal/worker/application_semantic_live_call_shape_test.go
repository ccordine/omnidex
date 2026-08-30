package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func assertLiveCodingQualificationCalls(
	t *testing.T,
	calls []liveCodingQualificationCall,
	frozen assemblyline.FrozenApplicationWorkload,
	userRequest string,
) {
	t.Helper()
	counts := map[assemblyline.WorkKind]int{}
	bySubject := map[string]map[assemblyline.WorkKind]int{}
	bySubjectDimension := map[string]map[assemblyline.ApplicationRequirementCandidateContentDimension]int{}
	for _, call := range calls {
		counts[call.kind]++
		for _, digest := range []string{
			call.jobSHA256,
			call.promptSHA256,
			call.requestSHA256,
			call.responseSHA256,
			call.candidateSHA256,
		} {
			if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
				t.Fatal("live qualification call lacks an exact digest")
			}
		}
		if call.promptBytes < 1 || call.promptTokens < 1 || call.outputTokens < 1 ||
			call.providerDuration <= 0 || call.wallDuration <= 0 {
			t.Fatal("live qualification call lacks bounded native metrics")
		}
		if call.subject != "" {
			if bySubject[call.subject] == nil {
				bySubject[call.subject] = map[assemblyline.WorkKind]int{}
			}
			bySubject[call.subject][call.kind]++
			if call.contentDimension != "" {
				if bySubjectDimension[call.subject] == nil {
					bySubjectDimension[call.subject] = map[assemblyline.ApplicationRequirementCandidateContentDimension]int{}
				}
				bySubjectDimension[call.subject][call.contentDimension]++
			}
		}
	}
	if counts[assemblyline.WorkApplicationProductContext] != 1 ||
		counts[assemblyline.WorkApplicationRequirementInventory] != 1 {
		t.Fatalf("live qualification did not use one product leaf and one candidate inventory: %v", counts)
	}
	for _, task := range frozen.Tasks {
		if bySubject[task.RequirementQuote][assemblyline.WorkApplicationRequirementCandidateKind] != 2 ||
			bySubjectDimension[task.RequirementQuote][assemblyline.ApplicationRequirementCandidateRuntimeContentDimension] != 1 ||
			bySubjectDimension[task.RequirementQuote][assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension] != 1 {
			t.Fatalf(
				"accepted leaf %q did not receive one call per content dimension: calls=%+v dimensions=%+v",
				task.RequirementQuote,
				bySubject[task.RequirementQuote],
				bySubjectDimension[task.RequirementQuote],
			)
		}
		for _, kind := range []assemblyline.WorkKind{
			assemblyline.WorkApplicationRequirementCandidateCardinality,
			assemblyline.WorkApplicationRequirementCandidateResultRelation,
		} {
			if bySubject[task.RequirementQuote][kind] != 1 {
				t.Fatalf(
					"accepted leaf %q was not sieved exactly once for %q: %+v",
					task.RequirementQuote,
					kind,
					bySubject[task.RequirementQuote],
				)
			}
		}
		expectedAuthorizationCalls := 1
		if task.RequirementQuote == userRequest {
			expectedAuthorizationCalls = 0
		}
		if bySubject[task.RequirementQuote][assemblyline.WorkApplicationRequirementCandidateAuthorization] != expectedAuthorizationCalls {
			t.Fatalf(
				"accepted leaf %q semantic authorization calls=%d want=%d: %+v",
				task.RequirementQuote,
				bySubject[task.RequirementQuote][assemblyline.WorkApplicationRequirementCandidateAuthorization],
				expectedAuthorizationCalls,
				bySubject[task.RequirementQuote],
			)
		}
	}
}

func liveCodingQualificationCallSubject(
	job assemblyline.PortableJob,
) (string, assemblyline.ApplicationRequirementCandidateContentDimension, error) {
	switch job.Kind {
	case assemblyline.WorkApplicationRequirementCandidateKind:
		var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, input.Dimension, nil
	case assemblyline.WorkApplicationRequirementCandidateCardinality:
		var input assemblyline.ApplicationRequirementCandidateCardinalityInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidateAuthorization:
		var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
		var input assemblyline.ApplicationRequirementCandidateOutcomeRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelation:
		var input assemblyline.ApplicationRequirementCandidateResultRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
		var input assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.CandidateAuthority.Candidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
		var input assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.CurrentCandidate, "", nil
	case assemblyline.WorkApplicationRequirementCandidatePartition:
		var input assemblyline.ApplicationRequirementCandidatePartitionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", "", err
		}
		return input.Candidate, "", nil
	case assemblyline.WorkApplicationProductContext,
		assemblyline.WorkApplicationRequirementInventory:
		return "", "", nil
	default:
		return "", "", nil
	}
}
