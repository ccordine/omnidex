package cognitiongauntlet

import (
	"fmt"
	"sort"
)

func measureFullCognitionScaleCase(
	authority ScaleFamilyAuthority,
	fixture MicrogauntletCase,
	runs []FullCognitionRunResult,
) (ScaleMeasurement, error) {
	if len(runs) == 0 {
		return ScaleMeasurement{}, fmt.Errorf("full cognition scale case has no sealed runs")
	}
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		return ScaleMeasurement{}, err
	}
	public, err := fixture.PublicManifest(SurfaceSymbolic)
	if err != nil {
		return ScaleMeasurement{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return ScaleMeasurement{}, err
	}
	relevantBytes, err := labyrinthRelevantSurfaceBytes(fixture)
	if err != nil {
		return ScaleMeasurement{}, err
	}
	contexts := make([]float64, len(runs))
	decisions := make([]float64, len(runs))
	retrievals := make([]float64, len(runs))
	var successes, causalAdmissions, cleanDeskAdmissions int
	for index, run := range runs {
		if err := run.Validate(); err != nil {
			return ScaleMeasurement{}, err
		}
		if run.Authority.Scenario != public.Scenario || run.Authority.Budget != authority.Budget ||
			run.Authority.RatGeneration != authority.RatGeneration || run.Authority.Runtime != authority.Runtime {
			return ScaleMeasurement{}, fmt.Errorf("full cognition scale run changed its family authority")
		}
		contexts[index] = float64(run.Episode.Manifest.Resources.PeakContextBytes)
		decisions[index] = float64(run.Episode.Manifest.Resources.ModelDecisions)
		retrievals[index] = float64(
			run.Episode.Manifest.Resources.SearchOperations + run.Episode.Manifest.Resources.ReadOperations,
		)
		if run.Evaluation.GoalSuccess {
			successes++
			if run.CausalAcquisition.AcquiredEvidence == run.CausalAcquisition.RequiredEvidence {
				causalAdmissions++
			}
			if run.Evaluation.CleanDesk != nil && run.Evaluation.CleanDesk.ConcentrationQualified &&
				run.Evaluation.CleanDesk.MissingCriticalBytes == 0 {
				cleanDeskAdmissions++
			}
		}
	}
	count := float64(len(runs))
	measurement := ScaleMeasurement{
		CaseID: fixture.spec.CaseID, GeneratorVersion: fixture.spec.Generator.GeneratorVersion,
		Seed: fixture.spec.Generator.Seed, Scenario: public.Scenario, OracleSHA256: oracle.OracleSHA256,
		FamilyAuthoritySHA256: authoritySHA, WorldSize: public.Difficulty.WorldSize,
		RelevantSurfaceBytes: relevantBytes, MedianContextBytes: int64(median(contexts)),
		MedianModelDecisions: median(decisions), SuccessRate: float64(successes) / count,
		CausalAdmissionRate:    float64(causalAdmissions) / count,
		CleanDeskAdmissionRate: float64(cleanDeskAdmissions) / count,
		MedianRetrievalRounds:  median(retrievals),
	}
	return measurement, measurement.validate(authoritySHA, authority.Budget)
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}
