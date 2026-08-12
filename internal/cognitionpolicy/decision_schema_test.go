package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestDecisionSchemaUsesStrictTaggedPlanningAndRevisionProposals(t *testing.T) {
	t.Parallel()
	raw, err := decisionSchemaJSON(policyTestSnapshotCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties := requireSchemaMap(t, root, "properties")
	proposals := requireSchemaMap(t, properties, "ledger_proposals")
	items := requireSchemaMap(t, proposals, "items")
	variants, ok := items["oneOf"].([]any)
	if !ok || len(variants) != 6 {
		t.Fatalf("proposal variants = %#v, want six exact tagged variants", items["oneOf"])
	}
	var obligation, revision, planRevision map[string]any
	for _, rawVariant := range variants {
		variant, ok := rawVariant.(map[string]any)
		if !ok || variant["additionalProperties"] != false {
			t.Fatalf("proposal variant is not strict: %#v", rawVariant)
		}
		variantProperties := requireSchemaMap(t, variant, "properties")
		kind := requireSchemaMap(t, variantProperties, "kind")
		if kind["const"] == "obligation" {
			obligation = variant
		} else if kind["const"] == "revision" {
			revision = variant
		} else if kind["const"] == "plan_revision" {
			planRevision = variant
		}
	}
	if obligation == nil {
		t.Fatal("strict obligation proposal variant is missing")
	}
	if revision == nil {
		t.Fatal("strict revision proposal variant is missing")
	}
	if planRevision == nil {
		t.Fatal("strict plan revision proposal variant is missing")
	}
	if got := obligation["required"]; !reflect.DeepEqual(got, []any{"kind", "obligation"}) {
		t.Fatalf("obligation required fields = %#v", got)
	}
	obligationProperties := requireSchemaMap(t, obligation, "properties")
	if len(obligationProperties) != 2 {
		t.Fatalf("obligation top-level properties = %#v", obligationProperties)
	}
	payload := requireSchemaMap(t, obligationProperties, "obligation")
	if payload["additionalProperties"] != false {
		t.Fatalf("obligation payload is not strict: %#v", payload)
	}
	if got := payload["required"]; !reflect.DeepEqual(got, []any{"desired", "evidence_refs"}) {
		t.Fatalf("obligation payload required fields = %#v", got)
	}
	payloadProperties := requireSchemaMap(t, payload, "properties")
	for _, forbidden := range []string{"id", "status", "completion_check", "completion_check_id", "depends_on"} {
		if _, exists := payloadProperties[forbidden]; exists {
			t.Fatalf("model-owned obligation field %q is exposed", forbidden)
		}
	}
	if refs := requireSchemaMap(t, payloadProperties, "evidence_refs"); refs["minItems"] != float64(1) {
		t.Fatalf("obligation evidence is not required: %#v", refs)
	}
	revisionProperties := requireSchemaMap(t, revision, "properties")
	if len(revisionProperties) != 2 {
		t.Fatalf("revision top-level properties = %#v", revisionProperties)
	}
	revisionPayload := requireSchemaMap(t, revisionProperties, "revision")
	if revisionPayload["additionalProperties"] != false ||
		!reflect.DeepEqual(revisionPayload["required"], []any{"target_ref", "evidence_refs"}) {
		t.Fatalf("revision payload is not strict: %#v", revisionPayload)
	}
	revisionFields := requireSchemaMap(t, revisionPayload, "properties")
	target := requireSchemaMap(t, revisionFields, "target_ref")
	if target["additionalProperties"] != false ||
		!reflect.DeepEqual(target["required"], []any{"uri", "version", "content_sha256"}) {
		t.Fatalf("revision target is not exact: %#v", target)
	}
	if refs := requireSchemaMap(t, revisionFields, "evidence_refs"); refs["minItems"] != float64(1) {
		t.Fatalf("revision contradiction evidence is not required: %#v", refs)
	}
	planFields := requireSchemaMap(t, planRevision, "properties")
	if len(planFields) != 2 {
		t.Fatalf("plan revision top-level properties = %#v", planFields)
	}
	planPayload := requireSchemaMap(t, planFields, "plan_revision")
	if planPayload["additionalProperties"] != false ||
		!reflect.DeepEqual(planPayload["required"], []any{"next", "evidence_refs"}) {
		t.Fatalf("plan revision payload is not strict: %#v", planPayload)
	}
	planProperties := requireSchemaMap(t, planPayload, "properties")
	for _, forbidden := range []string{"id", "root", "generation", "status", "depends_on", "completion_check"} {
		if _, exists := planProperties[forbidden]; exists {
			t.Fatalf("model-owned plan field %q is exposed", forbidden)
		}
	}
	if refs := requireSchemaMap(t, planProperties, "evidence_refs"); refs["minItems"] != float64(1) {
		t.Fatalf("plan revision evidence is not required: %#v", refs)
	}
}

func TestPolicyAcceptsTypedObligationAndRejectsRemovedFreeTextForm(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "typed obligation context")
	snapshot, evidence := policyTestSnapshot(t, projection)
	typed := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{
			Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Expose public properties.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalObligation,
			Obligation: &cognition.ObligationProposal{
				Desired: cognition.GoalExpression{All: []cognition.Predicate{{
					Name: "condition.prerequisite", Args: []string{"target-1"},
				}}},
				EvidenceRefs: []cognition.EvidenceRef{evidence},
			},
		}},
	}
	raw, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := New(
		&policyTestClient{response: string(raw)}, policyTestAttestedBrain(), policyTestActivation(),
		newPolicyTestProjectionLoader(projection), &policyTestCallJournal{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); err != nil {
		t.Fatalf("typed obligation decision: %v", err)
	}

	removed := typed.Clone()
	removed.Proposals[0].Obligation = nil
	removed.Proposals[0].Content = "Resolve the prerequisite."
	removed.Proposals[0].EvidenceRefs = []cognition.EvidenceRef{evidence}
	raw, err = json.Marshal(removed)
	if err != nil {
		t.Fatal(err)
	}
	journal := &policyTestCallJournal{}
	policy, err = New(
		&policyTestClient{response: string(raw)}, policyTestAttestedBrain(), policyTestActivation(),
		newPolicyTestProjectionLoader(projection), journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("removed free-text form error = %v, want ErrInvalidDecision", err)
	}
	if len(journal.results) != 1 || journal.results[0].Status != CallResultRejected {
		t.Fatalf("removed form did not persist a rejection: %#v", journal.results)
	}
}

func policyTestSnapshotCatalog(t *testing.T) cognition.ActionCatalog {
	t.Helper()
	projection := policyTestProjection(t, "schema context")
	snapshot, _ := policyTestSnapshot(t, projection)
	return snapshot.ActionCatalog()
}

func requireSchemaMap(t *testing.T, value map[string]any, key string) map[string]any {
	t.Helper()
	result, ok := value[key].(map[string]any)
	if !ok {
		t.Fatalf("schema field %q = %#v, want object", key, value[key])
	}
	return result
}
