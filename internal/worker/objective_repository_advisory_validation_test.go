package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

func TestRepositoryAdvisoryConsumerRejectsForgedScopeAuthorityAndAccounting(t *testing.T) {
	proof := advisoryProofCase{
		name: "report-validation", objectiveID: "objective-report-validation",
		requirement: "Which adapter satisfies the callback interface?",
		evidence: []assemblyline.GroundedEvidenceCapsule{{
			ID: "R01", Text: "CallbackFunc satisfies Callback through Apply.",
		}},
		incorrect: "A matching function is sufficient.", correct: "CallbackFunc is required.",
		advice:          "Check CallbackFunc at the interface conversion boundary.",
		relevanceNeedle: "callbackfunc", provider: "proof-validation-provider", model: "reasoner-validation",
	}
	provider := &advisoryProofProvider{raw: proof.advice, provider: proof.provider, model: proof.model}
	runtime := newAdvisoryProofRuntime(objectiveadvisory.ModeActive, proof, provider)
	valid := runAdvisoryProofClosure(t, proof, proof.correct, runtime).result.Advisory
	options := objectiveRepositoryGroundedClosureOptions{ObjectiveID: proof.objectiveID, Generation: 1}
	gap := objectiveadvisory.SemanticGap{
		ObjectiveID: proof.objectiveID, Generation: 1, Requirement: proof.requirement,
		Candidate: proof.correct, Evidence: objectiveRepositoryAdvisoryEvidence(proof.evidence),
	}
	if err := validateObjectiveRepositoryAdvisoryReport(
		valid, valid.Projection.Input, gap, runtime.Configuration(), options,
	); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*objectiveadvisory.Report){
		"projection identity": func(report *objectiveadvisory.Report) {
			report.Projection.RenderedBytes++
		},
		"projection scope": func(report *objectiveadvisory.Report) {
			input := report.Projection.Input
			input.ObjectiveID = "objective-other"
			projection, err := objectiveadvisory.BuildProjection(input)
			if err != nil {
				t.Fatal(err)
			}
			report.Projection = projection
		},
		"capsule authority": func(report *objectiveadvisory.Report) {
			report.ActiveCapsules[0].Authority = "fact"
		},
		"capsule count": func(report *objectiveadvisory.Report) {
			report.ActiveCapsules = append(report.ActiveCapsules, report.ActiveCapsules[0])
			report.Metrics.SelectedCapsules = len(report.ActiveCapsules)
			report.Metrics.SelectedCapsuleContentBytes *= 2
		},
		"selected count": func(report *objectiveadvisory.Report) {
			report.Metrics.SelectedCapsules = 0
		},
		"downstream bytes": func(report *objectiveadvisory.Report) {
			report.Metrics.SelectedCapsuleContentBytes++
		},
		"downstream tokens": func(report *objectiveadvisory.Report) {
			report.Metrics.SelectedCapsuleContentTokens++
		},
		"shadow active capsule": func(report *objectiveadvisory.Report) {
			report.Mode = objectiveadvisory.ModeShadow
		},
		"shadow downstream bytes": func(report *objectiveadvisory.Report) {
			report.Mode = objectiveadvisory.ModeShadow
			report.ActiveCapsules = []objectiveadvisory.Capsule{}
			report.Metrics.SelectedCapsules = 0
			report.Metrics.SelectedCapsuleContentBytes = 1
		},
		"shadow downstream tokens": func(report *objectiveadvisory.Report) {
			report.Mode = objectiveadvisory.ModeShadow
			report.ActiveCapsules = []objectiveadvisory.Capsule{}
			report.Metrics.SelectedCapsules = 0
			report.Metrics.SelectedCapsuleContentBytes = 0
			report.Metrics.SelectedCapsuleContentTokens = 1
		},
	}
	for name, mutate := range tests {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			report := valid
			report.ActiveCapsules = append([]objectiveadvisory.Capsule(nil), valid.ActiveCapsules...)
			mutate(&report)
			if err := validateObjectiveRepositoryAdvisoryReport(
				report, valid.Projection.Input, gap, runtime.Configuration(), options,
			); err == nil {
				t.Fatalf("forged report was accepted: %#v", report)
			}
		})
	}
}
