package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateRefinementUsesOneBoundLeafAtATime(t *testing.T) {
	t.Parallel()
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Provide interactive drum pads and a playable keyboard.",
	}
	cardinalityPrompt, err := BuildApplicationRequirementCandidateCardinalityPrompt(
		cardinalityInput,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinality, err := DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput, ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	splitInput := ApplicationRequirementCandidateSplitInput{
		Candidate: cardinalityInput.Candidate, Cardinality: cardinality,
	}
	splitPrompt, err := BuildApplicationRequirementCandidateSplitPrompt(splitInput)
	if err != nil {
		t.Fatal(err)
	}
	split, err := DecodeApplicationRequirementCandidateSplitLeaf(
		splitInput, "Provide interactive drum pads.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if split != "Provide interactive drum pads." {
		t.Fatalf("split=%q", split)
	}
	if strings.Count(cardinalityPrompt, cardinalityInput.Candidate) != 1 ||
		strings.Contains(cardinalityPrompt, "ACCEPTED REQUIREMENTS") ||
		!strings.Contains(cardinalityPrompt, "A second required response meaning is a separate outcome") {
		t.Fatalf("cardinality prompt exceeded candidate-only authority:\n%s", cardinalityPrompt)
	}
	if strings.Count(splitPrompt, cardinalityInput.Candidate) != 1 ||
		strings.Count(
			splitPrompt,
			"CODE-ESTABLISHED CARDINALITY RELATION:\n"+
				ApplicationRequirementMultipleRuntimeOutcomes,
		) != 1 || strings.Contains(splitPrompt, "ACCEPTED REQUIREMENTS") {
		t.Fatalf("split prompt exceeded candidate-plus-cardinality authority:\n%s", splitPrompt)
	}
}

func TestApplicationRequirementCandidateRefinementRejectsUnboundState(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Provide interactive drum pads and a playable keyboard.",
	}
	multiple, err := DecodeApplicationRequirementCandidateCardinalityResult(
		input, ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ApplicationRequirementCandidateCardinalityResult){
		"schema": func(result *ApplicationRequirementCandidateCardinalityResult) {
			result.Schema = "invalid"
		},
		"hash": func(result *ApplicationRequirementCandidateCardinalityResult) {
			result.CandidateSHA256 = strings.Repeat("0", 64)
		},
		"relation": func(result *ApplicationRequirementCandidateCardinalityResult) {
			result.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := multiple
			mutate(&candidate)
			if _, err := NewApplicationRequirementCandidateSplitJob(
				ApplicationRequirementCandidateSplitInput{
					Candidate: input.Candidate, Cardinality: candidate,
				},
			); err == nil {
				t.Fatal("unbound cardinality opened a split job")
			}
		})
	}
	one, err := DecodeApplicationRequirementCandidateCardinalityResult(
		input, ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewApplicationRequirementCandidateSplitJob(
		ApplicationRequirementCandidateSplitInput{
			Candidate: input.Candidate, Cardinality: one,
		},
	); err == nil || !strings.Contains(err.Error(), ApplicationRequirementMultipleRuntimeOutcomes) {
		t.Fatalf("one-outcome split error=%v", err)
	}
	splitInput := ApplicationRequirementCandidateSplitInput{
		Candidate: input.Candidate, Cardinality: multiple,
	}
	unchanged, err := DecodeApplicationRequirementCandidateSplitLeaf(
		splitInput, input.Candidate,
	)
	if err != nil || unchanged != input.Candidate {
		t.Fatalf("preserve unchanged split=%q error=%v", unchanged, err)
	}
	correctionInput := ApplicationRequirementCandidateSplitCorrectionInput{
		CurrentCandidate: unchanged,
		Cardinality:      multiple,
		Defect:           ApplicationRequirementUnchangedSplitDefect,
	}
	if _, err := NewApplicationRequirementCandidateSplitCorrectionJob(correctionInput); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeApplicationRequirementCandidateSplitCorrectionLeaf(
		correctionInput, unchanged,
	); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("unchanged correction error=%v", err)
	}
	if corrected, err := DecodeApplicationRequirementCandidateSplitCorrectionLeaf(
		correctionInput, "Provide interactive drum pads.",
	); err != nil || corrected != "Provide interactive drum pads." {
		t.Fatalf("corrected split=%q error=%v", corrected, err)
	}
	correctionInput.Defect = "UNKNOWN"
	if _, err := NewApplicationRequirementCandidateSplitCorrectionJob(correctionInput); err == nil {
		t.Fatal("unregistered grounded defect opened a correction job")
	}
}

func TestApplicationRequirementRefinementKindsProduceValidPortableJobs(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Provide interactive drum pads and a playable keyboard.",
	}
	job, err := NewApplicationRequirementCandidateCardinalityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err != nil {
		t.Fatal(err)
	}
}
