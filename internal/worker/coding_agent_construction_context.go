package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) resolveAgentConstructionContext(inputs directCodingApplicationInputs) ([]codingagent.ConstructionFact, error) {
	input := assemblyline.ConstructionContextInput{UserRequest: inputs.RequestAuthority.modelRequest}
	job, err := assemblyline.NewConstructionContextJob(input)
	if err != nil {
		return nil, err
	}
	model, err := session.workerModel(station.CodingProjectStackConstraint)
	if err != nil {
		return nil, err
	}
	context, err := runDirectCodingSemanticLeafCall(inputs.Runtime, model, "application_construction_context", job, inputs.Identities,
		func(raw string) (assemblyline.ConstructionContext, error) {
			return assemblyline.DecodeConstructionContext(input, raw)
		})
	if err != nil {
		return nil, err
	}
	facts := make([]codingagent.ConstructionFact, len(context.Facts))
	for index, fact := range context.Facts {
		quote, err := assemblyline.RestoreArtifactIdentities(fact.Quote, inputs.Identities)
		if err != nil {
			return nil, err
		}
		facts[index] = codingagent.ConstructionFact{Kind: fact.Kind, Quote: quote}
	}
	return facts, nil
}
