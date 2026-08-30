package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserAcceptancePreambleCapturesBoundedRoleObservations(t *testing.T) {
	t.Parallel()

	source := genericBrowserAcceptancePreamble("./runtime")
	if err := assemblyline.ValidateTypeScriptSource(source, true); err != nil {
		t.Fatalf("role-observation preamble is not parseable TypeScript: %v", err)
	}
	for _, required := range []string{
		"// @ts-expect-error dom-accessibility-api 0.5.16 omits its declarations from package exports.",
		"import { computeAccessibleName } from 'dom-accessibility-api';",
		"configure, fireEvent, getRoles, render, screen, waitFor",
		"getElementError: (message, container): Error =>",
		"error.name = 'TestingLibraryElementError';",
		"new Error(message === null ? '' : message)",
		"Object.defineProperty(error, 'omnidexTestingLibraryRoleObservation'",
		"enumerable: true",
		"schema: 'omnidex.testing-library-role-observation.v1'",
		"status: 'complete' | 'limit_exceeded' | 'capture_failed'",
		"omnidexTestingLibraryRoleLimit = 64",
		"omnidexTestingLibraryElementLimit = 100",
		"omnidexTestingLibraryNameLimit = 256",
		"element_count: elementCount, names: []",
		"element_count: elementCount, names",
		"computeAccessibleName(element)",
		"getRoles as unknown as",
		"hidden: missingRole.visibility === 'available'",
		"Object.hasOwn(roles, missingRole.requestedRole)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("role-observation preamble omits %q", required)
		}
	}
	for _, forbidden := range []string{
		"prettyRoles", "prettyDOM", "innerHTML", "outerHTML", "querySelector",
		"Here are the accessible roles", "Name \"", "hasOwnProperty.call",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("role-observation preamble parses provider/DOM prose through %q", forbidden)
		}
	}
}

func TestBrowserAcceptanceMissingRoleGrammarIsExactToPinnedProviderPrefix(t *testing.T) {
	t.Parallel()

	source := genericBrowserTestingLibraryRoleObservationSupport
	for _, prefix := range []string{
		`Unable to find an accessible element with the role "`,
		`Unable to find an element with the role "`,
	} {
		if strings.Count(source, prefix) != 1 {
			t.Fatalf("missing-role grammar has %d copies of exact prefix %q", strings.Count(source, prefix), prefix)
		}
	}
	for _, boundary := range []string{
		"message.startsWith(omnidexTestingLibraryAccessibleRolePrefix)",
		"message.startsWith(omnidexTestingLibraryAvailableRolePrefix)",
		"const closingQuote = message.indexOf('\"', prefix.length)",
		"suffix.startsWith('\\n\\n')",
		"suffix.startsWith(' and name ')",
		"suffix.startsWith(' and description ')",
	} {
		if !strings.Contains(source, boundary) {
			t.Fatalf("missing-role grammar omits strict boundary %q", boundary)
		}
	}
	if strings.Contains(source, "message.match(") || strings.Contains(source, "new RegExp(") {
		t.Fatal("missing-role capture regained broad prose parsing")
	}
}

func TestGenericBrowserAcceptanceDocumentsInstallObservationProviderOncePerFile(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "shared verification fixture",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "show available inventory"},
			{ID: "requirement_002", SourceQuote: "show scheduled deliveries"},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		t.Fatal(err)
	}
	coverage := assemblyline.ApplicationFileCoveragePlan{
		WorkloadSHA256: workload.SHA256,
		Files: []assemblyline.ApplicationFileCoverage{
			{
				Path: "src/Features.tsx", Kind: assemblyline.TargetArtifactImplementation,
				TaskIDs: []string{"task_001", "task_002"},
			},
			{
				Path: "src/Features.test.tsx", Kind: assemblyline.TargetArtifactVerification,
				TaskIDs: []string{"task_001", "task_002"},
			},
		},
	}
	documents, err := genericBrowserAcceptanceDocuments(
		specification,
		contexts,
		directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil},
		coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("shared verification path produced %d documents, want 1", len(documents))
	}
	preamble := documents[0].Preamble
	if strings.Count(preamble, "configure({") != 1 ||
		strings.Count(preamble, "omnidexTestingLibraryRoleObservation'") != 1 {
		t.Fatalf("acceptance preamble did not install exactly one provider:\n%s", preamble)
	}
	if !strings.Contains(preamble, "from './runtime';") {
		t.Fatalf("acceptance preamble lost its runtime import:\n%s", preamble)
	}
}

func TestTypeScriptBrowserPinsAccessibleNameProviderAsDirectDependency(t *testing.T) {
	t.Parallel()

	profile := requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1)
	if got := profile.NPMDevDependencies["dom-accessibility-api"]; got != "0.5.16" {
		t.Fatalf("dom-accessibility-api profile pin=%q want=0.5.16", got)
	}
	var lock struct {
		Packages map[string]struct {
			Version         string            `json:"version"`
			DevDependencies map[string]string `json:"devDependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(typeScriptBrowserPackageLockTemplate, &lock); err != nil {
		t.Fatal(err)
	}
	if got := lock.Packages[""].DevDependencies["dom-accessibility-api"]; got != "0.5.16" {
		t.Fatalf("lock root dom-accessibility-api pin=%q want=0.5.16", got)
	}
	if got := lock.Packages["node_modules/dom-accessibility-api"].Version; got != "0.5.16" {
		t.Fatalf("locked dom-accessibility-api version=%q want=0.5.16", got)
	}
}
