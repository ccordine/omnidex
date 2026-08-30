package worker

import (
	"fmt"

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
	authorityContext := context
	authorityContext.Facts = append([]assemblyline.ApplicationContextFact(nil), context.Facts...)
	inventoryInput := assemblyline.ApplicationContextQuestionInventoryInput{
		UserRequest: authority,
		Context:     authorityContext,
	}
	inventoryJob, err := assemblyline.NewApplicationContextQuestionInventoryJob(inventoryInput)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, modelName, "application_context_question_inventory",
		inventoryJob, identities,
		func(raw string) (assemblyline.ApplicationContextQuestionInventory, error) {
			return assemblyline.DecodeApplicationContextQuestionInventory(inventoryInput, raw)
		},
		func(value assemblyline.ApplicationContextQuestionInventory) error {
			if err := value.ValidateFor(inventoryInput); err != nil {
				return err
			}
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application context question inventory",
				runtime.PathProvenance,
				value.Candidates...,
			)
		},
	)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	acceptedQuestions := make([]string, 0, len(inventory.Candidates))
	seenCandidates := make(map[string]struct{}, len(inventory.Candidates))
	for candidateIndex, question := range inventory.Candidates {
		if _, duplicate := seenCandidates[question]; duplicate {
			continue
		}
		seenCandidates[question] = struct{}{}
		necessityInput := assemblyline.ApplicationContextQuestionNecessityInput{
			Authority:      inventoryInput,
			Inventory:      inventory,
			CandidateIndex: candidateIndex,
			CurrentContext: context,
		}
		necessityJob, err := assemblyline.NewApplicationContextQuestionNecessityJob(
			necessityInput,
		)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		necessity, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "application_context_question_necessity",
			necessityJob, identities,
			func(raw string) (assemblyline.ApplicationContextQuestionNecessityResult, error) {
				return assemblyline.DecodeApplicationContextQuestionNecessityResult(
					necessityInput,
					raw,
				)
			},
			func(value assemblyline.ApplicationContextQuestionNecessityResult) error {
				return value.ValidateFor(necessityInput)
			},
		)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		if necessity.Relation != assemblyline.ApplicationContextQuestionNecessary {
			continue
		}
		semanticDuplicate := false
		for _, acceptedQuestion := range acceptedQuestions {
			relationInput := assemblyline.ApplicationContextQuestionRelationInput{
				CandidateQuestion: question,
				AcceptedQuestion:  acceptedQuestion,
			}
			relationJob, err := assemblyline.NewApplicationContextQuestionRelationJob(
				relationInput,
			)
			if err != nil {
				return assemblyline.ApplicationContext{}, err
			}
			relation, err := runDirectCodingSemanticLeafCall(
				runtime, modelName, "application_context_question_relation",
				relationJob, identities,
				func(raw string) (assemblyline.ApplicationContextQuestionRelationResult, error) {
					return assemblyline.DecodeApplicationContextQuestionRelationResult(
						relationInput,
						raw,
					)
				},
				func(value assemblyline.ApplicationContextQuestionRelationResult) error {
					return value.ValidateFor(relationInput)
				},
			)
			if err != nil {
				return assemblyline.ApplicationContext{}, err
			}
			if relation.Relation == assemblyline.ApplicationContextQuestionsSameFact {
				semanticDuplicate = true
				break
			}
		}
		if semanticDuplicate {
			continue
		}
		if resolveEvidence == nil {
			return assemblyline.ApplicationContext{}, fmt.Errorf(
				"authorized application context repository-fact question has no registered resolver: %s",
				question,
			)
		}
		need, err := assemblyline.NewApplicationRepositoryContextNeed(
			len(acceptedQuestions)+1,
			question,
		)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		evidence, err := resolveEvidence(need)
		if err != nil {
			return assemblyline.ApplicationContext{}, fmt.Errorf(
				"resolve application evidence need %q: %w",
				need.ID,
				err,
			)
		}
		context, err = assemblyline.AppendApplicationContextEvidence(context, need, evidence)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		acceptedQuestions = append(acceptedQuestions, question)
	}
	return context, nil
}
