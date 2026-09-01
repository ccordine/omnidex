package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserPublicSurfaceAndVerificationStayPathBlindAndMechanicallyGrounded(t *testing.T) {
	fixtures := []struct {
		name           string
		implementation string
		verification   string
		controlName    string
		outputName     string
	}{
		{
			name: "maintenance tracker",
			implementation: `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return (
    <div className="grid gap-2">
      <button type="button" onClick={() => actions.set('status', 'Scheduled')}>Schedule maintenance</button>
      <output aria-label="Maintenance status">{String(state.status ?? '')}</output>
    </div>
  );
}`,
			verification: `async function VerifyFeature001(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Schedule maintenance' }));
  expect(screen.getByRole('status', { name: 'Maintenance status' })).toHaveTextContent(/^Scheduled$/);
}`,
			controlName: "Schedule maintenance",
			outputName:  "Maintenance status",
		},
		{
			name: "text summarizer",
			implementation: `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return (
    <div className="grid gap-2">
      <textarea aria-label="Source text" value={String(state.summary ?? '')} onChange={(event) => actions.set('summary', event.currentTarget.value)} />
      <output aria-label="Summary">{String(state.summary ?? '')}</output>
    </div>
  );
}`,
			verification: `async function VerifyFeature001(): Promise<void> {
  fireEvent.change(screen.getByRole('textbox', { name: 'Source text' }), { target: { value: 'Condensed note' } });
  expect(screen.getByRole('status', { name: 'Summary' })).toHaveTextContent(/^Condensed note$/);
}`,
			controlName: "Source text",
			outputName:  "Summary",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			surface, err := extractDirectCodingBrowserPublicInteractionSurface(fixture.implementation)
			if err != nil {
				t.Fatalf("extract implementation public surface: %v", err)
			}
			if len(surface.Controls) != 1 || surface.Controls[0].AccessibleName != fixture.controlName {
				t.Fatalf("controls=%+v; want one exact accessible control", surface.Controls)
			}
			if len(surface.Outputs) != 1 || surface.Outputs[0].AccessibleName != fixture.outputName {
				t.Fatalf("outputs=%+v; want one exact named status output", surface.Outputs)
			}
			receipt, err := renderDirectCodingBrowserPublicInteractionSurface(surface)
			if err != nil {
				t.Fatalf("render portable public surface: %v", err)
			}
			if strings.Contains(receipt, "Scheduled") || strings.Contains(receipt, "Condensed note") {
				t.Fatalf("public receipt leaked expected result authority: %s", receipt)
			}
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				fixture.verification,
				true,
				surface,
				assemblyline.ApplicationRequirementExplicitResultRelation,
			); err != nil {
				t.Fatalf("validate grounded verifier: %v", err)
			}
		})
	}
}

func TestBrowserVerificationRejectsInventedSelectorsMissingOracleAndHostAuthority(t *testing.T) {
	implementation := `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return (
    <div>
      <button type="button" onClick={() => actions.set('status', 'Ready')}>Confirm item</button>
      <output aria-label="Item status">{String(state.status ?? '')}</output>
    </div>
  );
}`
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(implementation)
	if err != nil {
		t.Fatalf("extract fixture public surface: %v", err)
	}
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "invented selector",
			source: `async function VerifyFeature001(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Delete item' }));
  expect(screen.getByRole('status', { name: 'Item status' })).toHaveTextContent(/^Ready$/);
}`,
		},
		{
			name: "missing independently derived assertion",
			source: `async function VerifyFeature001(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Confirm item' }));
  expect(screen.getByRole('status', { name: 'Item status' })).toBeInTheDocument();
}`,
		},
		{
			name: "host and tool authority",
			source: `async function VerifyFeature001(): Promise<void> {
  fetch('https://invalid.example');
  expect(screen.getByRole('status', { name: 'Item status' })).toHaveTextContent(/^Ready$/);
}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				test.source,
				true,
				surface,
				assemblyline.ApplicationRequirementExplicitResultRelation,
			); err == nil {
				t.Fatal("invalid verifier unexpectedly crossed the code-owned AST boundary")
			}
		})
	}
}

func TestBrowserVerificationRejectsExplicitResultsWithoutGroundedInteraction(t *testing.T) {
	fixtures := []struct {
		name           string
		implementation string
		verification   string
	}{
		{
			name: "numeric transform",
			implementation: `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return <div><input type="number" aria-label="Input value" value={String(state.value ?? '')} onChange={(event) => actions.set('value', event.currentTarget.value)} /><output aria-label="Transformed value">{String(state.value ?? '')}</output></div>;
}`,
			verification: `async function VerifyFeature001(): Promise<void> {
  expect(screen.getByRole('status', { name: 'Transformed value' })).toHaveTextContent(/^4$/);
}`,
		},
		{
			name: "text transform",
			implementation: `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return <div><textarea aria-label="Source text" value={String(state.value ?? '')} onChange={(event) => actions.set('value', event.currentTarget.value)} /><output aria-label="Transformed text">{String(state.value ?? '')}</output></div>;
}`,
			verification: `async function VerifyFeature001(): Promise<void> {
  expect(screen.getByRole('status', { name: 'Transformed text' })).toHaveTextContent(/^RESULT$/);
}`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			surface, err := extractDirectCodingBrowserPublicInteractionSurface(fixture.implementation)
			if err != nil {
				t.Fatalf("extract fixture public surface: %v", err)
			}
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				fixture.verification,
				true,
				surface,
				assemblyline.ApplicationRequirementExplicitResultRelation,
			); err == nil {
				t.Fatal("zero-interaction explicit result unexpectedly received acceptance authority")
			}
		})
	}
}

func TestBrowserVerificationBindingRejectsImplementationSurfaceDrift(t *testing.T) {
	const taskID = "task_001"
	const implementationID = "feature.001"
	const verificationID = "acceptance.001"
	source := `function Feature001View({ state, actions }: Feature001ViewProps): ReactElement {
  return <div><button type="button" onClick={() => actions.set('status', 'Ready')}>Confirm item</button><output aria-label="Item status">{String(state.status ?? '')}</output></div>;
}`
	program := directCodingProgram{
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{
				ID: "feature", Path: "src/feature001.tsx",
				Blocks: []assemblyline.SourceBlock{{
					ID:        implementationID,
					Signature: "function Feature001View({ state, actions }: Feature001ViewProps): ReactElement",
					TaskID:    taskID,
					Role:      assemblyline.SourceBlockTaskImplementation,
				}},
			},
			{
				ID: "acceptance", Path: "src/feature001.test.tsx",
				Blocks: []assemblyline.SourceBlock{{
					ID:        verificationID,
					Signature: "async function VerifyFeature001(): Promise<void>",
					DependsOn: []string{implementationID},
					TaskID:    taskID,
					Role:      assemblyline.SourceBlockTaskVerification,
				}},
			},
		}},
		Generated: map[string]string{implementationID: source},
	}
	initial, err := deriveDirectCodingBrowserPublicSurfaceBinding(&program, taskID)
	if err != nil {
		t.Fatalf("derive initial binding: %v", err)
	}
	initial.verificationBlockID = verificationID
	program.Generated[implementationID] = strings.Replace(source, "Confirm item", "Approve item", 1)
	current, err := deriveDirectCodingBrowserPublicSurfaceBinding(&program, taskID)
	if err != nil {
		t.Fatalf("derive drifted binding: %v", err)
	}
	if err := validateDirectCodingBrowserFrozenPublicSurface(
		taskID,
		initial,
		current,
		verificationID,
		initial.resultRelation,
	); err == nil {
		t.Fatal("implementation surface drift unexpectedly retained verifier authority")
	}
}

func TestTypeScriptInitialCandidateValidatorsReachOnlyImplementationGeneration(t *testing.T) {
	validatorCalls := 0
	validator := func(string) error {
		validatorCalls++
		return nil
	}
	for _, role := range []assemblyline.SourceBlockRole{
		assemblyline.SourceBlockTaskImplementation,
	} {
		job := directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "generated.source", Signature: "function GeneratedSource(): void",
				Contract: "Return one complete declaration.", Role: role,
			},
			dialect: "TypeScript", validateInitialCandidate: validator,
		}
		if _, err := newDirectCodingTypeScriptPortableJob(job); err != nil {
			t.Fatalf("%s validator did not reach fragment generation: %v", role, err)
		}
		if job.validateInitialCandidate == nil {
			t.Fatalf("%s validator was dropped before fragment generation", role)
		}
		if err := job.validateInitialCandidate("candidate"); err != nil {
			t.Fatalf("%s validator rejected fixture candidate: %v", role, err)
		}
	}
	if validatorCalls != 1 {
		t.Fatalf("validator calls=%d; want implementation only", validatorCalls)
	}

	for _, role := range []assemblyline.SourceBlockRole{
		assemblyline.SourceBlockTaskVerification,
		assemblyline.SourceBlockTaskRepresentation,
		assemblyline.SourceBlockRole("unsupported"),
	} {
		_, err := newDirectCodingTypeScriptPortableJob(directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "rejected.source", Signature: "function RejectedSource(): void",
				Contract: "Return one complete declaration.", Role: role,
			},
			dialect: "TypeScript", validateInitialCandidate: validator,
		})
		if err == nil {
			t.Fatalf("%s validator unexpectedly crossed the role boundary", role)
		}
	}
}

func TestTypeScriptVerificationGenerationIsCodeOwnedAndDoesNotRequireASession(t *testing.T) {
	program := testTypeScriptBrowserProgram(
		t,
		"code-owned-verification",
		"neutral control",
		"The finished software lets a user confirm the item.",
	)
	task := program.Workload.Tasks[0]
	implementationID, err := directCodingTaskBlockIDByRole(
		program.Source, task.ID, assemblyline.SourceBlockTaskImplementation,
	)
	if err != nil {
		t.Fatal(err)
	}
	implementationBlock, exists := directCodingSourceBlueprintBlock(
		program.Source, implementationID,
	)
	if !exists {
		t.Fatalf("implementation block %s is absent", implementationID)
	}
	implementation, err := assemblyline.ComposeSourceDeclaration(
		implementationBlock.Signature,
		`return <div><button type="button" onClick={() => actions.set('confirmed', true)}>Confirm item</button></div>;`,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated[implementationID] = implementation

	verificationID, err := directCodingTaskBlockIDByRole(
		program.Source, task.ID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	var verificationRef assemblyline.SourceBlockRef
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.ID == verificationID {
				verificationRef = assemblyline.SourceBlockRef{Document: document, Block: block}
			}
		}
	}
	if verificationRef.Block.ID == "" {
		t.Fatalf("verification block %s is absent", verificationID)
	}
	contexts, err := directCodingApplicationTaskContexts(program.Workload)
	if err != nil {
		t.Fatal(err)
	}
	context, exists := contexts[task.RequirementID]
	if !exists {
		t.Fatalf("task context for %s is absent", task.RequirementID)
	}

	// A nil session is intentional: reaching the provider/model path would panic.
	executor := &directCodingTypeScriptProjectStageExecutor{
		publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
	}
	verification, err := executor.GenerateBlock(context, &program, verificationRef)
	if err != nil {
		t.Fatalf("generate code-owned verification: %v", err)
	}
	for _, wanted := range []string{
		`fireEvent.click(screen.getByRole("button", { name: "Confirm item" }));`,
		`expect(screen.getByRole("button", { name: "Confirm item" })).toBeInTheDocument();`,
	} {
		if !strings.Contains(verification, wanted) {
			t.Fatalf("code-owned verification omitted %q: %s", wanted, verification)
		}
	}
	for _, forbidden := range []string{
		"PUBLIC_INTERACTION_SURFACE", "role_ordinal", "response schema", "Return only",
	} {
		if strings.Contains(verification, forbidden) {
			t.Fatalf("code-owned verification contains model protocol text %q: %s", forbidden, verification)
		}
	}
	assertion := strings.Index(verification, `expect(screen.getByRole("button", { name: "Confirm item" })).toBeInTheDocument();`)
	interaction := strings.Index(verification, `fireEvent.click(screen.getByRole("button", { name: "Confirm item" }));`)
	if assertion < 0 || interaction < 0 || assertion > interaction {
		t.Fatalf("code-owned verifier invented a post-interaction assertion: %s", verification)
	}
}

func TestCodeOwnedBrowserVerificationFailsWithoutExactDerivedResultOracle(t *testing.T) {
	ref := assemblyline.SourceBlockRef{Block: assemblyline.SourceBlock{
		ID:        "acceptance.001",
		Signature: "async function VerifyFeature001(): Promise<void>",
		Role:      assemblyline.SourceBlockTaskVerification,
	}}
	binding := directCodingBrowserPublicSurfaceBinding{
		verificationBlockID: ref.Block.ID,
		verificationTSX:     true,
		surface: directCodingBrowserPublicInteractionSurface{
			Controls: []directCodingBrowserPublicControl{{
				Role: "button", RoleOrdinal: 1, RoleCount: 1,
				AccessibleName: "Calculate", ValueKind: "action",
			}},
			Outputs: []directCodingBrowserPublicOutput{{AccessibleName: "Result"}},
		},
		resultRelation: assemblyline.ApplicationRequirementCandidateResultRelationResult{
			Relation: assemblyline.ApplicationRequirementExplicitResultRelation,
		},
	}
	_, err := renderDirectCodingBrowserVerificationDeclaration(ref, binding)
	if err == nil || !strings.Contains(err.Error(), "exact derived-result oracle") {
		t.Fatalf("derived-result oracle failure=%v", err)
	}
}

func TestCodeOwnedBrowserVerificationRejectsUnprovenSelectionValue(t *testing.T) {
	_, err := renderDirectCodingBrowserMechanicalVerificationStatements(
		directCodingBrowserPublicInteractionSurface{
			Controls: []directCodingBrowserPublicControl{{
				Role: "combobox", RoleOrdinal: 1, RoleCount: 1,
				AccessibleName: "Destination", ValueKind: "selection",
			}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "exact selectable value") {
		t.Fatalf("unproven selection failure=%v", err)
	}
}

func TestBrowserPublicSurfaceRejectsConditionalOutputPresence(t *testing.T) {
	_, err := extractDirectCodingBrowserPublicInteractionSurface(`function Feature001View({ state }: Feature001ViewProps): ReactElement {
  return <div>{state.ready && <output aria-label="Result">{String(state.result ?? '')}</output>}</div>;
}`)
	if err == nil || !strings.Contains(err.Error(), "dynamic output cardinality") {
		t.Fatalf("conditional output failure=%v", err)
	}
}
