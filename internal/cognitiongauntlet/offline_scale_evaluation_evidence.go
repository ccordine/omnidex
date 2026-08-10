package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

const OfflineScaleEvaluationEvidenceSchemaV1 = "omnidex.offline-scale-evaluation-evidence.v1"

type OfflineScaleEvaluationEvidence struct {
	Schema                   string                          `json:"schema"`
	Case                     OfflineScaleCase                `json:"case"`
	Family                   labyrinth.ScaleFamilyDescriptor `json:"family"`
	FamilySHA256             string                          `json:"family_sha256"`
	Scenario                 cognition.ScenarioRef           `json:"scenario"`
	OracleSHA256             string                          `json:"oracle_sha256"`
	RelevantSurfaceBytes     int64                           `json:"relevant_surface_bytes"`
	SolutionDepth            int                             `json:"solution_depth"`
	RelevantEvidenceCount    int                             `json:"relevant_evidence_count"`
	SemanticDecisionCount    int                             `json:"semantic_decision_count"`
	EpisodeSealSHA256        string                          `json:"episode_seal_sha256"`
	EvaluationArtifactSHA256 string                          `json:"evaluation_artifact_sha256"`
}

func (evidence OfflineScaleEvaluationEvidence) Validate() error {
	if evidence.Schema != OfflineScaleEvaluationEvidenceSchemaV1 ||
		evidence.Family.Validate() != nil || evidence.Scenario.Validate() != nil ||
		!validDigest(evidence.FamilySHA256) || !validDigest(evidence.OracleSHA256) ||
		!validDigest(evidence.EpisodeSealSHA256) ||
		!validDigest(evidence.EvaluationArtifactSHA256) ||
		evidence.RelevantSurfaceBytes <= 0 || evidence.SolutionDepth <= 0 ||
		evidence.RelevantEvidenceCount <= 0 || evidence.SemanticDecisionCount <= 0 {
		return fmt.Errorf("offline Scale evaluation evidence is invalid")
	}
	wantFamilySHA, err := digestJSON(evidence.Family)
	if err != nil || wantFamilySHA != evidence.FamilySHA256 {
		return fmt.Errorf("offline Scale evaluation family digest changed")
	}
	matched := false
	for _, item := range evidence.Family.Cases {
		if item.WorldSize == evidence.Case.WorldSize {
			matched = item.Scenario == evidence.Scenario
			break
		}
	}
	if !matched {
		return fmt.Errorf("offline Scale evaluation evidence changed its world")
	}
	return nil
}

func SealOfflineScaleEvaluationEvidence(
	path string,
	evidence OfflineScaleEvaluationEvidence,
) error {
	if err := evidence.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, evidence, "offline Scale evaluation evidence")
}

func LoadOfflineScaleEvaluationEvidence(
	path string,
) (OfflineScaleEvaluationEvidence, string, error) {
	var evidence OfflineScaleEvaluationEvidence
	if err := loadStrictJSONFile(path, &evidence, "offline Scale evaluation evidence"); err != nil {
		return OfflineScaleEvaluationEvidence{}, "", err
	}
	if err := evidence.Validate(); err != nil {
		return OfflineScaleEvaluationEvidence{}, "", err
	}
	digest, err := hashExactFile(path, 64*1024+1)
	return evidence, digest, err
}
