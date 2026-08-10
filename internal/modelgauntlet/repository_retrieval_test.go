package modelgauntlet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryRetrievalGauntletUsesFinalMemoOnly(t *testing.T) {
	t.Parallel()

	fixture := RepositoryRetrievalCase{ID: "references", Input: assemblyline.RepositoryRetrievalInput{
		ResearchNeed: "Find every direct reference to ApplyResponseCorrection before changing its contract.",
	}}
	generator := &scriptedGenerator{generate: func(request GenerateRequest) (GenerateResponse, error) {
		switch request.Stage {
		case StageBriefing:
			return GenerateResponse{Content: `{"schema":"omnidex.repository-retrieval-briefing.v2","lens":"relation_direction"}`}, nil
		case StageDeliberation:
			return GenerateResponse{Thinking: "private evidence-only chain", Content: "The need asks for incoming direct references."}, nil
		case StageDirect, StageSynthesis:
			raw, _ := json.Marshal(assemblyline.RepositoryRetrievalDecision{
				Schema:     assemblyline.RepositoryRetrievalSchemaV2,
				Operation:  assemblyline.RetrievalDirectReferences,
				QueryQuote: "ApplyResponseCorrection",
			})
			return GenerateResponse{Content: string(raw)}, nil
		default:
			return GenerateResponse{}, nil
		}
	}}
	report, err := RunRepositoryRetrieval(context.Background(), RepositoryRetrievalConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, []RepositoryRetrievalCase{fixture}, generator)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Calls) != 4 || len(report.Predictions) != 2 {
		t.Fatalf("calls=%d predictions=%d", len(report.Calls), len(report.Predictions))
	}
	synthesis := report.Calls[3].Request.SystemPrompt
	if !strings.Contains(synthesis, "UNTRUSTED_ADVISORY_MEMO_JSON") || strings.Contains(synthesis, "private evidence-only chain") {
		t.Fatalf("synthesis violated final-memo boundary:\n%s", synthesis)
	}
	for _, prediction := range report.Predictions {
		if !prediction.Valid || prediction.Operation != assemblyline.RetrievalDirectReferences {
			t.Fatalf("prediction=%#v", prediction)
		}
	}

	evaluation, err := EvaluateRepositoryRetrieval(report, []RepositoryRetrievalLabel{{
		CaseID: fixture.ID, Operation: assemblyline.RetrievalDirectReferences,
		QueryQuote: "ApplyResponseCorrection",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Scores[VariantDirect].Correct != 1 || evaluation.Scores[VariantDeliberated].Correct != 1 {
		t.Fatalf("scores=%#v", evaluation.Scores)
	}
}

func TestRepositoryRetrievalLabelsMustRemainGrounded(t *testing.T) {
	t.Parallel()

	inputs := map[string]assemblyline.RepositoryRetrievalInput{
		"declaration": {ResearchNeed: "Locate the declaration of ParseEnvelope."},
	}
	_, err := validateRepositoryRetrievalLabels(inputs, []RepositoryRetrievalLabel{{
		CaseID: "declaration", Operation: assemblyline.RetrievalSymbolDeclaration,
		QueryQuote: "missing symbol",
	}})
	if err == nil || !strings.Contains(err.Error(), "exact source") {
		t.Fatalf("error=%v", err)
	}
}
