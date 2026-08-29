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
	questions := make([]string, 0, assemblyline.MaxApplicationEvidenceNeeds)
	for {
		leafInput := assemblyline.ApplicationContextNeedLeafInput{
			UserRequest: authority, Context: context,
			AcceptedQuestions: append([]string{}, questions...),
		}
		coverageJob, err := assemblyline.NewApplicationContextNeedCoverageJob(leafInput)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		coverage, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "application_context_need_coverage",
			coverageJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationContextNeedCoverageLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		if coverage == assemblyline.ApplicationNoUncoveredContextNeed {
			break
		}
		if len(questions) == assemblyline.MaxApplicationEvidenceNeeds {
			return assemblyline.ApplicationContext{}, fmt.Errorf(
				"application context need coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxApplicationEvidenceNeeds,
			)
		}
		questionJob, err := assemblyline.NewApplicationContextNeedQuestionJob(leafInput)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		question, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "application_context_need_question",
			questionJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationContextNeedQuestionLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"application context need question", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		questions = append(questions, question)
	}
	decision, err := assemblyline.AssembleApplicationContextNeedDecision(input, questions)
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
