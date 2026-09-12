package assemblyline

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPortableWorkAndResultContainValuesNotContentIdentities(t *testing.T) {
	t.Parallel()
	for _, input := range []any{
		ApplicationClassificationInput{UserRequest: "Classify one interface."},
		FragmentGenerationInput{
			Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function Total(left, right)",
			Behavior: "Return the sum of both inputs.",
		},
	} {
		kind := WorkApplicationClassify
		if _, source := input.(FragmentGenerationInput); source {
			kind = WorkFragmentGeneration
		}
		work, err := newPortableJob(kind, input)
		if err != nil {
			t.Fatal(err)
		}
		if err := work.Validate(); err != nil {
			t.Fatal(err)
		}
		copy, err := newPortableJob(kind, input)
		if err != nil || work.Key() != copy.Key() {
			t.Fatalf("equal inputs did not compare equal: %v", err)
		}
		payload, err := json.Marshal(work)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 || fields["id"] != nil || fields["kind"] == nil || fields["payload"] == nil {
			t.Fatalf("portable work contains identity or lost actual inputs: %s", payload)
		}
		candidate := "A"
		if kind == WorkFragmentGeneration {
			candidate = "return left + right;"
		}
		result := PortableResult{Candidate: candidate}
		if err := result.ValidateFor(work); err != nil {
			t.Fatal(err)
		}
		if reflect.TypeOf(result).NumField() != 1 {
			t.Fatal("portable result contains something besides the semantic candidate")
		}
	}
}

func TestPortableKeyComparesAllExactInputBytesAndKind(t *testing.T) {
	t.Parallel()
	one := PortableJob{Schema: PortableJobSchemaV2, Kind: WorkApplicationClassify, Payload: json.RawMessage(`{"user_request":"first"}`)}
	two := one
	two.Payload = json.RawMessage(`{"user_request":"second"}`)
	if one.Key() == two.Key() {
		t.Fatal("changed input compared equal")
	}
	two = one
	two.Kind = WorkArtifactHandling
	if one.Key() == two.Key() {
		t.Fatal("changed station compared equal")
	}
	key := one.Key()
	one.Payload[0] = '['
	if key.Input != `{"user_request":"first"}` {
		t.Fatal("captured input key changed with a mutable input slice")
	}
}
