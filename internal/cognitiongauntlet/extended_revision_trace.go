package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

type extendedRevisionAction struct {
	ordinal int64
	action  queue.CognitionTraceAction
}

func validateExtendedRevisionTrace(trace productionTrace) (string, error) {
	hypothesis, revision, plan, err := extendedRevisionActions(trace)
	if err != nil {
		return "", err
	}
	materialization, record, err := extendedBeliefRevision(trace)
	if err != nil {
		return "", err
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(revision.action.Decision)
	if err != nil || materialization.SourceDecisionSHA256 != decisionSHA ||
		record.CallOrdinal != revision.ordinal {
		return "", fmt.Errorf("extended revision rejection differs from its exact policy decision")
	}
	proposal := revision.action.Decision.Proposals[0].Revision
	if proposal == nil || proposal.TargetRef != materialization.TargetRef ||
		!reflect.DeepEqual(proposal.EvidenceRefs, materialization.EvidenceRefs) {
		return "", fmt.Errorf("extended revision proposal differs from its code-owned rejection")
	}
	if !revisionTargetProjected(trace, revision.ordinal, materialization.TargetRef) ||
		revisionTargetProjected(trace, plan.ordinal, materialization.TargetRef) {
		return "", fmt.Errorf("extended revision target was not active only until its rejection")
	}
	if !contradictionEvidencePrecedes(trace, revision.ordinal, materialization.EvidenceRefs) {
		return "", fmt.Errorf("extended revision lacks exact earlier tool contradiction evidence")
	}
	planRevision, planRecord, graphSHA, err := extendedPlanRevision(trace, plan)
	if err != nil {
		return "", err
	}
	return digestJSON(struct {
		HypothesisDecision string `json:"hypothesis_decision_sha256"`
		RevisionDescriptor string `json:"revision_descriptor_sha256"`
		PlanDescriptor     string `json:"plan_descriptor_sha256"`
		ResultGraph        string `json:"result_graph_sha256"`
		GraphPayload       string `json:"graph_payload_sha256"`
	}{
		mustDecisionSHA(hypothesis.action.Decision), record.SHA256,
		planRecord.SHA256, planRevision.ResultGraphSHA256, graphSHA,
	})
}

func extendedRevisionActions(
	trace productionTrace,
) (extendedRevisionAction, extendedRevisionAction, extendedRevisionAction, error) {
	var hypothesis, revision, plan extendedRevisionAction
	for _, record := range trace.Records {
		if record.Kind != "action" {
			continue
		}
		var action queue.CognitionTraceAction
		if decodeProductionPayload(record.Payload, &action, "extended revision action") != nil ||
			len(action.Decision.Proposals) != 1 {
			continue
		}
		candidate := extendedRevisionAction{record.CallOrdinal, action}
		switch action.Decision.Proposals[0].Kind {
		case cognition.ProposalHypothesis:
			if hypothesis.ordinal != 0 {
				return hypothesis, revision, plan, fmt.Errorf("extended revision has duplicate hypotheses")
			}
			hypothesis = candidate
		case cognition.ProposalRevision:
			revision = candidate
		case cognition.ProposalPlanRevision:
			plan = candidate
		}
	}
	if hypothesis.ordinal <= 0 || revision.ordinal <= hypothesis.ordinal || plan.ordinal <= revision.ordinal {
		return hypothesis, revision, plan, fmt.Errorf("extended revision proposal sequence is incomplete or unordered")
	}
	return hypothesis, revision, plan, nil
}

func extendedBeliefRevision(
	trace productionTrace,
) (cognitionstate.BeliefRevisionMaterialization, queue.CognitionSealedTraceRecord, error) {
	var value cognitionstate.BeliefRevisionMaterialization
	var found queue.CognitionSealedTraceRecord
	for _, record := range trace.Records {
		if record.Kind != "belief_revision" {
			continue
		}
		if found.ID != "" || decodeProductionPayload(record.Payload, &value, "belief revision") != nil ||
			value.Validate() != nil {
			return value, record, fmt.Errorf("extended revision has invalid or duplicate rejection authority")
		}
		found = record
	}
	if found.ID == "" {
		return value, found, fmt.Errorf("extended revision sealed trace omitted its code-owned rejection")
	}
	return value, found, nil
}

func revisionTargetProjected(
	trace productionTrace,
	ordinal int64,
	target cognition.EpistemicRef,
) bool {
	for _, record := range trace.Records {
		if record.Kind != "context_projection" || record.CallOrdinal != ordinal {
			continue
		}
		var projection contextbuilder.Projection
		if decodeProductionPayload(record.Payload, &projection, "revision projection") != nil {
			return false
		}
		for _, selected := range projection.Selected {
			if selected.Role == workingset.RoleHypothesis && selected.Ref.URI == target.URI &&
				selected.Ref.Version == target.Version && selected.Ref.Hash == target.SHA256 {
				return true
			}
		}
	}
	return false
}

func contradictionEvidencePrecedes(
	trace productionTrace,
	revisionOrdinal int64,
	refs []cognition.EvidenceRef,
) bool {
	want := make(map[cognition.EvidenceRef]struct{}, len(refs))
	for _, ref := range refs {
		want[ref] = struct{}{}
	}
	for _, record := range trace.Records {
		if record.Kind != "transition" || record.CallOrdinal >= revisionOrdinal {
			continue
		}
		var transition cognition.Transition
		if decodeProductionPayload(record.Payload, &transition, "revision evidence transition") != nil {
			return false
		}
		for _, observation := range transition.Observations {
			delete(want, observation.EvidenceRef())
		}
	}
	return len(want) == 0
}

func extendedPlanRevision(
	trace productionTrace,
	plan extendedRevisionAction,
) (cognition.PlanRevisionMaterialization, queue.CognitionSealedTraceRecord, string, error) {
	proposal := plan.action.Decision.Proposals[0].PlanRevision
	if proposal == nil {
		return cognition.PlanRevisionMaterialization{}, queue.CognitionSealedTraceRecord{}, "",
			fmt.Errorf("extended plan revision omitted its typed descriptor")
	}
	var value cognition.PlanRevisionMaterialization
	var revisionRecord queue.CognitionSealedTraceRecord
	for _, record := range trace.Records {
		if record.Kind != "plan_revision" {
			continue
		}
		if revisionRecord.ID != "" ||
			decodeProductionPayload(record.Payload, &value, "plan revision") != nil ||
			value.Validate() != nil {
			return value, record, "", fmt.Errorf("extended plan revision descriptor is invalid or duplicated")
		}
		revisionRecord = record
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(plan.action.Decision)
	if err != nil || revisionRecord.ID == "" || revisionRecord.ID != value.ID ||
		revisionRecord.CallOrdinal != plan.ordinal ||
		value.SourceSnapshotSHA256 != plan.action.SnapshotSHA256 ||
		value.SourceDecisionSHA256 != decisionSHA ||
		!reflect.DeepEqual(value.Next.Desired, proposal.Next) ||
		!reflect.DeepEqual(value.Next.SupportingRefs, proposal.EvidenceRefs) {
		return value, revisionRecord, "", fmt.Errorf("extended plan revision differs from its exact policy decision")
	}
	for _, record := range trace.Records {
		if record.Kind != "obligation_graph" || record.Sequence != revisionRecord.Sequence ||
			record.ID != value.ID || record.Phase != 72 {
			continue
		}
		var graph cognition.ObligationGraphSnapshot
		if err := decodeProductionPayload(record.Payload, &graph, "revised obligation graph"); err != nil {
			return value, revisionRecord, "", err
		}
		if graph.Generation != value.NextGeneration || graph.RootID != value.Root.ID ||
			graph.SHA256 != value.ResultGraphSHA256 {
			return value, revisionRecord, "", fmt.Errorf("extended plan revision result graph changed")
		}
		root, rootFound := extendedTraceObligation(graph, value.Root.ID)
		next, nextFound := extendedTraceObligation(graph, value.Next.ID)
		if !rootFound || !nextFound || root.Status != cognition.ObligationBlocked ||
			next.Status != cognition.ObligationActive ||
			!reflect.DeepEqual(root.Desired, value.Root.Desired) ||
			!reflect.DeepEqual(root.DependsOn, value.Root.DependsOn) ||
			!reflect.DeepEqual(next.Desired, value.Next.Desired) ||
			!reflect.DeepEqual(next.SupportingRefs, value.Next.SupportingRefs) {
			return value, revisionRecord, "", fmt.Errorf("extended plan revision graph changed its exact root or next obligation")
		}
		return value, revisionRecord, record.SHA256, nil
	}
	return value, revisionRecord, "", fmt.Errorf("extended plan revision lacks its exact sealed result graph")
}

func extendedTraceObligation(
	graph cognition.ObligationGraphSnapshot,
	id cognition.ObligationID,
) (cognition.Obligation, bool) {
	for _, obligation := range graph.Obligations {
		if obligation.ID == id {
			return obligation, true
		}
	}
	return cognition.Obligation{}, false
}

func mustDecisionSHA(decision cognition.CognitionDecision) string {
	value, err := cognitionruntime.DecisionSHA256(decision)
	if err != nil {
		return ""
	}
	return value
}
