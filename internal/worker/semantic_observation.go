package worker

import (
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

// This observes the existing station contract and already-rendered request.
// It adds no model-visible bytes, calls, decisions or execution authority.
func beginSemanticObservation(runtime typedWorkerRuntime, job assemblyline.PortableJob, modelName, station, prompt string, inferenceFree bool, provenResult string) (*model.PlanningActivity, error) {
	if runtime.ObservePlanning == nil || runtime.Context.Err() != nil {
		return nil, nil
	}
	contract, err := assemblyline.SemanticUncertaintyContractForWorkKind(job.Kind)
	if err != nil {
		return nil, err
	}
	subject, against, err := semanticObservationSubjects(job)
	if err != nil {
		return nil, err
	}
	activity := model.PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: string(job.Kind), Station: station, Model: modelName, Subject: subject, Against: against, Question: contract.ExactQuestion, Why: contract.DeterministicLimitation, Consumer: contract.DeterministicConsumer, InferenceFree: inferenceFree}
	activity.Options = []string{"Free-form output: " + contract.SingleResult}
	options, opaque, err := assemblyline.PortableJobOpaqueChoiceOptions(job)
	if err != nil {
		return nil, err
	}
	if opaque {
		activity.Options = options
		if !inferenceFree {
			activity.Question, err = assemblyline.PortableJobOpaqueQuestion(job, prompt, options)
			if err != nil {
				return nil, err
			}
		}
	}
	if inferenceFree {
		activity.Why = "The station's existing deterministic resolver proves one result from the supplied inputs; no model call is needed."
		if !opaque {
			activity.Options = []string{"Unique code-resolved result: " + provenResult}
		}
	}
	if err := publishSemanticOpening(runtime, activity); err != nil {
		return nil, err
	}
	return &activity, nil
}

func completeSemanticObservation(runtime typedWorkerRuntime, opened *model.PlanningActivity, validatedResult string, callID int64) {
	if opened == nil {
		return
	}
	activity := *opened
	activity.Kind, activity.Result, activity.CallID = "semantic_completed", strings.TrimSpace(validatedResult), callID
	for _, option := range activity.Options {
		id, _, exists := strings.Cut(option, ". ")
		if exists && activity.Result == id {
			activity.Result = option
			break
		}
	}
	publishSemanticResult(runtime, activity)
}
