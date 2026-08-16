package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type applicationJobSpecificationReviewTarget struct {
	field      assemblyline.ApplicationJobSpecificationField
	evidenceID string
}

func applicationJobSpecificationReviewTargets(
	retained assemblyline.ApplicationJobSpecification,
) []applicationJobSpecificationReviewTarget {
	targets := []applicationJobSpecificationReviewTarget{{
		field: assemblyline.ApplicationJobSpecificationObjectiveField, evidenceID: "E1",
	}}
	for index := range retained.RequiredBehaviors {
		targets = append(targets, applicationJobSpecificationReviewTarget{
			field:      assemblyline.ApplicationJobSpecificationRequiredBehaviorsField,
			evidenceID: fmt.Sprintf("E%d", index+1),
		})
	}
	for index := range retained.AcceptanceCriteria {
		targets = append(targets, applicationJobSpecificationReviewTarget{
			field:      assemblyline.ApplicationJobSpecificationAcceptanceCriteriaField,
			evidenceID: fmt.Sprintf("E%d", index+1),
		})
	}
	return targets
}
