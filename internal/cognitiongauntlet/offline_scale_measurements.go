package cognitiongauntlet

import "fmt"

func deriveOfflineScaleMeasurements(
	registration OfflineScalePreregistration,
	authority ScaleFamilyAuthority,
	runs []OfflineScaleRunReceipt,
) ([]ScaleMeasurement, error) {
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		return nil, err
	}
	measurements := make([]ScaleMeasurement, len(registration.WorldSizes))
	for worldIndex, worldSize := range registration.WorldSizes {
		group := make([]OfflineScaleRunResult, 0, registration.Plan.Repetitions)
		for _, run := range runs {
			if run.Case.WorldSize == worldSize {
				group = append(group, run.Result)
			}
		}
		measurement, err := deriveOfflineScaleMeasurement(
			registration, authoritySHA, worldSize, group,
		)
		if err != nil {
			return nil, fmt.Errorf("derive Scale world %d: %w", worldSize, err)
		}
		measurements[worldIndex] = measurement
	}
	return measurements, nil
}

func deriveOfflineScaleMeasurement(
	registration OfflineScalePreregistration,
	authoritySHA string,
	worldSize int,
	runs []OfflineScaleRunResult,
) (ScaleMeasurement, error) {
	if len(runs) != registration.Plan.Repetitions {
		return ScaleMeasurement{}, fmt.Errorf("Scale world has incomplete repetitions")
	}
	first := runs[0]
	contexts := make([]float64, len(runs))
	decisions := make([]float64, len(runs))
	retrievals := make([]float64, len(runs))
	var successes, causal, clean int
	for index, run := range runs {
		if run.Case.WorldSize != worldSize || run.Case.Repetition != index+1 ||
			run.Scenario != first.Scenario || run.OracleSHA256 != first.OracleSHA256 ||
			run.GeneratorVersion != first.GeneratorVersion ||
			run.RelevantSurfaceBytes != first.RelevantSurfaceBytes {
			return ScaleMeasurement{}, fmt.Errorf("Scale repetitions changed their world authority")
		}
		contexts[index] = float64(run.PeakContextBytes)
		decisions[index] = float64(run.ModelDecisions)
		retrievals[index] = float64(run.RetrievalRounds)
		if run.CompetenceQualifiedSuccess {
			successes++
		}
		if run.CausalAdmissionComplete {
			causal++
		}
		if run.CleanDeskQualified {
			clean++
		}
	}
	count := float64(len(runs))
	measurement := ScaleMeasurement{
		CaseID:           fmt.Sprintf("scale-world-%d", worldSize),
		GeneratorVersion: first.GeneratorVersion, Seed: registration.Plan.Seed,
		Scenario: first.Scenario, OracleSHA256: first.OracleSHA256,
		FamilyAuthoritySHA256: authoritySHA, WorldSize: worldSize,
		RelevantSurfaceBytes: first.RelevantSurfaceBytes,
		MedianContextBytes:   int64(median(contexts)), MedianModelDecisions: median(decisions),
		SuccessRate: float64(successes) / count, CausalAdmissionRate: float64(causal) / count,
		CleanDeskAdmissionRate: float64(clean) / count,
		MedianRetrievalRounds:  median(retrievals),
	}
	return measurement, measurement.validate(authoritySHA, registration.Fixed.Budget)
}
