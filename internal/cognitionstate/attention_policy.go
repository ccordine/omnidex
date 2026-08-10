package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func BuildDefaultReconciliation(input ReconciliationInput) (ReconciliationPlan, error) {
	mandatory, evidence, err := buildMandatoryCandidates(input)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	mandatory, err = applyRequiredAttention(input, mandatory, evidence)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	optional, outcomes, err := applyAdvisoryRequests(input, mandatory, evidence)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	candidates := append(append([]attentionCandidate(nil), mandatory...), optional...)
	sortCandidates(candidates)
	sourceSHA, err := reconciliationSourceSHA(input)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	builder, err := newAttentionCommandBuilder(input, sourceSHA)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	if err := builder.releaseUndesired(candidates); err != nil {
		return ReconciliationPlan{}, err
	}
	accepted, outcomes, err := builder.ensureCandidates(candidates, outcomes)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	goalRef, exists := candidateForKey(accepted, "goal")
	if !exists {
		return ReconciliationPlan{}, fmt.Errorf("%w: required goal candidate is missing", ErrInvalidReconciliation)
	}
	spec, materials, err := buildContext(builder.set, accepted, goalRef.ref, sourceSHA)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	descriptor, err := attentionPlanDescriptor(sourceSHA, builder.commands, spec)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	plan := ReconciliationPlan{
		descriptor: descriptor, commands: append([]WorkingSetMutation(nil), builder.commands...),
		spec: spec, materials: materials, outcomes: outcomes,
	}
	if err := plan.Validate(); err != nil {
		return ReconciliationPlan{}, err
	}
	return plan, nil
}

func reconciliationSourceSHA(input ReconciliationInput) (string, error) {
	evidence := append([]EvidenceMaterial(nil), input.Evidence...)
	return mappingDigest(struct {
		Schema      string
		StateSHA256 string
		GraphSHA256 string
		Ledger      taskstate.MaterializedState
		WorkingSet  workingset.Snapshot
		Evidence    []EvidenceMaterial
		Required    []cognition.AttentionRequest
		Attention   []cognition.AttentionRequest
		Rejected    []cognition.EvidenceRef
	}{
		AttentionPlanSchemaV1, input.State.SHA256(), input.ObligationGraph.SHA256,
		input.Ledger, input.WorkingSet, evidence, input.RequiredAttention, input.Attention,
		append([]cognition.EvidenceRef(nil), input.CapacityRejected...),
	})
}

func attentionPlanDescriptor(
	sourceSHA string,
	commands []WorkingSetMutation,
	spec any,
) (AttentionPlanDescriptor, error) {
	commandHashes := make([]string, len(commands))
	for index, command := range commands {
		commandHashes[index] = command.descriptor.SHA256
	}
	digest, err := mappingDigest(struct {
		Schema, SourceSHA string
		Commands          []string
		Spec              any
	}{AttentionPlanSchemaV1, sourceSHA, commandHashes, spec})
	if err != nil {
		return AttentionPlanDescriptor{}, err
	}
	return AttentionPlanDescriptor{
		Schema: AttentionPlanSchemaV1, ID: "cognition_attention_" + digest,
		SHA256: digest, SourceSHA256: sourceSHA, CommandCount: len(commands),
	}, nil
}

func candidateForKey(candidates []attentionCandidate, key string) (attentionCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.key == key {
			return candidate, true
		}
	}
	return attentionCandidate{}, false
}
