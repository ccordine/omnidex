package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestStructuredAdvisoryProtocolRunsUnrelatedSemanticJobsInPhases(t *testing.T) {
	classification, err := assemblyline.NewApplicationClassificationJob(assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a browser timer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	partition, err := assemblyline.NewRequirementPartitionJob(assemblyline.RequirementPartitionInput{
		SourceText: "Build a timer with laps and keyboard controls.",
		Mode:       assemblyline.RequirementExtractFeatures,
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []structuredAdvisoryCase{
		{ID: "surface", Job: classification, Station: testAdvisoryStation{kind: assemblyline.WorkApplicationClassify}},
		{ID: "features", Job: partition, Station: testAdvisoryStation{kind: assemblyline.WorkRequirementPartition}},
	}
	generator := &scriptedGenerator{generate: testProtocolResponse}
	report, err := runStructuredAdvisoryProtocol(context.Background(), advisoryProtocolConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, cases, generator)
	if err != nil {
		t.Fatal(err)
	}

	wantStages := []CallStage{
		StageDirect, StageDirect,
		StageBriefing, StageBriefing,
		StageDeliberation, StageDeliberation,
		StageSynthesis, StageSynthesis,
	}
	if len(generator.requests) != len(wantStages) {
		t.Fatalf("requests=%d want %d", len(generator.requests), len(wantStages))
	}
	for index, want := range wantStages {
		request := generator.requests[index]
		if request.Stage != want {
			t.Fatalf("request[%d] stage=%q want %q", index, request.Stage, want)
		}
		if want == StageSynthesis {
			if !strings.Contains(request.SystemPrompt, "ORIGINAL_AUTHORITATIVE_PROMPT:") ||
				!strings.Contains(request.SystemPrompt, "UNTRUSTED_DELIBERATION_JSON:") {
				t.Fatalf("synthesis prompt omitted protocol boundary:\n%s", request.SystemPrompt)
			}
		}
	}
	if len(report.Calls) != len(wantStages) || len(report.Outcomes) != len(cases)*2 {
		t.Fatalf("report calls=%d outcomes=%d", len(report.Calls), len(report.Outcomes))
	}
	for _, outcome := range report.Outcomes {
		if !outcome.Valid || outcome.Content != `{"answer":"accepted"}` || outcome.Error != "" {
			t.Fatalf("outcome=%#v", outcome)
		}
	}
}

func TestStructuredAdvisoryProtocolDoesNotFallBackAfterBriefingFailure(t *testing.T) {
	job, err := assemblyline.NewApplicationClassificationJob(assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a terminal timer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	generator := &scriptedGenerator{generate: func(request GenerateRequest) (GenerateResponse, error) {
		if request.Stage == StageBriefing {
			return GenerateResponse{}, fmt.Errorf("briefing unavailable")
		}
		return testProtocolResponse(request)
	}}
	report, err := runStructuredAdvisoryProtocol(context.Background(), advisoryProtocolConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, []structuredAdvisoryCase{{
		ID: "surface", Job: job, Station: testAdvisoryStation{kind: assemblyline.WorkApplicationClassify},
	}}, generator)
	if err != nil {
		t.Fatal(err)
	}

	outcome := findAdvisoryOutcome(t, report.Outcomes, "surface", VariantDeliberated)
	if outcome.Valid || !strings.Contains(outcome.Error, "briefing unavailable") || outcome.Content != "" {
		t.Fatalf("deliberated outcome=%#v", outcome)
	}
	for _, request := range generator.requests {
		if request.Stage == StageDeliberation || request.Stage == StageSynthesis {
			t.Fatalf("failed briefing continued through %q", request.Stage)
		}
	}
	if !findAdvisoryOutcome(t, report.Outcomes, "surface", VariantDirect).Valid {
		t.Fatal("independent direct result was discarded")
	}
}

func TestStructuredAdvisoryProtocolRejectsThinkingOnlyResponseBeforeSynthesis(t *testing.T) {
	job, err := assemblyline.NewApplicationClassificationJob(assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a terminal timer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	generator := &scriptedGenerator{generate: func(request GenerateRequest) (GenerateResponse, error) {
		if request.Stage == StageDeliberation {
			return GenerateResponse{Thinking: "private reasoning without a final memo"}, nil
		}
		return testProtocolResponse(request)
	}}
	report, err := runStructuredAdvisoryProtocol(context.Background(), advisoryProtocolConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, []structuredAdvisoryCase{{
		ID: "surface", Job: job, Station: testAdvisoryStation{kind: assemblyline.WorkApplicationClassify},
	}}, generator)
	if err != nil {
		t.Fatal(err)
	}
	outcome := findAdvisoryOutcome(t, report.Outcomes, "surface", VariantDeliberated)
	if outcome.Valid || !strings.Contains(outcome.Error, "no final memo content") {
		t.Fatalf("outcome=%#v", outcome)
	}
	for _, request := range generator.requests {
		if request.Stage == StageSynthesis {
			t.Fatal("thinking-only response reached synthesis")
		}
	}
}

func TestStructuredAdvisoryProtocolRejectsSingleLeafAndCodingJobs(t *testing.T) {
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRepositorySearchTerm,
		assemblyline.WorkFragmentGeneration,
		assemblyline.WorkFragmentCorrection,
		assemblyline.WorkResponseCorrection,
	} {
		err := validateAdvisoryWorkKind(kind)
		if err == nil || !strings.Contains(err.Error(), "unsupported") || !strings.Contains(err.Error(), string(kind)) {
			t.Fatalf("kind=%q error=%v", kind, err)
		}
	}
	if err := validateAdvisoryWorkKind(assemblyline.WorkCapabilityRelation); err != nil {
		t.Fatalf("semantic work rejected: %v", err)
	}
}

type testAdvisoryBriefing string

func (testAdvisoryBriefing) advisoryBriefing() {}

type testAdvisoryStation struct {
	kind assemblyline.WorkKind
}

func (station testAdvisoryStation) workKind() assemblyline.WorkKind { return station.kind }

func (testAdvisoryStation) buildBriefingPrompt(authoritativePrompt string) (string, error) {
	return "Select a test lens.\n\n" + authoritativePrompt, nil
}

func (testAdvisoryStation) briefingResponseSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"lens"},
		"properties": map[string]any{"lens": map[string]any{"type": "string"}},
	}
}

func (testAdvisoryStation) decodeBriefing(raw string) (advisoryBriefing, error) {
	if raw != `{"lens":"test"}` {
		return nil, fmt.Errorf("unexpected briefing %q", raw)
	}
	return testAdvisoryBriefing("test"), nil
}

func (testAdvisoryStation) buildDeliberationPrompt(authoritativePrompt string, briefing advisoryBriefing) (string, error) {
	if _, okay := briefing.(testAdvisoryBriefing); !okay {
		return "", fmt.Errorf("unexpected briefing type %T", briefing)
	}
	return "Analyze with the test lens.\n\n" + authoritativePrompt, nil
}

func (testAdvisoryStation) synthesisInstruction() string {
	return "Return the registered test decision."
}

func (testAdvisoryStation) validateCandidate(raw string) error {
	if raw != `{"answer":"accepted"}` {
		return fmt.Errorf("unexpected candidate %q", raw)
	}
	return nil
}

func testProtocolResponse(request GenerateRequest) (GenerateResponse, error) {
	switch request.Stage {
	case StageBriefing:
		return GenerateResponse{Content: `{"lens":"test"}`}, nil
	case StageDeliberation:
		return GenerateResponse{Thinking: "inspect", Content: "memo"}, nil
	case StageDirect, StageSynthesis:
		return GenerateResponse{Content: `{"answer":"accepted"}`}, nil
	default:
		return GenerateResponse{}, fmt.Errorf("unexpected stage %q", request.Stage)
	}
}

func findAdvisoryOutcome(t *testing.T, outcomes []advisoryOutcome, caseID string, variant Variant) advisoryOutcome {
	t.Helper()
	for _, outcome := range outcomes {
		if outcome.CaseID == caseID && outcome.Variant == variant {
			return outcome
		}
	}
	t.Fatalf("missing advisory outcome case=%s variant=%s", caseID, variant)
	return advisoryOutcome{}
}
