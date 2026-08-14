package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

const objectiveRepositoryUsefulAdvice = "Identify implications, risks, edge cases, ambiguities, hidden constraints, verification ideas, or questions relevant to reviewing this candidate answer."

type objectiveAdvisoryRunner interface {
	Configuration() objectiveadvisory.Config
	Run(
		context.Context,
		objectiveadvisory.ProjectionInput,
		objectiveadvisory.SemanticGap,
	) (objectiveadvisory.Report, error)
}

type objectiveRepositoryGroundedClosureOptions struct {
	ObjectiveID string
	Generation  int64
	Advisory    objectiveAdvisoryRunner
}

func runObjectiveRepositoryAdvisory(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
	review assemblyline.RepositoryGroundedReviewInput,
	options objectiveRepositoryGroundedClosureOptions,
) (objectiveadvisory.Report, []objectiveadvisory.Capsule, error) {
	if options.Advisory == nil {
		return objectiveadvisory.Report{}, nil, nil
	}
	if options.ObjectiveID == "" || options.Generation < 1 {
		return objectiveadvisory.Report{}, nil, fmt.Errorf(
			"repository advisory requires an exact objective ID and positive generation",
		)
	}
	projection := objectiveRepositoryAdvisoryProjection(input, options)
	config := options.Advisory.Configuration()
	if err := config.Validate(); err != nil {
		return objectiveadvisory.Report{}, nil, fmt.Errorf("repository advisory runner configuration: %w", err)
	}
	gap := objectiveadvisory.SemanticGap{
		ObjectiveID: options.ObjectiveID, Generation: options.Generation,
		Requirement: input.ExactRequirement, Candidate: review.AnswerText,
		Evidence: objectiveRepositoryAdvisoryEvidence(review.Evidence),
	}
	report, err := options.Advisory.Run(ctx, projection, gap)
	if err != nil {
		return report, nil, err
	}
	if err := validateObjectiveRepositoryAdvisoryReport(report, projection, gap, config, options); err != nil {
		return report, nil, err
	}
	if report.Mode != objectiveadvisory.ModeActive {
		return report, nil, nil
	}
	return report, cloneObjectiveAdvisoryCapsules(report.ActiveCapsules), nil
}

func objectiveRepositoryAdvisoryProjection(
	input assemblyline.GroundedAnswerInput,
	options objectiveRepositoryGroundedClosureOptions,
) objectiveadvisory.ProjectionInput {
	authorities := make([]objectiveadvisory.TextAuthority, 0, len(input.Context.UserAuthorities)+2)
	authorities = append(authorities, objectiveadvisory.TextAuthority{
		ID: "current-user-objective", Content: input.ExactRequirement,
	})
	for _, authority := range input.Context.UserAuthorities {
		authorities = append(authorities, objectiveadvisory.TextAuthority{
			ID: fmt.Sprintf("user-message-%d", authority.MessageID), Content: authority.Content,
		})
	}
	if replan := input.Context.ReplanAuthority; replan != nil {
		authorities = append(authorities, objectiveadvisory.TextAuthority{
			ID:      fmt.Sprintf("replan-job-%d-generation-%d", replan.JobID, replan.Generation),
			Content: replan.Feedback,
		})
	}
	return objectiveadvisory.ProjectionInput{
		ObjectiveID: options.ObjectiveID, Generation: options.Generation,
		Objective: input.ExactRequirement, UserAuthorities: authorities,
		GroundedEvidence: objectiveRepositoryAdvisoryEvidence(input.Evidence),
		Constraints:      []string{}, Decisions: []string{}, Invariants: []string{},
		UnresolvedQuestions: []string{}, UsefulAdvice: objectiveRepositoryUsefulAdvice,
	}
}

func objectiveRepositoryAdvisoryEvidence(
	evidence []assemblyline.GroundedEvidenceCapsule,
) []objectiveadvisory.EvidenceSummary {
	projected := make([]objectiveadvisory.EvidenceSummary, len(evidence))
	for index, item := range evidence {
		digest := sha256.Sum256([]byte(item.Text))
		projected[index] = objectiveadvisory.EvidenceSummary{
			ID: item.ID, Summary: item.Text, SHA256: hex.EncodeToString(digest[:]),
		}
	}
	return projected
}

func validateObjectiveRepositoryAdvisoryReport(
	report objectiveadvisory.Report,
	input objectiveadvisory.ProjectionInput,
	gap objectiveadvisory.SemanticGap,
	config objectiveadvisory.Config,
	options objectiveRepositoryGroundedClosureOptions,
) error {
	if input.ObjectiveID != options.ObjectiveID || input.Generation != options.Generation {
		return fmt.Errorf("repository advisory report projection escaped objective or generation scope")
	}
	if err := report.ValidateFor(input, gap, config); err != nil {
		return fmt.Errorf("repository advisory report provenance: %w", err)
	}
	return nil
}

func cloneObjectiveAdvisoryCapsules(
	values []objectiveadvisory.Capsule,
) []objectiveadvisory.Capsule {
	return append([]objectiveadvisory.Capsule(nil), values...)
}
