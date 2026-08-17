package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type applicationEvidenceResolver func(
	assemblyline.ApplicationEvidenceNeed,
) ([]assemblyline.ApplicationContextEvidence, error)

func resolveDirectCodingApplicationContext(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	context assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
	resolveEvidence applicationEvidenceResolver,
) (assemblyline.ApplicationContext, error) {
	if context.WorkspaceState == assemblyline.ApplicationWorkspaceEmpty {
		// Code already knows that this workspace has no repository behavior or
		// ownership to establish. Asking a model whether it has questions would
		// be a ceremonial call, not a named semantic uncertainty.
		return context, nil
	}
	input := assemblyline.ApplicationContextNeedInput{UserRequest: authority, Context: context}
	job, err := assemblyline.NewApplicationContextNeedJob(input)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	decision, err := runDirectCodingSemanticCall[assemblyline.ApplicationContextNeedDecision](
		runtime, modelName, "application_context_needs", job, identities,
		func(value assemblyline.ApplicationContextNeedDecision) error { return value.Validate() },
	)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	if len(decision.Questions) == 0 {
		return context, nil
	}
	if resolveEvidence == nil {
		return assemblyline.ApplicationContext{}, fmt.Errorf(
			"application context has %d unresolved evidence needs without a registered resolver: %s",
			len(decision.Questions), strings.Join(decision.Questions, " | "),
		)
	}
	for index, question := range decision.Questions {
		need, err := assemblyline.NewApplicationRepositoryContextNeed(index+1, question)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		evidence, err := resolveEvidence(need)
		if err != nil {
			return assemblyline.ApplicationContext{}, fmt.Errorf(
				"resolve application evidence need %q: %w", need.ID, err,
			)
		}
		context, err = assemblyline.AppendApplicationContextEvidence(context, need, evidence)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
	}
	return context, nil
}
