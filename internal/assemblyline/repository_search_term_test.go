package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRepositorySearchTermStationReturnsOneBoundedLeaf(t *testing.T) {
	t.Parallel()
	input := RepositorySearchTermInput{UnresolvedConcept: "Invitation delivery timing implementation"}
	decision := RepositorySearchTermDecision{
		Schema: RepositorySearchTermSchemaV1,
		Term:   "dispatch interval",
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}

	decisionType := reflect.TypeOf(decision)
	if decisionType.NumField() != 2 {
		t.Fatalf("decision exposes %d fields, want exactly schema and term", decisionType.NumField())
	}
	for index, want := range []string{"schema", "term"} {
		if got := strings.Split(decisionType.Field(index).Tag.Get("json"), ",")[0]; got != want {
			t.Fatalf("decision field %d JSON name=%q want %q", index, got, want)
		}
	}

	schema := RepositorySearchTermResponseSchema()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Required             []string                  `json:"required"`
		Properties           map[string]map[string]any `json:"properties"`
		AdditionalProperties bool                      `json:"additionalProperties"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AdditionalProperties || !reflect.DeepEqual(decoded.Required, []string{"schema", "term"}) {
		t.Fatalf("response schema boundary=%s", encoded)
	}
	if len(decoded.Properties) != 2 || decoded.Properties["schema"] == nil || decoded.Properties["term"] == nil {
		t.Fatalf("response schema properties=%v", decoded.Properties)
	}
}

func TestRepositorySearchTermRejectsMalformedInputAndOutput(t *testing.T) {
	t.Parallel()
	invalidUTF8 := string([]byte{0xff})
	if utf8.ValidString(invalidUTF8) {
		t.Fatal("test fixture unexpectedly contains valid UTF-8")
	}
	for name, concept := range map[string]string{
		"empty":        "",
		"whitespace":   "   \n",
		"nul":          "delivery\x00timing",
		"invalid_utf8": invalidUTF8,
		"oversized":    strings.Repeat("x", maxRepositorySearchConceptBytes+1),
	} {
		t.Run("input_"+name, func(t *testing.T) {
			if _, err := NewRepositorySearchTermJob(RepositorySearchTermInput{UnresolvedConcept: concept}); err == nil {
				t.Fatalf("malformed concept %q was accepted", name)
			}
		})
	}
	exact := "  explain the owner  \n"
	job, err := NewRepositorySearchTermJob(RepositorySearchTermInput{UnresolvedConcept: exact})
	if err != nil {
		t.Fatalf("exact untrimmed user authority was rejected: %v", err)
	}
	prompt, _, err := RenderPortableJob(job)
	if err != nil || !strings.Contains(prompt, exact) {
		t.Fatalf("exact user authority was rewritten: prompt=%q error=%v", prompt, err)
	}

	input := RepositorySearchTermInput{UnresolvedConcept: "Find invitation timing behavior"}
	for name, decision := range map[string]RepositorySearchTermDecision{
		"schema":       {Schema: "wrong", Term: "dispatch"},
		"empty":        {Schema: RepositorySearchTermSchemaV1},
		"whitespace":   {Schema: RepositorySearchTermSchemaV1, Term: " dispatch "},
		"nul":          {Schema: RepositorySearchTermSchemaV1, Term: "dis\x00patch"},
		"invalid_utf8": {Schema: RepositorySearchTermSchemaV1, Term: invalidUTF8},
		"oversized":    {Schema: RepositorySearchTermSchemaV1, Term: strings.Repeat("x", maxRepositorySearchTermBytes+1)},
	} {
		t.Run("output_"+name, func(t *testing.T) {
			if err := decision.ValidateFor(input); err == nil {
				t.Fatalf("malformed decision %q was accepted", name)
			}
		})
	}
}

func TestRepositorySearchTermPortableJobRendersOnlyTheNamedGap(t *testing.T) {
	t.Parallel()
	job, err := NewRepositorySearchTermJob(RepositorySearchTermInput{
		UnresolvedConcept: "Invitation delivery timing implementation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositorySearchTerm {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Invitation delivery timing implementation") {
		t.Fatalf("prompt omitted unresolved concept: %q", prompt)
	}
	if len(schema) == 0 {
		t.Fatal("search-term station has no output schema")
	}
	for _, forbidden := range []string{"registered repository retrieval operation", "tool catalog", "action arguments", "query routing"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("prompt exposes forbidden model authority %q: %q", forbidden, prompt)
		}
	}
}

func TestRejectedRepositoryRetrievalWorkKindsAreUnsupported(t *testing.T) {
	t.Parallel()
	for _, kind := range []WorkKind{
		"repository_retrieval",
		"repository_retrieval_briefing",
		"repository_retrieval_advisory",
		"repository_retrieval_synthesis",
	} {
		if validWorkKind(kind) {
			t.Fatalf("rejected model-owned retrieval kind %q remains supported", kind)
		}
	}
}
