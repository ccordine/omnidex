package cognitiongauntlet

import (
	"fmt"
)

const ResumeBaselineArtifactSchemaV1 = "omnidex.resume-uninterrupted-baseline.v1"

type ResumeBaselineCheckpoint struct {
	DecisionBoundary uint32                    `json:"decision_boundary"`
	PreCall          SemanticPreCallCheckpoint `json:"pre_call"`
}

func (checkpoint ResumeBaselineCheckpoint) Validate() error {
	if checkpoint.PreCall.Validate() != nil {
		return fmt.Errorf("Resume baseline checkpoint authority is invalid")
	}
	return nil
}

type ResumeBaselineArtifact struct {
	Schema                   string                     `json:"schema"`
	PublicRunAuthoritySHA256 string                     `json:"public_run_authority_sha256"`
	EpisodeSealSHA256        string                     `json:"episode_seal_sha256"`
	Semantics                ResumeEpisodeSemantics     `json:"episode_semantics"`
	Checkpoints              []ResumeBaselineCheckpoint `json:"checkpoints"`
}

func NewResumeBaselineArtifact(
	publicSHA256 string,
	episode SealedEpisode,
	checkpoints []ResumeBaselineCheckpoint,
) (ResumeBaselineArtifact, error) {
	semantics, err := DeriveResumeEpisodeSemantics(episode)
	if err != nil {
		return ResumeBaselineArtifact{}, err
	}
	artifact := ResumeBaselineArtifact{
		Schema:                   ResumeBaselineArtifactSchemaV1,
		PublicRunAuthoritySHA256: publicSHA256,
		EpisodeSealSHA256:        episode.SealSHA256, Semantics: semantics,
		Checkpoints: cloneResumeBaselineCheckpoints(checkpoints),
	}
	return artifact, artifact.Validate()
}

func (artifact ResumeBaselineArtifact) Validate() error {
	if artifact.Schema != ResumeBaselineArtifactSchemaV1 ||
		!validDigest(artifact.PublicRunAuthoritySHA256) ||
		!validDigest(artifact.EpisodeSealSHA256) || artifact.Semantics.Validate() != nil ||
		artifact.Checkpoints == nil || len(artifact.Checkpoints) == 0 {
		return fmt.Errorf("Resume uninterrupted baseline authority is invalid")
	}
	for index, checkpoint := range artifact.Checkpoints {
		if checkpoint.DecisionBoundary != uint32(index) || checkpoint.Validate() != nil {
			return fmt.Errorf("Resume baseline checkpoint %d is not exact", index+1)
		}
		if index > 0 {
			before := artifact.Checkpoints[index-1].PreCall.Bound.Attempt
			after := checkpoint.PreCall.Bound.Attempt
			if before != after {
				return fmt.Errorf("uninterrupted Resume baseline changed attempt authority")
			}
		}
	}
	if len(artifact.Checkpoints) < artifact.Semantics.ModelDecisions {
		return fmt.Errorf("Resume baseline omitted a model decision boundary")
	}
	return nil
}

func (artifact ResumeBaselineArtifact) checkpoint(
	decisionBoundary uint32,
) (ResumeBaselineCheckpoint, error) {
	if err := artifact.Validate(); err != nil {
		return ResumeBaselineCheckpoint{}, err
	}
	if int(decisionBoundary) >= len(artifact.Checkpoints) {
		return ResumeBaselineCheckpoint{}, fmt.Errorf("Resume baseline lacks decision boundary %d", decisionBoundary)
	}
	return artifact.Checkpoints[decisionBoundary], nil
}

func cloneResumeBaselineCheckpoints(
	values []ResumeBaselineCheckpoint,
) []ResumeBaselineCheckpoint {
	if values == nil {
		return nil
	}
	return append([]ResumeBaselineCheckpoint{}, values...)
}

func SealResumeBaselineArtifact(path string, artifact ResumeBaselineArtifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, artifact, "Resume uninterrupted baseline")
}

func LoadResumeBaselineArtifact(path string) (ResumeBaselineArtifact, error) {
	var artifact ResumeBaselineArtifact
	if err := loadStrictJSONFile(path, &artifact, "Resume uninterrupted baseline"); err != nil {
		return ResumeBaselineArtifact{}, err
	}
	if err := artifact.Validate(); err != nil {
		return ResumeBaselineArtifact{}, err
	}
	return artifact, nil
}

func LoadResumeBaselineArtifactWithSHA(
	path string,
) (ResumeBaselineArtifact, string, error) {
	artifact, err := LoadResumeBaselineArtifact(path)
	if err != nil {
		return ResumeBaselineArtifact{}, "", err
	}
	digest, err := hashExactFile(path, maxOfflineMatrixArtifactBytes)
	if err != nil {
		return ResumeBaselineArtifact{}, "", err
	}
	return artifact, digest, nil
}
