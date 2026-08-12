package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
)

type PublicAblationRunRequest struct {
	Actor                   cognition.AttemptRef
	Client                  llm.Client
	Environment             cognition.Environment
	Completion              cognitionruntime.CompletionEvaluator
	ContaminatedEvidence    *ContaminatedEvidencePacket
	EpisodeSealPath         string
	EvidenceSealPath        string
	OmnidexCommit           string
	LedgerSchemaVersion     string
	WorkingSetPolicyVersion string
	ProjectionPolicyVersion string
}

type PublicAblationRunResult struct {
	Authority PublicRunAuthority        `json:"authority"`
	Episode   SealedEpisode             `json:"episode"`
	Evidence  AblationEvidenceAuthority `json:"evidence"`
}

func (request PublicAblationRunRequest) validate(bundle PublicInferenceBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	variant := bundle.Authority.Variant
	if !executableAblation(variant) || nilRunDependency(request.Client) ||
		nilRunDependency(request.Environment) || nilRunDependency(request.Completion) {
		return fmt.Errorf("public cognition ablation dependencies are incomplete")
	}
	if err := request.Actor.Validate(); err != nil {
		return err
	}
	if variant == VariantOracleEvidence {
		if request.ContaminatedEvidence == nil || request.ContaminatedEvidence.Validate() != nil {
			return fmt.Errorf("oracle-evidence ceiling requires an explicit contaminated grant")
		}
	} else if request.ContaminatedEvidence != nil {
		return fmt.Errorf("non-oracle ablation received private evaluator authority")
	}
	if request.EpisodeSealPath == "" || filepath.Clean(request.EpisodeSealPath) != request.EpisodeSealPath {
		return fmt.Errorf("public cognition ablation episode path is inexact")
	}
	if _, err := os.Stat(request.EpisodeSealPath); !os.IsNotExist(err) {
		return fmt.Errorf("public cognition ablation episode output already exists or is inaccessible")
	}
	if info, err := os.Stat(filepath.Dir(request.EpisodeSealPath)); err != nil || !info.IsDir() {
		return fmt.Errorf("public cognition ablation episode directory is unavailable")
	}
	if err := validateAblationEvidenceOutputPath(
		request.EvidenceSealPath, request.EpisodeSealPath,
	); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        request.LedgerSchemaVersion,
		"Working Set policy version":        request.WorkingSetPolicyVersion,
		"Context Projection policy version": request.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if request.OmnidexCommit != "" && !validCommitIdentity(request.OmnidexCommit) {
		return fmt.Errorf("public cognition ablation commit is invalid")
	}
	return nil
}

func (result PublicAblationRunResult) Validate() error {
	if err := result.Authority.Validate(); err != nil {
		return err
	}
	if !executableAblation(result.Authority.Variant) {
		return fmt.Errorf("public cognition ablation variant is invalid")
	}
	if err := result.Episode.Validate(); err != nil {
		return err
	}
	if err := result.Evidence.Validate(); err != nil {
		return err
	}
	boundEvidence, err := ablationEvidenceAuthorityFromEpisode(result.Episode)
	if err != nil || boundEvidence != result.Evidence {
		return fmt.Errorf("public cognition ablation changed its evidence authority")
	}
	want, err := result.Authority.SHA256()
	if err != nil || result.Episode.Manifest.PublicRunAuthoritySHA256 != want ||
		result.Episode.Manifest.Variant != result.Authority.Variant ||
		result.Episode.Manifest.Scenario != result.Authority.Scenario {
		return fmt.Errorf("public cognition ablation changed its sealed authority")
	}
	return nil
}
