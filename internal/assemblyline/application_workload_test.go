package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationWorkloadValidatesTaskValuesWithoutHashReceipts(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		"The software lists the supplied labels in order.",
		"The software calculates the total of the supplied numbers.",
	} {
		t.Run(statement, func(t *testing.T) {
			specification := ApplicationSpecification{
				Surface: ApplicationSurfaceBrowser, ProductQuote: "a small application",
				Requirements: []Requirement{{ID: "requirement_001", SourceQuote: statement}},
			}
			workload, err := FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatal(err)
			}
			context, err := ProjectApplicationTaskContext(workload, "task_001")
			if err != nil {
				t.Fatal(err)
			}
			if err := context.ValidateFor(workload); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal([]any{workload, context})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "sha256") {
				t.Fatalf("workload still carries a hash receipt: %s", encoded)
			}
			context.Task.RequirementQuote = "a different outcome"
			if err := context.ValidateFor(workload); err == nil {
				t.Fatal("accepted a different requirement under the same task ID")
			}
			workload.Tasks[0].RequirementQuote = "another requirement"
			if err := ValidateFrozenApplicationWorkloadFor(specification, workload); err == nil {
				t.Fatal("accepted a workload that differs from the accepted requirements")
			}
		})
	}
}
