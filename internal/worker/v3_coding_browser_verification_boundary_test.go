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

func TestTypeScriptInitialCandidateValidatorsReachOnlyImplementationAndVerificationGeneration(t *testing.T) {
	validatorCalls := 0
	validator := func(string) error {
		validatorCalls++
		return nil
	}
	for _, role := range []assemblyline.SourceBlockRole{
		assemblyline.SourceBlockTaskImplementation,
		assemblyline.SourceBlockTaskVerification,
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
	if validatorCalls != 2 {
		t.Fatalf("validator calls=%d; want implementation and verification", validatorCalls)
	}

	for _, role := range []assemblyline.SourceBlockRole{
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
