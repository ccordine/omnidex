package worker

import (
	"encoding/json"
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
	seen := make(map[string]struct{})
	needIndex := 0
	for round := 1; ; round++ {
		identity, err := applicationContextIdentity(context)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		if _, repeated := seen[identity]; repeated {
			return assemblyline.ApplicationContext{}, fmt.Errorf(
				"application context investigation repeated unchanged authoritative state",
			)
		}
		seen[identity] = struct{}{}
		input := assemblyline.ApplicationContextNeedInput{
			UserRequest: authority, Context: context,
		}
		job, err := assemblyline.NewApplicationContextNeedJob(input)
		if err != nil {
			return assemblyline.ApplicationContext{}, err
		}
		decision, err := runDirectCodingSemanticCall[assemblyline.ApplicationContextNeedDecision](
			runtime, modelName, fmt.Sprintf("application_context_needs_%d", round),
			job, identities,
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
		for _, question := range decision.Questions {
			needIndex++
			need, err := assemblyline.NewApplicationRepositoryContextNeed(needIndex, question)
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
	}
}

func applicationContextIdentity(context assemblyline.ApplicationContext) (string, error) {
	if err := context.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(context)
	if err != nil {
		return "", fmt.Errorf("encode application context identity: %w", err)
	}
	return assemblyline.ExactObjectiveContextSHA(string(raw)), nil
}
