package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FullCognitionRunRequest struct {
	Surface                 Surface
	RatGeneration           RatGeneration
	RuntimeFingerprint      RuntimeFingerprint
	Repetition              int
	Attempt                 model.StepAttemptAuthority
	Pool                    *pgxpool.Pool
	Client                  llm.Client
	HostStore               *labyrinthhost.Store
	RestartAfterCycles      []uint32
	EpisodeSealPath         string
	EvaluationPath          string
	OmnidexCommit           string
	LedgerSchemaVersion     string
	WorkingSetPolicyVersion string
	ProjectionPolicyVersion string
}

type FullCognitionRunResult struct {
	Authority         PairedRunAuthority      `json:"authority"`
	Variant           VariantResult           `json:"variant"`
	Episode           SealedEpisode           `json:"episode"`
	Oracle            OracleManifest          `json:"oracle"`
	Evaluation        Evaluation              `json:"evaluation"`
	Efficiency        EfficiencyMetric        `json:"efficiency"`
	CausalAcquisition CausalAcquisitionReport `json:"causal_acquisition"`
}

func (request FullCognitionRunRequest) Validate() error {
	if _, err := request.Surface.Version(); err != nil {
		return err
	}
	if err := request.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := request.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if request.Repetition <= 0 || request.Repetition > 10_000 || request.Pool == nil ||
		nilRunDependency(request.Client) || request.HostStore == nil {
		return fmt.Errorf("full cognition run requires repetition, PostgreSQL, an LLM client, and an explicit durable host")
	}
	if request.Attempt.JobID <= 0 || request.Attempt.Generation <= 0 || request.Attempt.StepID <= 0 ||
		request.Attempt.Attempt <= 0 || request.Attempt.WorkerID == "" {
		return fmt.Errorf("full cognition run attempt authority is incomplete")
	}
	if err := validateRunOutputPaths(request.EpisodeSealPath, request.EvaluationPath); err != nil {
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
		return fmt.Errorf("full cognition run Omnidex commit is invalid")
	}
	previous := uint32(0)
	for _, cycle := range request.RestartAfterCycles {
		if cycle == 0 || cycle <= previous || cycle >= 1_000_000 {
			return fmt.Errorf("full cognition restart cycles must be positive, unique, and sorted")
		}
		previous = cycle
	}
	return nil
}

func validateRunOutputPaths(episodePath, evaluationPath string) error {
	if episodePath == "" || evaluationPath == "" || episodePath == evaluationPath ||
		filepath.Clean(episodePath) != episodePath || filepath.Clean(evaluationPath) != evaluationPath {
		return fmt.Errorf("full cognition run requires distinct exact output paths")
	}
	for _, path := range []string{episodePath, evaluationPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("full cognition run output already exists or is inaccessible")
		}
		info, err := os.Stat(filepath.Dir(path))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("full cognition run output directory is unavailable")
		}
	}
	episodeDir, _ := os.Stat(filepath.Dir(episodePath))
	evaluationDir, _ := os.Stat(filepath.Dir(evaluationPath))
	if os.SameFile(episodeDir, evaluationDir) {
		return fmt.Errorf("full cognition episode and private evaluation require separate directories")
	}
	return nil
}

func nilRunDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (result FullCognitionRunResult) Validate() error {
	if result.Variant.Variant != VariantFullCognition || result.Authority != result.Variant.Authority ||
		result.Evaluation.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
		result.Evaluation.OracleSHA256 != result.Oracle.OracleSHA256 ||
		result.CausalAcquisition.EpisodeSealSHA256 != result.Episode.SealSHA256 {
		return fmt.Errorf("full cognition result authority is inconsistent")
	}
	if err := ValidateVariantEpisode(result.Variant, result.Episode); err != nil {
		return err
	}
	if err := ValidateEvaluationAuthority(result.Evaluation, result.Episode, result.Oracle); err != nil {
		return err
	}
	metric, err := result.Evaluation.EfficiencyMetric()
	if err != nil || metric != result.Efficiency {
		return fmt.Errorf("full cognition efficiency is inconsistent")
	}
	return result.CausalAcquisition.Validate()
}
