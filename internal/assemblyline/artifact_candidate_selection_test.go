package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestArtifactCandidateSelectionIsBoundedAndPathBlind(t *testing.T) {
	t.Parallel()
	input := ArtifactCandidateSelectionInput{
		RequirementQuote: "The known artifact declaring LegacyAdapter must no longer exist.",
		Candidates: []ArtifactCandidateEvidence{
			{
				CandidateID:  "ARTIFACT_CANDIDATE_1",
				Declarations: []string{"function LegacyAdapter: func LegacyAdapter() int"},
			},
			{
				CandidateID:  "ARTIFACT_CANDIDATE_2",
				Declarations: []string{"function CurrentAdapter: func CurrentAdapter() int"},
			},
		},
	}
	job, err := NewArtifactCandidateSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkArtifactCandidateSelection {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.RequirementQuote, "ARTIFACT_CANDIDATE_1", "ARTIFACT_CANDIDATE_2",
		"LegacyAdapter", ArtifactCandidateSelectionNone,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q: %s", required, prompt)
		}
	}
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		ArtifactCandidateSelectionSchemaV1, "candidate_id",
		"ARTIFACT_CANDIDATE_1", "ARTIFACT_CANDIDATE_2", ArtifactCandidateSelectionNone,
	} {
		if !strings.Contains(string(rawSchema), required) {
			t.Fatalf("schema omitted %q: %s", required, rawSchema)
		}
	}
	for _, forbidden := range []string{
		"legacy.go", "current.go", "path", "filename", "create_file", "delete_file",
		"write_file", "rename_file", "move_file", "shell command",
	} {
		if strings.Contains(strings.ToLower(prompt+string(rawSchema)), forbidden) {
			t.Fatalf("model envelope exposed forbidden repository authority %q", forbidden)
		}
	}
}

func TestArtifactCandidateSelectionAcceptsOnlyOneAvailableIDOrNone(t *testing.T) {
	t.Parallel()
	input := artifactCandidateSelectionFixture()
	for _, candidateID := range []string{"ARTIFACT_CANDIDATE_1", ArtifactCandidateSelectionNone} {
		decision := ArtifactCandidateSelectionDecision{
			Schema: ArtifactCandidateSelectionSchemaV1, CandidateID: candidateID,
		}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("candidate %q: %v", candidateID, err)
		}
	}
	for name, decision := range map[string]ArtifactCandidateSelectionDecision{
		"wrong schema": {Schema: "wrong", CandidateID: "ARTIFACT_CANDIDATE_1"},
		"unknown ID":   {Schema: ArtifactCandidateSelectionSchemaV1, CandidateID: "ARTIFACT_CANDIDATE_3"},
		"physical ID":  {Schema: ArtifactCandidateSelectionSchemaV1, CandidateID: "legacy.go"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decision.ValidateFor(input); err == nil {
				t.Fatalf("invalid decision accepted: %+v", decision)
			}
		})
	}
}

func TestArtifactCandidateSelectionRejectsUnsafeOrUnboundedEvidence(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ArtifactCandidateSelectionInput){
		"single candidate": func(input *ArtifactCandidateSelectionInput) {
			input.Candidates = input.Candidates[:1]
		},
		"duplicate candidate": func(input *ArtifactCandidateSelectionInput) {
			input.Candidates[1].CandidateID = input.Candidates[0].CandidateID
		},
		"noncanonical ID": func(input *ArtifactCandidateSelectionInput) {
			input.Candidates[0].CandidateID = "ARTIFACT_CANDIDATE_2"
		},
		"path in quote": func(input *ArtifactCandidateSelectionInput) {
			input.RequirementQuote = "Remove legacy.go or current.go."
		},
		"path in evidence": func(input *ArtifactCandidateSelectionInput) {
			input.Candidates[0].Declarations[0] = "function LegacyAdapter in legacy.go"
		},
		"too many declarations": func(input *ArtifactCandidateSelectionInput) {
			input.Candidates[0].Declarations = []string{"A", "B", "C", "D", "E"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := artifactCandidateSelectionFixture()
			mutate(&input)
			if _, err := NewArtifactCandidateSelectionJob(input); err == nil {
				t.Fatalf("unsafe candidate input accepted: %+v", input)
			}
		})
	}
}

func artifactCandidateSelectionFixture() ArtifactCandidateSelectionInput {
	return ArtifactCandidateSelectionInput{
		RequirementQuote: "The known artifact declaring LegacyAdapter must no longer exist.",
		Candidates: []ArtifactCandidateEvidence{
			{
				CandidateID:  "ARTIFACT_CANDIDATE_1",
				Declarations: []string{"function LegacyAdapter: func LegacyAdapter() int"},
			},
			{
				CandidateID:  "ARTIFACT_CANDIDATE_2",
				Declarations: []string{"function CurrentAdapter: func CurrentAdapter() int"},
			},
		},
	}
}
