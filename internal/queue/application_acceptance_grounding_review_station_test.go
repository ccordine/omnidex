package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationAcceptanceGroundingReviewHasDedicatedReviewStationOwner(t *testing.T) {
	t.Parallel()

	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		assemblyline.ApplicationTaskContext{
			WorkloadSHA256: strings.Repeat("a", 64),
			Task: assemblyline.ApplicationTaskContextTask{
				TaskID: "task_001",
				AcceptanceCriteria: []string{
					"The observable result is true.",
				},
			},
		},
		`function VerifyResult(): void { expect(true).toBe(true); }`,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingWorkloadReview {
		t.Fatalf("station=%q want=%q", got, station.CodingWorkloadReview)
	}
}
