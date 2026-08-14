package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestReviewedChannelsJobRendersBothLeavesWithinTheExactProviderBudget(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "browser music studio",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "channels"},
			{ID: "requirement_002", SourceQuote: "drum pads"},
			{ID: "requirement_003", SourceQuote: "keyboard"},
		},
	}
	jobSpecifications := []assemblyline.ApplicationJobSpecification{
		{
			Objective: "Implement the channels feature in the browser music studio to manage and control multiple audio streams.",
			RequiredBehaviors: []string{
				"Display a list of audio channels with volume, mute, and solo controls.",
				"Assign specific audio sources or instruments to each channel.",
				"Adjust the pan position for each channel.",
				"Provide a visual representation of audio levels for each channel.",
			},
			AcceptanceCriteria: []string{
				"The browser music studio displays a list of at least 8 audio channels, each with a volume slider (0-100%), a mute toggle, and a solo toggle.",
				"Each channel in the browser music studio can be assigned a specific audio source or instrument, with a dropdown menu showing at least 5 available options.",
				"The browser music studio allows users to adjust the pan position for each channel using a slider ranging from -100% (left) to +100% (right).",
				"The browser music studio provides a real-time visual representation of audio levels for each channel, updating at least 30 times per second.",
			},
		},
		workerJobSpecificationForRequirement("groups records"),
		workerJobSpecificationForRequirement("filters records"),
	}
	input := applicationWorkloadInput(specification)
	draft, err := assemblyline.MaterializeApplicationWorkloadDraft(input, jobSpecifications)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := assemblyline.FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		"requirement_001": {
			{RequirementID: "requirement_002", CapabilityID: "capability_002", Purpose: "drum pads"},
			{RequirementID: "requirement_003", CapabilityID: "capability_003", Purpose: "keyboard"},
		},
		"requirement_002": nil,
		"requirement_003": nil,
	}
	program, err := compileDirectCodingProgram(
		"music-studio", specification, nil, map[string]directCodingSkillBinding{}, frozen, capabilities,
	)
	if err != nil {
		t.Fatal(err)
	}
	context, err := assemblyline.ProjectApplicationTaskContext(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}

	feature, exists := directCodingTypeScriptBlueprintBlock(stage.TypeScript, "feature.001")
	if !exists {
		t.Fatal("feature.001 is missing")
	}
	featurePrompt := renderLiveJobFragmentPrompt(t, &stage, feature)
	if got, want := len(featurePrompt), 3106; got != want {
		t.Fatalf("feature prompt=%dB want exact live packet=%dB", got, want)
	}
	rawFeatureInput, err := llm.ExactPreparedModelInput(featurePrompt, llm.MinimalGeneratePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rawFeatureInput), 3140; got != want {
		t.Fatalf("feature raw input=%dB want exact live boundary=%dB", got, want)
	}
	fragmentJob, err := directCodingApplicationTaskFragmentJob(&stage, feature)
	if err != nil {
		t.Fatal(err)
	}
	fragmentJob.current = strings.Repeat("x", 4521)
	fragmentJob.failure = "CORRECTION_REJECTION: TypeScript syntax rejected: ERROR at line 100 column 2"
	correctionJob, err := newDirectCodingTypeScriptPortableJob(fragmentJob)
	if err != nil {
		t.Fatal(err)
	}
	correctionPrompt, _, err := assemblyline.RenderPortableJob(correctionJob)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(correctionPrompt), 5895; got != want {
		t.Fatalf("correction prompt=%dB want exact live boundary=%dB", got, want)
	}
	correctionContract, err := llmResponseContractForPortableJob(correctionJob, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExactStationStaticCall(
		correctionPrompt, nil, correctionContract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b-q4_K_M", NativeContextLimit: 8192},
	); err != nil {
		t.Fatalf("measured correction packet must fit the retained 8K context: %v", err)
	}
	stage.Generated[feature.ID] = feature.Signature +
		` { return <section aria-label="Channels">Channels</section>; }`
	acceptance, exists := directCodingTypeScriptBlueprintBlock(stage.TypeScript, "acceptance.001")
	if !exists {
		t.Fatal("acceptance.001 is missing")
	}
	acceptancePrompt := renderLiveJobFragmentPrompt(t, &stage, acceptance)
	t.Logf(
		"live reviewed job prompt bytes: feature=%d raw_feature=%d acceptance=%d",
		len(featurePrompt), len(rawFeatureInput), len(acceptancePrompt),
	)

	contract, err := llmResponseContractForScope("portable_fragment_worker")
	if err != nil {
		t.Fatal(err)
	}
	selection := llm.ProviderIdentitySelection{
		Model: "qwen3.5:9b-q4_K_M", NativeContextLimit: 8192,
	}
	for label, prompt := range map[string]string{
		"feature": featurePrompt, "acceptance": acceptancePrompt,
	} {
		if err := validateExactStationStaticCall(prompt, nil, contract, selection); err != nil {
			t.Fatalf("%s prompt=%dB does not fit final exact provider budget: %v", label, len(prompt), err)
		}
		for _, required := range []string{
			specification.ProductQuote, "channels", jobSpecifications[0].Objective,
			jobSpecifications[0].RequiredBehaviors[3], jobSpecifications[0].AcceptanceCriteria[3],
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("%s prompt omitted reviewed authority %q", label, required)
			}
		}
	}
}

func renderLiveJobFragmentPrompt(
	t *testing.T,
	stage *directCodingProgram,
	block assemblyline.TypeScriptBlock,
) string {
	t.Helper()
	fragment, err := directCodingApplicationTaskFragmentJob(stage, block)
	if err != nil {
		t.Fatal(err)
	}
	job, err := newDirectCodingTypeScriptPortableJob(fragment)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}
