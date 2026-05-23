package coding

import (
	"context"
	"fmt"
)

type deterministicSummarizer struct{}

func (deterministicSummarizer) Summarize(_ context.Context, result Result) (string, error) {
	return fmt.Sprintf("coding workflow complete: %d planner task(s)", len(result.Plan.Tasks)), nil
}
