package cognitiongauntlet

import "fmt"

type FullCognitionScaleCaseRequest struct {
	WorldSize int
	Runs      []FullCognitionRunRequest
}

type FullCognitionScaleRequest struct {
	WorldSizes []int
	Cases      []FullCognitionScaleCaseRequest
}

type FullCognitionScaleCaseResult struct {
	WorldSize int                      `json:"world_size"`
	Runs      []FullCognitionRunResult `json:"runs"`
}

type FullCognitionScaleResult struct {
	Authority ScaleFamilyAuthority           `json:"authority"`
	Cases     []FullCognitionScaleCaseResult `json:"cases"`
	Report    ScaleRailReport                `json:"report"`
}

func (result FullCognitionScaleResult) Validate() error {
	if err := result.Authority.Validate(); err != nil {
		return err
	}
	if len(result.Cases) < 2 || len(result.Report.Measurements) != len(result.Cases) ||
		result.Report.Authority != result.Authority {
		return fmt.Errorf("full cognition scale result authority is inconsistent")
	}
	for index, item := range result.Cases {
		if item.WorldSize != result.Report.Measurements[index].WorldSize || len(item.Runs) == 0 {
			return fmt.Errorf("full cognition scale case %d is inconsistent", index+1)
		}
		for runIndex, run := range item.Runs {
			if err := run.Validate(); err != nil {
				return fmt.Errorf("full cognition scale case %d run %d: %w", index+1, runIndex+1, err)
			}
		}
	}
	return nil
}
