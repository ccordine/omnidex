package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParserPartitionsEveryByteWithinTheLeafBound(t *testing.T) {
	for _, source := range []string{"  One statement.\n\nSecond statement.\n  ", strings.Repeat("日🦊\t", 1700), "```text\n" + strings.Repeat("x", 9000) + "\n```\n"} {
		blocks, err := PartitionParserSource(source)
		if err != nil {
			t.Fatal(err)
		}
		var recovered strings.Builder
		for _, block := range blocks {
			if block.Start != recovered.Len() || block.End != block.Start+len(block.Text) || len(block.Text) > ParserBlockBytes || !utf8.ValidString(block.Text) {
				t.Fatalf("invalid source block: %+v", block)
			}
			recovered.WriteString(block.Text)
		}
		if recovered.String() != source {
			t.Fatal("partition lost or changed final source")
		}
	}
	for _, source := range []string{"", " \n\t", string([]byte{255})} {
		if _, err := PartitionParserSource(source); err == nil {
			t.Fatal("invalid final source was accepted")
		}
	}
}

func TestParserDispositionsRejectLossDuplicationAndInvalidKinds(t *testing.T) {
	for _, parts := range [][]ParserPart{
		nil, {{Text: "first", Kind: "task", Reason: "one obligation"}},
		{{Text: "first\nsecondfirst\nsecond", Kind: "task", Reason: "duplicated"}},
		{{Text: "second\nfirst", Kind: "task", Reason: "reordered"}},
		{{Text: "first\nsecond", Kind: "task"}},
		{{Text: "first\nsecond", Kind: "approve", Reason: "invented control"}},
	} {
		if err := validateParserParts(PlanningInitial, "first\nsecond", parts); err == nil {
			t.Fatalf("unsafe dispositions accepted: %+v", parts)
		}
	}
}

func TestParserDispositionsIgnoreUnknownCommentary(t *testing.T) {
	parts, err := parserScanParts(PlanningInitial, "first\nsecond", parserScanResult{Units: map[string]parserUnitDisposition{
		"unit_1": {Kind: "unknown", Reason: "parser commentary", GroupStart: "unit_1"},
		"unit_2": {Kind: "task", Reason: "second obligation", GroupStart: "unit_2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Kind != "ignore" || parts[0].Text != "first\n" {
		t.Fatalf("unknown commentary was not ignored: %+v", parts)
	}
}

func TestParserSourceIDsOwnBytesAndRejectMissingOrInventedUnits(t *testing.T) {
	source := "One exact statement.\n\n"
	parts, err := parserScanParts(PlanningInitial, source, scanOracle(source, "task"))
	if err != nil || parts[0].Text != "One exact statement.\n" || parts[1].Text != "\n" {
		t.Fatalf("code lost original whitespace: %+v %v", parts, err)
	}
	for _, raw := range []string{
		`{"units":{}}`,
		`{"units":{"foreign":{"kind":"task","reason":"x","group_start":"foreign"}}}`,
		`{"units":{"unit_1":{"kind":"task","reason":"x","group_start":"future"}}}`,
		`{"units":{"unit_1":{"kind":"task","reason":"x","group_start":"unit_1"},"unit_1":{"kind":"ignore","reason":"overwrite","group_start":"unit_1"}}}`,
	} {
		call := Assignment{Role: PlanningInitial, ParserStep: &ParserStep{Stage: "scan", Source: source}}
		if _, err := decodeParserScan(call, json.RawMessage(raw)); err == nil {
			t.Fatalf("unsafe source identities accepted: %s", raw)
		}
	}
}

func sieveTestProvider(t *testing.T, resolve func(Assignment) any) func(context.Context, Assignment, func(NativeEvent) error) (Result, error) {
	t.Helper()
	return func(_ context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
		if call.ParserStep == nil || len(call.ParserStep.Source) > ParserBlockBytes {
			t.Fatal("whole response reached semantic leaf")
		}
		raw, err := json.Marshal(resolve(call))
		if err != nil {
			return Result{}, err
		}
		if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"semantic-leaf"}`)}); err != nil {
			return Result{}, err
		}
		return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, Content: raw}, nil
	}
}

func scanOracle(source, kind string) parserScanResult {
	result := parserScanResult{Units: map[string]parserUnitDisposition{}}
	group := ""
	for _, unit := range parserSourceUnits(source) {
		if unit.ID != "" {
			if group == "" {
				group = unit.ID
			}
			result.Units[unit.ID] = parserUnitDisposition{Kind: kind, Reason: "fixed semantic oracle", GroupStart: group}
		}
		if strings.Contains(unit.Text, "\n") {
			group = ""
		}
	}
	return result
}

func TestExecutionSieveCannotHideInputOrFailureBehindProgress(t *testing.T) {
	for _, kind := range []string{"question", "permission", "error"} {
		call := Assignment{Role: ExecutionResponse, Task: &Task{ID: "task", Goal: "Complete the requested work and required verification."}, Route: Route{Provider: "fixture", Model: "fixture"}, Response: "Completed some work.\n\nAn outstanding need remains."}
		provider := sieveTestProvider(t, func(call Assignment) any {
			state := "done"
			if call.ParserStep.BlockID == "block_2" {
				state = kind
			}
			return scanOracle(call.ParserStep.Source, state)
		})
		result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, provider)
		if err != nil || string(result.Content) != fmt.Sprintf(`{"kind":%q}`, kind) {
			t.Fatalf("outstanding %s was hidden: %s %v", kind, result.Content, err)
		}
	}
}

func TestPlanningParserGroupingCrossesBlankLinesWithinOwningTask(t *testing.T) {
	source := "One task.\n\nObjective: restore routing.\nInspect a.go and create a_test.go for routing checks.\nConstraint: preserve approved authority.\nAcceptance: passes.\nVerification: run routing checks.\n"
	scan := parserScanResult{Units: map[string]parserUnitDisposition{}}
	for _, unit := range parserSourceUnits(source) {
		if unit.ID != "" {
			scan.Units[unit.ID] = parserUnitDisposition{Kind: "task", Reason: "same task supporting detail", GroupStart: "unit_1"}
		}
	}
	parts, err := parserScanParts(PlanningInitial, source, scan)
	if err != nil || len(parts) != 1 || parts[0].Text != source {
		t.Fatalf("task supporting text was split: %+v %v", parts, err)
	}
}
