package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"
)

type VerifiedOfflineResumeReceipt struct {
	receipt OfflineResumeReceipt
}

func (verified VerifiedOfflineResumeReceipt) PromotionEligible() bool {
	return verified.receipt.PromotionEligible
}

func (verified VerifiedOfflineResumeReceipt) GateEvidenceQualified() bool {
	return verified.receipt.GateEvidenceQualified
}

func (verified VerifiedOfflineResumeReceipt) RunCount() int {
	return len(verified.receipt.Runs)
}

func (verified VerifiedOfflineResumeReceipt) Receipt() OfflineResumeReceipt {
	copy := verified.receipt
	copy.Runs = append([]OfflineResumeRunReceipt{}, verified.receipt.Runs...)
	for index := range copy.Runs {
		copy.Runs[index].Schedule.DecisionBoundaries = append(
			[]uint32{}, verified.receipt.Runs[index].Schedule.DecisionBoundaries...,
		)
		copy.Runs[index].Interruptions = append(
			[]OfflineResumeInterruptionReceipt{}, verified.receipt.Runs[index].Interruptions...,
		)
		if verified.receipt.Runs[index].LiveStaleProbe != nil {
			probe := *verified.receipt.Runs[index].LiveStaleProbe
			probe.Probes = append(
				[]LiveStalePortProof{}, verified.receipt.Runs[index].LiveStaleProbe.Probes...,
			)
			copy.Runs[index].LiveStaleProbe = &probe
		}
	}
	copy.Gate.Reasons = append([]string{}, verified.receipt.Gate.Reasons...)
	return copy
}

func SealOfflineResumeReceipt(
	path string,
	receipt OfflineResumeReceipt,
	registration OfflineResumePreregistration,
	baseline ResumeBaselineArtifact,
) error {
	if err := receipt.Validate(registration, baseline); err != nil {
		return err
	}
	return sealScenarioArtifact(path, receipt, "offline Resume receipt")
}

func LoadOfflineResumeReceipt(
	path string,
	registration OfflineResumePreregistration,
	baseline ResumeBaselineArtifact,
) (OfflineResumeReceipt, error) {
	var receipt OfflineResumeReceipt
	if err := loadStrictJSONFile(path, &receipt, "offline Resume receipt"); err != nil {
		return OfflineResumeReceipt{}, err
	}
	if err := receipt.Validate(registration, baseline); err != nil {
		return OfflineResumeReceipt{}, err
	}
	return receipt, nil
}

func LoadVerifiedOfflineResumeReceipt(
	config OfflineResumeConfig,
) (VerifiedOfflineResumeReceipt, error) {
	if err := config.Validate(); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	registration, err := LoadOfflineResumePreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	baseline, _, err := loadConfiguredResumeBaseline(config, registration)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	receipt, err := LoadOfflineResumeReceipt(config.Paths().Receipt, registration, baseline)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	if err := VerifyOfflineResumeReceipt(config, receipt); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	return VerifiedOfflineResumeReceipt{receipt: receipt}, nil
}

func loadConfiguredResumeBaseline(
	config OfflineResumeConfig,
	registration OfflineResumePreregistration,
) (ResumeBaselineArtifact, string, error) {
	if len(registration.Schedules) == 0 ||
		registration.Schedules[0].Kind != ResumeUninterrupted {
		return ResumeBaselineArtifact{}, "", fmt.Errorf("Resume preregistration lacks its baseline schedule")
	}
	run, err := config.derivedRunConfig(registration, registration.Schedules[0])
	if err != nil {
		return ResumeBaselineArtifact{}, "", err
	}
	return LoadResumeBaselineArtifactWithSHA(
		filepath.Join(run.PrivateOutputDirectory, "resume-baseline.json"),
	)
}

func VerifyOfflineResumeReceipt(
	config OfflineResumeConfig,
	receipt OfflineResumeReceipt,
) error {
	if err := config.Validate(); err != nil {
		return err
	}
	registration, err := LoadOfflineResumePreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	baseline, baselineSHA, err := loadConfiguredResumeBaseline(config, registration)
	if err != nil {
		return err
	}
	if receipt.BaselineArtifactSHA256 != baselineSHA {
		return fmt.Errorf("Resume receipt changed its uninterrupted baseline artifact")
	}
	if err := receipt.Validate(registration, baseline); err != nil {
		return err
	}
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for index, schedule := range registration.Schedules {
		rebuilt, err := verifyOfflineResumeRun(config, registration, schedule, baseline)
		if err != nil {
			return fmt.Errorf("verify Resume run %d: %w", index+1, err)
		}
		if !reflect.DeepEqual(receipt.Runs[index], rebuilt) {
			return fmt.Errorf("Resume run %d differs from its exact artifacts", index+1)
		}
		resumeChronologyFromRun(rebuilt, &lastInference, &firstEvaluator, &completedAt)
	}
	if lastInference != receipt.LastInferenceExitedAt ||
		firstEvaluator != receipt.FirstEvaluatorStartedAt || completedAt != receipt.CompletedAt {
		return fmt.Errorf("Resume receipt changed global inference/evaluator chronology")
	}
	return nil
}
