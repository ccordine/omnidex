package datasource

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestJobMetadataRoundTrip(t *testing.T) {
	raw, err := JobMetadata("ds-1", "Hospital DB", "How many appointments tomorrow?", "")
	if err != nil {
		t.Fatal(err)
	}
	if !IsJobMetadata(raw) {
		t.Fatal("expected data source job metadata")
	}
	sourceID, sourceName, question, err := ParseJobMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sourceID != "ds-1" || sourceName != "Hospital DB" || question != "How many appointments tomorrow?" {
		t.Fatalf("unexpected metadata fields: %#v %#v %#v", sourceID, sourceName, question)
	}
}

func TestIsJobMetadataRejectsOtherSources(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"source": "omni-scrum"})
	if IsJobMetadata(raw) {
		t.Fatal("scrum metadata should not match data source jobs")
	}
}

func TestPipelineConstant(t *testing.T) {
	if Pipeline() != model.PipelineDataQuery {
		t.Fatalf("pipeline = %q", Pipeline())
	}
}

func TestFormatJobResultIncludesAnswerAndSQL(t *testing.T) {
	summary, payload, err := FormatJobResult(QueryResult{
		Answer:  "42 appointments",
		SQL:     "SELECT COUNT(*) FROM appointments",
		Columns: []string{"count"},
		Rows:    []map[string]any{{"count": 42}},
		Count:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary == "" || payload == "" {
		t.Fatalf("summary=%q payload=%q", summary, payload)
	}
	parsed, err := ParseJobResult(payload)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Count != 1 || parsed.Answer != "42 appointments" {
		t.Fatalf("parsed=%#v", parsed)
	}
}
