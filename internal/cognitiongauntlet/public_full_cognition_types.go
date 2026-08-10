package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PublicFullCognitionRunRequest struct {
	Attempt                 model.StepAttemptAuthority
	Pool                    *pgxpool.Pool
	Client                  llm.Client
	Environment             cognition.Environment
	Completion              cognitionruntime.CompletionEvaluator
	EpisodeSealPath         string
	OmnidexCommit           string
	LedgerSchemaVersion     string
	WorkingSetPolicyVersion string
	ProjectionPolicyVersion string
	liveStaleProbe          *liveStalePortController
	recoverStalePort        liveStalePort
}

type PublicFullCognitionRunResult struct {
	Authority PublicRunAuthority `json:"authority"`
	Episode   SealedEpisode      `json:"episode"`
}

func (request PublicFullCognitionRunRequest) validate(bundle PublicInferenceBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if bundle.Authority.Variant != VariantFullCognition {
		return fmt.Errorf("public full cognition received variant %q", bundle.Authority.Variant)
	}
	if request.Pool == nil || nilRunDependency(request.Client) || nilRunDependency(request.Environment) ||
		nilRunDependency(request.Completion) {
		return fmt.Errorf("public full cognition run requires PostgreSQL, policy, environment, and completion ports")
	}
	if request.Attempt.JobID <= 0 || request.Attempt.Generation <= 0 || request.Attempt.StepID <= 0 ||
		request.Attempt.Attempt <= 0 || request.Attempt.WorkerID == "" {
		return fmt.Errorf("public full cognition run attempt authority is incomplete")
	}
	if request.EpisodeSealPath == "" || filepath.Clean(request.EpisodeSealPath) != request.EpisodeSealPath {
		return fmt.Errorf("public full cognition episode path must be exact")
	}
	if _, err := os.Stat(request.EpisodeSealPath); !os.IsNotExist(err) {
		return fmt.Errorf("public full cognition episode output already exists or is inaccessible")
	}
	info, err := os.Stat(filepath.Dir(request.EpisodeSealPath))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("public full cognition episode output directory is unavailable")
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
		return fmt.Errorf("public full cognition Omnidex commit is invalid")
	}
	if request.recoverStalePort != "" && request.recoverStalePort.Validate() != nil {
		return fmt.Errorf("public full cognition stale recovery port is not registered")
	}
	return nil
}

func (result PublicFullCognitionRunResult) Validate() error {
	if err := result.Authority.Validate(); err != nil {
		return err
	}
	if err := result.Episode.Validate(); err != nil {
		return err
	}
	if result.Authority.Variant != VariantFullCognition ||
		result.Episode.Manifest.PublicRunAuthoritySHA256 == "" {
		return fmt.Errorf("public full cognition result authority is invalid")
	}
	want, err := result.Authority.SHA256()
	if err != nil || result.Episode.Manifest.PublicRunAuthoritySHA256 != want {
		return fmt.Errorf("public full cognition episode changed its public authority")
	}
	return nil
}
