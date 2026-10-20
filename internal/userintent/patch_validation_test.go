package userintent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPatchStructuralValidationAndSequentialArrays(t *testing.T) {
	before := json.RawMessage(`{"items":["one","two","three"],"a/b":{"~key":false},"large":9007199254740993}`)
	patches := []Patch{{Op: "remove", Path: "/items/0"}, {Op: "replace", Path: "/items/1", Value: json.RawMessage(`"third"`)}, {Op: "replace", Path: "/a~1b/~0key", Value: json.RawMessage(`true`)}}
	after, err := applyPatch(before, patches, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	state, err := DecodeState[struct {
		Items   []string        `json:"items"`
		Escaped map[string]bool `json:"a/b"`
		Large   int64           `json:"large"`
	}](after)
	if err != nil || len(state.Items) != 2 || state.Items[0] != "two" || state.Items[1] != "third" || !state.Escaped["~key"] || state.Large != 9007199254740993 {
		t.Fatal(state, err)
	}
	for _, patches := range [][]Patch{
		{{Op: "add", Path: "/large/field", Value: json.RawMessage(`1`)}},
		{{Op: "replace", Path: "/missing", Value: json.RawMessage(`1`)}},
		{{Op: "replace", Path: "/items/01", Value: json.RawMessage(`1`)}},
		{{Op: "replace", Path: "/a~2b", Value: json.RawMessage(`1`)}},
		{{Op: "remove", Path: "/large", Value: json.RawMessage(`1`)}},
		{{Op: "move", Path: "/large"}},
		{{Op: "replace", Path: "/items", Value: json.RawMessage(`[]`)}, {Op: "add", Path: "/items/-", Value: json.RawMessage(`1`)}},
	} {
		if _, err := applyPatch(before, patches, func(string) bool { return true }); err == nil {
			t.Fatalf("accepted invalid patch %+v", patches)
		}
	}
}
func TestReusableParserPreservesAnotherTypedState(t *testing.T) {
	// DecodeState is consumer-owned type validation, with no model-visible menu.
	type settings struct {
		Language string `json:"language"`
		Enabled  bool   `json:"enabled"`
	}
	before := json.RawMessage(`{"language":"English","enabled":true}`)
	after, err := applyPatch(before, []Patch{{Op: "replace", Path: "/language", Value: json.RawMessage(`"French"`)}}, func(path string) bool { return path == "/language" })
	state, decodeErr := DecodeState[settings](after)
	if err != nil || decodeErr != nil || state.Language != "French" || !state.Enabled {
		t.Fatal(state, err, decodeErr)
	}
	if _, err := DecodeState[settings](json.RawMessage(`{"language":"French","unknown":true}`)); err == nil {
		t.Fatal("unknown typed field accepted")
	}
}

func TestRepeatedLiteralDashKeyIsNotAnAppend(t *testing.T) {
	_, err := applyPatch(json.RawMessage(`{"items":{}}`), []Patch{
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"two"`)},
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"three"`)},
	}, func(string) bool { return true })
	if err == nil {
		t.Fatal("overwriting a literal object key accepted as an append")
	}
}

func TestCompoundArrayAppends(t *testing.T) {
	before := json.RawMessage(`{"items":["one"],"approved":false}`)
	after, err := applyPatch(before, []Patch{
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"two"`)},
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"three"`)},
	}, func(string) bool { return true })
	if err != nil || string(after) != `{"approved":false,"items":["one","two","three"]}` {
		t.Fatal(string(after), err)
	}
}

func TestRejectRedundantArrayRemovalAndAddition(t *testing.T) {
	before := json.RawMessage(`{"items":["one","two"],"approved":false}`)
	for _, additions := range [][]Patch{
		{{Op: "remove", Path: "/items/1"}, {Op: "add", Path: "/items/-", Value: json.RawMessage(`"two"`)}},
		{{Op: "add", Path: "/items/-", Value: json.RawMessage(`"two"`)}, {Op: "remove", Path: "/items/1"}},
	} {
		patches := append([]Patch{additions[0], {Op: "replace", Path: "/approved", Value: json.RawMessage(`true`)}}, additions[1])
		if _, err := applyPatch(before, patches, func(string) bool { return true }); err == nil {
			t.Fatal("redundant array round trip accepted", patches)
		}
	}
	// Moving an item is an actual change, even with an unrelated approval.
	after, err := applyPatch(before, []Patch{
		{Op: "remove", Path: "/items/0"},
		{Op: "replace", Path: "/approved", Value: json.RawMessage(`true`)},
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"one"`)},
	}, func(string) bool { return true })
	if err != nil || string(after) != `{"approved":true,"items":["two","one"]}` {
		t.Fatal(string(after), err)
	}
}

func TestRepeatedArrayRemovals(t *testing.T) {
	after, err := applyPatch(json.RawMessage(`{"items":["one","two","three"]}`), []Patch{
		{Op: "remove", Path: "/items/0"}, {Op: "remove", Path: "/items/0"},
	}, func(string) bool { return true })
	if err != nil || string(after) != `{"items":["three"]}` {
		t.Fatal(string(after), err)
	}
}

func TestRejectRedundantRemoveReplaceAppendGroup(t *testing.T) {
	before := json.RawMessage(`{"items":["one","two","three"],"approved":false}`)
	_, err := applyPatch(before, []Patch{
		{Op: "remove", Path: "/items/0"},
		{Op: "replace", Path: "/approved", Value: json.RawMessage(`true`)},
		{Op: "replace", Path: "/items/1", Value: json.RawMessage(`"two"`)},
		{Op: "add", Path: "/items/-", Value: json.RawMessage(`"three"`)},
	}, func(string) bool { return true })
	if err == nil {
		t.Fatal("redundant group accepted")
	}
}

func TestBoundedArrayDistance(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		distance    int
	}{
		{`[]`, `[]`, 0}, {`[]`, `[1,2]`, 2}, {`[1,2]`, `[]`, 2},
		{`[1,2,3]`, `[2,3]`, 1}, {`[1,2,3]`, `[2,2,3]`, 1},
		{`[1,2]`, `[2,1]`, 2}, {`[1,2,3]`, `[4,5,6]`, 3},
	} {
		left, _ := DecodeState[[]any](json.RawMessage(tc.left))
		right, _ := DecodeState[[]any](json.RawMessage(tc.right))
		for limit := 1; limit <= 4; limit++ {
			if got := arrayEditDistance(left, right, limit); got != min(tc.distance, limit) {
				t.Fatalf("%s -> %s limit %d: got %d", tc.left, tc.right, limit, got)
			}
		}
	}
}

func TestInterpretSequentialArrayPositions(t *testing.T) {
	for _, tc := range []struct {
		name, before, response, want string
	}{
		{"edit_different_shifted_items", `{"items":[{"name":"one"},{"name":"two"},{"name":"three"}]}`, `{"patch":[{"op":"replace","path":"/items/1/name","value":"second"},{"op":"remove","path":"/items/0"},{"op":"replace","path":"/items/1/name","value":"third"}]}`, `{"items":[{"name":"second"},{"name":"third"}]}`},
		{"remove_shifted_nested_values", `{"items":[{"values":[3]},{"values":[1,2,3]}]}`, `{"patch":[{"op":"remove","path":"/items/0"},{"op":"remove","path":"/items/0/values/0"},{"op":"remove","path":"/items/0/values/0"}]}`, `{"items":[{"values":[3]}]}`},
		{"edit_then_shift_nested_values", `{"items":[{"values":[9]},{"values":[1,2,3]}]}`, `{"patch":[{"op":"remove","path":"/items/1/values/0"},{"op":"remove","path":"/items/0"},{"op":"remove","path":"/items/0/values/0"}]}`, `{"items":[{"values":[3]}]}`},
		{"replace_shifted", `{"items":["one","two","three"]}`, `{"patch":[{"op":"remove","path":"/items/0"},{"op":"replace","path":"/items/0","value":"changed"}]}`, `{"items":["changed","three"]}`},
		{"edit_shifted_field", `{"items":[{"name":"one"},{"name":"two"}]}`, `{"patch":[{"op":"remove","path":"/items/0"},{"op":"replace","path":"/items/0/name","value":"changed"}]}`, `{"items":[{"name":"changed"}]}`},
		{"insert_same_index", `{"items":["one"]}`, `{"patch":[{"op":"add","path":"/items/0","value":"two"},{"op":"add","path":"/items/0","value":"three"}]}`, `{"items":["three","two","one"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, validations := 0, 0
			snapshot := json.RawMessage(tc.before)
			result, err := Interpret(context.Background(), Input{
				Snapshot: snapshot, Prompt: "Apply these intended sequential item changes.",
				Writable: func(path string) bool { return strings.HasPrefix(path, "/items/") },
				Validate: func(before, after json.RawMessage) error {
					validations++
					if string(before) != tc.before || string(after) != tc.want {
						return fmt.Errorf("unexpected transition: %s -> %s", before, after)
					}
					return nil
				},
			}, func(context.Context, string) (string, error) {
				calls++
				return tc.response, nil
			})
			if err != nil || calls != 1 || validations != 1 || len(result.Patch) < 2 || string(snapshot) != tc.before {
				t.Fatalf("result=%+v error=%v calls=%d validations=%d snapshot=%s", result, err, calls, validations, snapshot)
			}
		})
	}
}

func TestInterpretRejectsEditsToAppendedItem(t *testing.T) {
	for _, operations := range []string{
		`{"op":"add","path":"/items/-","value":{"name":"draft"}},{"op":"replace","path":"/items/0/name","value":"final"}`,
		`{"op":"add","path":"/items/0","value":{"name":"draft"}},{"op":"replace","path":"/items/0/name","value":"final"}`,
		`{"op":"add","path":"/items/-","value":{"name":"draft"}},{"op":"add","path":"/items/0","value":{"name":"other"}},{"op":"replace","path":"/items/1/name","value":"final"}`,
		`{"op":"add","path":"/items/-","value":{"name":"draft"}},{"op":"replace","path":"/items/0","value":{"name":"final"}}`,
	} {
		calls, validations := 0, 0
		snapshot := json.RawMessage(`{"items":[]}`)
		result, err := Interpret(context.Background(), Input{
			Snapshot: snapshot, Prompt: "Add the final item.",
			Writable: func(string) bool { return true },
			Validate: func(before, after json.RawMessage) error { validations++; return nil },
		}, func(context.Context, string) (string, error) {
			calls++
			return `{"patch":[` + operations + `]}`, nil
		})
		if err == nil || calls != 1 || validations != 0 || len(result.Patch) != 0 || string(snapshot) != `{"items":[]}` {
			t.Fatalf("accepted nonminimal append: result=%+v error=%v calls=%d validations=%d snapshot=%s", result, err, calls, validations, snapshot)
		}
	}
}
