package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func validateDirectArtifactAbsenceTruth(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	absenceQuotes []string,
) error {
	absent := make(map[string]struct{}, len(absenceQuotes))
	for _, quote := range absenceQuotes {
		absent[quote] = struct{}{}
	}
	for _, directive := range directives {
		if directive.Disposition != assemblyline.ArtifactForbid &&
			directive.Disposition != assemblyline.ArtifactAbsenceCandidate {
			continue
		}
		quote, err := exactArtifactAbsenceRequirementQuote(
			directive.Token, featureQuotes,
		)
		if err != nil {
			return err
		}
		if _, accepted := absent[quote]; !accepted {
			return fmt.Errorf(
				"artifact absence stations disagree about desired truth for exact requirement %q",
				quote,
			)
		}
	}
	return nil
}
