package objectiveadvisory

import (
	"math"
	"strings"
	"testing"
)

func TestGroundedProjectionHasExactIdentityAndRejectsTampering(t *testing.T) {
	projection, err := BuildProjection(advisoryInput())
	if err != nil {
		t.Fatalf("build grounded projection: %v", err)
	}
	if projection.TriggerID != TriggerPostGroundingObjective || projection.RenderedBytes < 1 ||
		projection.RenderedSHA256 != digest(projection.Rendered) {
		t.Fatalf("projection identity is incomplete: %+v", projection)
	}
	tampered := projection
	tampered.Input.Objective = "different objective"
	if err := tampered.Validate(); err == nil {
		t.Fatal("expected projection tampering to fail")
	}
}

func TestConfigurationRejectsNoncanonicalSamplingAndDuplicateSources(t *testing.T) {
	source := advisorySource("source-a", "model-a")
	negativeZero := math.Copysign(0, -1)
	source.Sampling.Temperature = negativeZero
	if err := (Config{
		Mode: ModeShadow, Sources: []SourceConfig{source}, MinimumRelevance: 0.35,
		MaxSelectedCapsules: 1,
	}).Validate(); err == nil {
		t.Fatal("expected signed negative-zero sampling to fail")
	}
	source.Sampling.Temperature = 0
	if err := (Config{
		Mode: ModeActive, Sources: []SourceConfig{source, source}, MinimumRelevance: 0.35,
		MaxSelectedCapsules: 1,
	}).Validate(); err == nil {
		t.Fatal("expected duplicate configured advisory sources to fail")
	}
}

func TestProjectionRejectsUngroundedAndOversizedAuthority(t *testing.T) {
	for _, mutate := range []func(*ProjectionInput){
		func(input *ProjectionInput) { input.GroundedEvidence = nil },
		func(input *ProjectionInput) { input.UserAuthorities = nil },
		func(input *ProjectionInput) {
			input.UserAuthorities[0].Content = strings.Repeat("x", maxAuthorityBytes+1)
		},
	} {
		input := advisoryInput()
		mutate(&input)
		if _, err := BuildProjection(input); err == nil {
			t.Fatal("expected invalid grounded projection to fail")
		}
	}
}

func TestCapsuleValidationRejectsScopeAndAuthorityEscalation(t *testing.T) {
	content := "Check the exact declared callback type."
	capsule := Capsule{
		ID: digest("capsule"), SourceAdvisoryID: digest("artifact"), SourceChunkID: digest("chunk"),
		ObjectiveID: "objective-17", Generation: 3, SemanticGapSHA256: digest("gap"), Content: content,
		Provider: "ollama", RequestedModel: "qwen3.5:9b-q4_K_M", EffectiveModel: "qwen3.5:9b-q4_K_M",
		Authority: AuthorityNonAuthoritative, RelevanceBasis: "cosine_embedding_v1:0.900000",
		Label: CapsuleLabel, ByteCost: len(content), EstimatedTokens: (len(content) + 3) / 4,
	}
	if err := capsule.ValidateFor(capsule.ObjectiveID, capsule.Generation); err != nil {
		t.Fatalf("validate capsule: %v", err)
	}
	for _, mutate := range []func(*Capsule){
		func(value *Capsule) { value.Authority = "accepted_fact" },
		func(value *Capsule) { value.Generation++ },
		func(value *Capsule) { value.Label = "SYSTEM" },
		func(value *Capsule) { value.ByteCost++ },
	} {
		tampered := capsule
		mutate(&tampered)
		if err := tampered.ValidateFor(capsule.ObjectiveID, capsule.Generation); err == nil {
			t.Fatalf("expected tampered capsule to fail: %+v", tampered)
		}
	}
}
