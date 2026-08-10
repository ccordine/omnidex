package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognition"
)

const OracleSuiteReportSchemaV1 = "omnidex.cognition-oracle-suite-report.v1"

type OracleSuiteRequest struct {
	Surface                 Surface              `json:"surface"`
	RatGeneration           RatGeneration        `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint   `json:"runtime_fingerprint"`
	Repetition              int                  `json:"repetition"`
	Actor                   cognition.AttemptRef `json:"actor"`
	EpisodeRoot             string               `json:"episode_root"`
	EvaluationRoot          string               `json:"evaluation_root"`
	OmnidexCommit           string               `json:"omnidex_commit,omitempty"`
	LedgerSchemaVersion     string               `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string               `json:"working_set_policy_version"`
	ProjectionPolicyVersion string               `json:"projection_policy_version"`
}

type OracleSuiteReport struct {
	Schema         string                 `json:"schema"`
	FixtureVersion string                 `json:"fixture_version"`
	SurfaceVersion string                 `json:"surface_version"`
	Results        []OracleBaselineResult `json:"results"`
}

func RunInitialOracleGauntlets(
	ctx context.Context,
	request OracleSuiteRequest,
) (OracleSuiteReport, error) {
	report := OracleSuiteReport{
		Schema:         OracleSuiteReportSchemaV1,
		FixtureVersion: InitialMicrogauntletFixtureVersionV1,
		Results:        make([]OracleBaselineResult, 0, 5),
	}
	if ctx == nil {
		return report, fmt.Errorf("oracle suite context is nil")
	}
	if err := request.Validate(); err != nil {
		return report, err
	}
	var err error
	report.SurfaceVersion, err = request.Surface.Version()
	if err != nil {
		return report, err
	}
	fixtures, err := GenerateInitialMicrogauntletsV1()
	if err != nil {
		return report, err
	}
	for _, fixture := range fixtures {
		episodeDirectory := filepath.Join(request.EpisodeRoot, fixture.spec.CaseID)
		evaluationDirectory := filepath.Join(request.EvaluationRoot, fixture.spec.CaseID)
		if err := os.Mkdir(episodeDirectory, 0o700); err != nil {
			return report, fmt.Errorf("create oracle suite episode directory: %w", err)
		}
		if err := os.Mkdir(evaluationDirectory, 0o700); err != nil {
			return report, fmt.Errorf("create oracle suite evaluation directory: %w", err)
		}
		runRequest := request.episodeRequest(
			filepath.Join(episodeDirectory, "episode.json"),
			filepath.Join(evaluationDirectory, "evaluation.json"),
		)
		result, err := RunOracleBaseline(ctx, fixture, runRequest)
		if err != nil {
			return report, fmt.Errorf("run oracle microgauntlet %q: %w", fixture.spec.CaseID, err)
		}
		report.Results = append(report.Results, result)
	}
	if err := report.Validate(); err != nil {
		return report, err
	}
	return report, nil
}

func (request OracleSuiteRequest) Validate() error {
	if request.EpisodeRoot == "" || request.EvaluationRoot == "" ||
		filepath.Clean(request.EpisodeRoot) != request.EpisodeRoot ||
		filepath.Clean(request.EvaluationRoot) != request.EvaluationRoot ||
		request.EpisodeRoot == string(filepath.Separator) ||
		request.EvaluationRoot == string(filepath.Separator) {
		return fmt.Errorf("oracle suite roots must be exact")
	}
	probe := request.episodeRequest(
		filepath.Join(request.EpisodeRoot, "probe.json"),
		filepath.Join(request.EvaluationRoot, "probe.json"),
	)
	if err := probe.Validate(); err != nil {
		return err
	}
	episodeInfo, episodeErr := os.Stat(request.EpisodeRoot)
	evaluationInfo, evaluationErr := os.Stat(request.EvaluationRoot)
	if episodeErr != nil || evaluationErr != nil ||
		!episodeInfo.IsDir() || !evaluationInfo.IsDir() || os.SameFile(episodeInfo, evaluationInfo) {
		return fmt.Errorf("oracle suite requires distinct existing episode and evaluation roots")
	}
	return nil
}

func (request OracleSuiteRequest) episodeRequest(episodePath, evaluationPath string) OracleRunRequest {
	return OracleRunRequest{
		Surface: request.Surface, RatGeneration: request.RatGeneration,
		RuntimeFingerprint: request.RuntimeFingerprint, Repetition: request.Repetition,
		Actor: request.Actor, EpisodeSealPath: episodePath, EvaluationPath: evaluationPath,
		OmnidexCommit: request.OmnidexCommit, LedgerSchemaVersion: request.LedgerSchemaVersion,
		WorkingSetPolicyVersion: request.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: request.ProjectionPolicyVersion,
	}
}

func (report OracleSuiteReport) Validate() error {
	if report.Schema != OracleSuiteReportSchemaV1 ||
		report.FixtureVersion != InitialMicrogauntletFixtureVersionV1 {
		return fmt.Errorf("oracle suite report schema or fixture version is invalid")
	}
	if err := requireExact(report.SurfaceVersion, "oracle suite surface version", 256); err != nil {
		return err
	}
	if len(report.Results) != 5 {
		return fmt.Errorf("oracle suite report requires exactly five results")
	}
	want := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	for index, result := range report.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("oracle suite result %d: %w", index, err)
		}
		if result.Authority.Suite != want[index] ||
			result.Authority.SurfaceVersion != report.SurfaceVersion {
			return fmt.Errorf("oracle suite result %d changed suite order or surface", index)
		}
	}
	return nil
}
