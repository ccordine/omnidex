package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const browserAudioSamplesCapabilityID = "runtime.browser.audio_samples"

func TestBrowserAudioSampleCapabilityIsReusableAcrossUnrelatedProducts(t *testing.T) {
	fixtures := []struct {
		name    string
		product string
		need    string
	}{
		{
			name:    "building safety console",
			product: "building evacuation console",
			need:    "Play an audible evacuation warning when the operator activates the alarm test.",
		},
		{
			name:    "pronunciation exercise",
			product: "language pronunciation exercise",
			need:    "Play the submitted pronunciation sample when the learner activates playback.",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			program := browserHostCapabilityFixtureProgram(
				t, fixture.product, []string{fixture.need},
			)
			bound, err := bindDirectCodingBrowserHostCapabilities(
				program,
				directCodingRuntimeCapabilityGraph{
					"requirement_001": {browserAudioSamplesCapabilityID},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			implementation := browserHostCapabilityImplementationBlock(
				t, bound.Source, "requirement_001",
			)
			available := browserHostCapabilityAvailableDeclarations(t, bound.Source, implementation)
			if !strings.Contains(available, "playBrowserAudioSamples") ||
				strings.Contains(available, "AudioContext") ||
				strings.Contains(available, fixture.product) ||
				strings.Contains(available, fixture.need) {
				t.Fatalf("model-visible host API is not task-neutral and minimal:\n%s", available)
			}
		})
	}

	joined := strings.ToLower(strings.Join([]string{
		directCodingBrowserHostCapabilityRegistry[0].Purpose,
		directCodingBrowserHostCapabilityRegistry[0].API,
		directCodingBrowserHostCapabilityRegistry[0].Source,
		directCodingBrowserHostCapabilityRegistry[0].Driver,
	}, "\n"))
	for _, forbidden := range []string{"music", "tone", "sequencer", "instrument", "pronunciation", "evacuation"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("browser host capability contains product-domain term %q", forbidden)
		}
	}
}

func TestBrowserHostCapabilityProjectionIsPerRequirementAndCallOnly(t *testing.T) {
	program := browserHostCapabilityFixtureProgram(t, "operations dashboard", []string{
		"Play an audible warning when a critical reading is acknowledged.",
		"Remember the selected temperature scale after the page is reopened.",
	})
	bound, err := bindDirectCodingBrowserHostCapabilities(
		program,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": {browserAudioSamplesCapabilityID},
			"requirement_002": nil,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := browserHostCapabilityImplementationBlock(t, bound.Source, "requirement_001")
	unselected := browserHostCapabilityImplementationBlock(t, bound.Source, "requirement_002")
	selectedAvailable := browserHostCapabilityAvailableDeclarations(t, bound.Source, selected)
	unselectedAvailable := browserHostCapabilityAvailableDeclarations(t, bound.Source, unselected)
	if !strings.Contains(selectedAvailable, "playBrowserAudioSamples") ||
		strings.Contains(unselectedAvailable, "playBrowserAudioSamples") {
		t.Fatalf(
			"host API escaped task scope: selected=%q unselected=%q",
			selectedAvailable, unselectedAvailable,
		)
	}
	if len(selected.Policy.RequiredCalls) != 1 ||
		len(selected.Policy.RequiredCalls[0].Callees) != 1 ||
		selected.Policy.RequiredCalls[0].Callees[0] != "playBrowserAudioSamples" ||
		len(unselected.Policy.RequiredCalls) != 0 {
		t.Fatalf("host capability call policy is not candidate-bound: %+v / %+v", selected.Policy, unselected.Policy)
	}

	selectedCalls, err := directCodingBrowserHostCallsForBlock(selected)
	if err != nil {
		t.Fatal(err)
	}
	selectedSource := `function Feature001View({ state, capabilities, actions }: Feature001ViewProps): ReactElement {
  return <button type="button" onClick={() => playBrowserAudioSamples([0.25, -0.25], 8000)}>Play warning</button>;
}`
	if _, err := assemblyline.ParseTypeScriptFunction(
		assemblyline.TypeScriptFunctionContract{
			Signature: selected.Signature, TSX: true, Policy: selected.Policy,
		},
		selectedSource,
	); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
		selectedSource, selectedCalls,
	); err != nil {
		t.Fatalf("selected host call was rejected: %v", err)
	}
	if err := validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
		selectedSource, nil,
	); err == nil || !strings.Contains(err.Error(), "undeclared runtime identifier playBrowserAudioSamples") {
		t.Fatalf("unselected host call escaped task scope: %v", err)
	}
	rawHostSource := `function Feature001View({ state, capabilities, actions }: Feature001ViewProps): ReactElement {
  const context = new AudioContext();
  return <button type="button" onClick={() => playBrowserAudioSamples([0.25, -0.25], 8000)}>Play warning</button>;
}`
	if err := validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
		rawHostSource, selectedCalls,
	); err == nil || !strings.Contains(err.Error(), "runtime host authority identifier AudioContext") {
		t.Fatalf("selected wrapper exposed raw browser host authority: %v", err)
	}
	aliasedSource := `function Feature001View({ state, capabilities, actions }: Feature001ViewProps): ReactElement {
  const play = playBrowserAudioSamples;
  return <button type="button" onClick={() => play([0.25, -0.25], 8000)}>Play warning</button>;
}`
	if err := validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
		aliasedSource, selectedCalls,
	); err == nil || !strings.Contains(err.Error(), "registered runtime capability value escape") {
		t.Fatalf("selected wrapper could be aliased or escaped: %v", err)
	}
	eagerSource := `function Feature001View({ state, capabilities, actions }: Feature001ViewProps): ReactElement {
  playBrowserAudioSamples([0.25, -0.25], 8000);
  return <button type="button">Play warning</button>;
}`
	if err := validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
		eagerSource, selectedCalls,
	); err == nil || !strings.Contains(err.Error(), "inside one public event handler") {
		t.Fatalf("selected wrapper could execute eagerly before the host bridge mounted: %v", err)
	}
}

func TestBrowserHostCapabilityNoneAddsNoWrapperOrDriverAuthority(t *testing.T) {
	program := browserHostCapabilityFixtureProgram(t, "reading dashboard", []string{
		"Display the current sensor reading.",
	})
	bound, err := bindDirectCodingBrowserHostCapabilities(
		program,
		directCodingRuntimeCapabilityGraph{"requirement_001": nil},
	)
	if err != nil {
		t.Fatal(err)
	}
	implementation := browserHostCapabilityImplementationBlock(t, bound.Source, "requirement_001")
	available := browserHostCapabilityAvailableDeclarations(t, bound.Source, implementation)
	if strings.Contains(available, "playBrowserAudioSamples") ||
		len(implementation.Policy.RequiredCalls) != 0 {
		t.Fatalf("unselected browser host authority reached source model: %q %+v", available, implementation.Policy)
	}
	for _, document := range bound.Source.Documents {
		if strings.HasPrefix(document.ID, "acceptance_") &&
			strings.Contains(document.Preamble, "observeBrowserHostRequestReceipts") {
			t.Fatalf("unselected browser host receipt observer reached %s", document.ID)
		}
		if document.ID != "application_runtime" {
			continue
		}
		for _, block := range document.Blocks {
			if block.ID == browserAudioSamplesCapabilityID {
				t.Fatalf("unselected browser host source block %s was emitted", block.ID)
			}
		}
	}
}

func TestBrowserAudioHostUsesProductionDriverAndCodeOwnedEffectReceipt(t *testing.T) {
	program := browserHostCapabilityFixtureProgram(t, "audible status panel", []string{
		"Play an audible status signal when the user activates playback.",
	})
	bound, err := bindDirectCodingBrowserHostCapabilities(
		program,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": {browserAudioSamplesCapabilityID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var runtime, app assemblyline.SourceDocument
	for _, document := range bound.Source.Documents {
		switch document.ID {
		case "application_runtime":
			runtime = document
		case "application_shell":
			app = document
		}
	}
	if runtime.ID == "" || app.ID == "" {
		t.Fatal("bound browser program omitted its runtime or application shell")
	}
	runtimeSource := strings.Builder{}
	for _, block := range runtime.Blocks {
		runtimeSource.WriteString(block.Static)
		runtimeSource.WriteByte('\n')
	}
	for _, required := range []string{
		"function publishBrowserHostRequest", "function BrowserHostBridge",
		"function observeBrowserHostRequestReceipts",
		"Browser host request has no mounted bridge",
		`registerBrowserHostHandler("runtime.browser.audio_samples"`, "new AudioContext",
		"output.connect(context.destination)", "output.start()",
	} {
		if !strings.Contains(runtimeSource.String(), required) {
			t.Fatalf("browser production host path omits %q", required)
		}
	}
	if !strings.Contains(app.Preamble, "BrowserHostBridge") ||
		!strings.Contains(app.Blocks[0].Static, "<BrowserHostBridge />") {
		t.Fatal("assembled application does not mount the code-owned browser host bridge")
	}
	if _, err := assemblyline.ComposeTypeScriptDocument(
		runtime,
		assemblyline.SourceComposition{Generated: map[string]string{}, Interfaces: map[string]string{}},
	); err != nil {
		t.Fatalf("compose selected browser host runtime: %v", err)
	}
	bindDirectCodingTestRequirementRelations(t, &bound)
	context, err := assemblyline.ProjectApplicationTaskContext(bound.Workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(bound, context)
	if err != nil {
		t.Fatal(err)
	}
	stageIDs := make(map[string]struct{})
	stageAcceptancePreamble := ""
	stageHarnessSource := ""
	stageHostCapabilitySource := ""
	for _, document := range stage.Source.Documents {
		if strings.HasPrefix(document.ID, "acceptance_") {
			stageAcceptancePreamble = document.Preamble
		}
		for _, block := range document.Blocks {
			stageIDs[block.ID] = struct{}{}
			if block.ID == "acceptance.harness.001" {
				stageHarnessSource = block.Static
			}
			if block.ID == browserAudioSamplesCapabilityID {
				stageHostCapabilitySource = block.Static
			}
		}
	}
	for _, required := range []string{"runtime.host_bridge", browserAudioSamplesCapabilityID} {
		if _, exists := stageIDs[required]; !exists {
			t.Fatalf("isolated host-capability stage omits %s", required)
		}
	}
	if !strings.Contains(stageHostCapabilitySource, "new AudioContext") ||
		!strings.Contains(stageHostCapabilitySource, "playBrowserAudioSamples") {
		t.Fatal("isolated stage did not typecheck the selected code-owned wrapper and real driver together")
	}
	if !strings.Contains(stageAcceptancePreamble, "observeBrowserHostRequestReceipts") ||
		!strings.Contains(stageHarnessSource, "observeBrowserHostRequestReceipts") ||
		!strings.Contains(stageHarnessSource, browserAudioSamplesCapabilityID) ||
		!strings.Contains(stageHarnessSource, "Expected browser host request was not dispatched") {
		t.Fatalf(
			"isolated acceptance lacks the code-owned host-effect receipt:\npreamble=%s\nharness=%s",
			stageAcceptancePreamble, stageHarnessSource,
		)
	}

	for _, document := range bound.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			for _, identifier := range block.Policy.ForbiddenIdentifiers {
				if identifier == "screen" {
					t.Fatalf("acceptance block %s forbids its code-bound Testing Library screen", block.ID)
				}
			}
			forbidden := strings.Join(block.Policy.ForbiddenIdentifiers, "\n")
			for _, identifier := range []string{
				"AudioContext", "OfflineAudioContext", "Audio", "playBrowserAudioSamples",
				"observeBrowserHostRequestReceipts",
			} {
				if !strings.Contains(forbidden, identifier) {
					t.Fatalf("acceptance block %s can invoke host identifier %s", block.ID, identifier)
				}
			}
		}
	}
}

func browserHostCapabilityFixtureProgram(
	t *testing.T,
	product string,
	needs []string,
) directCodingProgram {
	t.Helper()
	requirements := make([]assemblyline.Requirement, len(needs))
	capabilities := make(directCodingCapabilityGraph, len(needs))
	for index, need := range needs {
		id := fmt.Sprintf("requirement_%03d", index+1)
		requirements[index] = assemblyline.Requirement{ID: id, SourceQuote: need}
		capabilities[id] = nil
	}
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: product,
		Requirements: requirements,
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID:          genericTypeScriptBrowserAdapter,
		VersionProfileID: typeScriptBrowserVersionProfileV1,
		Paths:            []string{"src/Features.tsx", "src/Features.test.tsx"},
	}
	taskIDs := make([]string, len(workload.Tasks))
	for index, task := range workload.Tasks {
		taskIDs[index] = task.ID
	}
	coverage := assemblyline.ApplicationFileCoveragePlan{
		WorkloadSHA256: workload.SHA256,
		Files: []assemblyline.ApplicationFileCoverage{
			{
				Path: target.Paths[0], Kind: assemblyline.TargetArtifactImplementation,
				TaskIDs: append([]string(nil), taskIDs...),
			},
			{
				Path: target.Paths[1], Kind: assemblyline.TargetArtifactVerification,
				TaskIDs: append([]string(nil), taskIDs...),
			},
		},
	}
	program, err := compileDirectCodingProgram(
		"browser-host-capability-fixture", specification, nil,
		map[string]directCodingSkillBinding{}, workload, capabilities, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func browserHostCapabilityImplementationBlock(
	t *testing.T,
	blueprint assemblyline.SourceBlueprint,
	requirementID string,
) assemblyline.SourceBlock {
	t.Helper()
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskImplementation {
				continue
			}
			for _, task := range []struct {
				id          string
				requirement string
			}{{"task_001", "requirement_001"}, {"task_002", "requirement_002"}} {
				if block.TaskID == task.id && task.requirement == requirementID {
					return block
				}
			}
		}
	}
	t.Fatalf("browser source omits implementation for %s", requirementID)
	return assemblyline.SourceBlock{}
}

func browserHostCapabilityAvailableDeclarations(
	t *testing.T,
	blueprint assemblyline.SourceBlueprint,
	block assemblyline.SourceBlock,
) string {
	t.Helper()
	declarations := make(map[string]string)
	for _, document := range blueprint.Documents {
		for _, candidate := range document.Blocks {
			declarations[candidate.ID] = candidate.API
		}
	}
	available, err := directCodingTypeScriptAvailableDeclarations(block, declarations)
	if err != nil {
		t.Fatal(err)
	}
	return available
}
