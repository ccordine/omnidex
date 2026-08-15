package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationJobSpecificationReviewHasIndependentStationOwner(t *testing.T) {
	t.Parallel()

	authority := assemblyline.ApplicationJobSpecificationInput{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "inventory console",
		AcceptedRequirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "filters stock items"},
		},
		FocusedRequirement: assemblyline.Requirement{
			ID: "requirement_001", SourceQuote: "filters stock items",
		},
	}
	retained := assemblyline.ApplicationJobSpecification{
		Objective:          "Implement stock-item filtering in the inventory console.",
		RequiredBehaviors:  []string{"Users can apply and clear a stock-item filter."},
		AcceptanceCriteria: []string{"Applying a filter changes the visible stock items."},
	}
	reviewInput, err := assemblyline.NewApplicationJobSpecificationReviewInput(
		authority, retained, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	reviewJob, err := assemblyline.NewApplicationJobSpecificationReviewJob(reviewInput)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := StationForPortableJob(reviewJob); err != nil {
		t.Fatal(err)
	} else if got != station.CodingWorkloadReview {
		t.Fatalf("review station=%q want=%q", got, station.CodingWorkloadReview)
	}

	specificationJob, err := assemblyline.NewApplicationJobSpecificationJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := StationForPortableJob(specificationJob); err != nil {
		t.Fatal(err)
	} else if got != station.CodingWorkload {
		t.Fatalf("specification station=%q want=%q", got, station.CodingWorkload)
	}
}
