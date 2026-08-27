package assemblyline

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestArtifactRedactionPreservesVersionsAndHidesSourceIdentity(t *testing.T) {
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{
		"docs/REQUEST.md", "docs/current-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := RedactArtifactIdentities(
		"Build this in Go 1.22. Do not modify REQUEST.md or docs/current-plan.md.",
		provenance,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(redacted, "Go 1.22") {
		t.Fatalf("version was corrupted: %q", redacted)
	}
	for _, leaked := range []string{"REQUEST.md", "docs/current-plan.md"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("identity %q leaked: %q", leaked, redacted)
		}
	}
	if len(identities) != 2 || !strings.Contains(redacted, identities[0].Token) || !strings.Contains(redacted, identities[1].Token) {
		t.Fatalf("redacted=%q identities=%#v", redacted, identities)
	}
}

func TestArtifactRedactionDoesNotTreatDottedGoSymbolsAsPaths(t *testing.T) {
	t.Parallel()
	request := "Use http.Client and time.Time while leaving transport.go unchanged."
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := RedactArtifactIdentities(request, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(redacted, "http.Client") || !strings.Contains(redacted, "time.Time") {
		t.Fatalf("dotted symbols were corrupted: %q", redacted)
	}
	if len(identities) != 1 || identities[0].Value != "transport.go" {
		t.Fatalf("artifact identities=%#v", identities)
	}
}

func TestArtifactRedactionCoversCrossPlatformAndExtensionlessPaths(t *testing.T) {
	request := `Protect /etc/passwd, src/generated, .env, C:\work\Foo.java, and My Files/a.js.`
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{".env", "My Files/a.js"})
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := RedactArtifactIdentities(request, provenance)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{
		"/etc/passwd", "src/generated", ".env", `C:\work\Foo.java`, "My Files/a.js",
	} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("filesystem identity %q leaked through %q", leaked, redacted)
		}
	}
	if len(identities) != 5 {
		t.Fatalf("redacted=%q identities=%#v", redacted, identities)
	}
}

func TestArtifactRedactionDoesNotGloballyExemptRoutesMIMEOrURLs(t *testing.T) {
	request := `Serve GET /records/{record_id} as application/json and link https://example.com/assets/app.js.`
	redacted, identities, err := RedactArtifactIdentities(
		request, modelcontext.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"/records/{record_id}", "application/json", "https://example.com/assets/app.js",
	} {
		if strings.Contains(redacted, value) {
			t.Fatalf("untyped path-shaped value %q survived redaction: %q", value, redacted)
		}
	}
	if len(identities) != 3 {
		t.Fatalf("redacted=%q identities=%#v", redacted, identities)
	}
}

func TestFragmentEnvelopeRejectsInventedFilesystemIdentity(t *testing.T) {
	_, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func feature() string",
		Behavior: "Write the result through src/generated.",
	})
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("fragment path-free error=%v", err)
	}
	for _, behavior := range []string{
		"Return application/json.", "Handle GET /records/{record_id}.",
	} {
		if _, err := NewFragmentGenerationJob(FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24", Signature: "func feature() string", Behavior: behavior,
		}); err == nil {
			t.Fatalf("untyped fragment behavior %q bypassed the path-free boundary", behavior)
		}
	}
}

func TestTypedEndpointFieldsOwnRouteAndMediaExemptions(t *testing.T) {
	t.Parallel()
	authority := ApplicationServiceEndpointTaskAuthority{
		ProductContext: "Inventory service", RequirementQuote: "Return one record",
		Objective: "Return one record", RequiredBehaviors: []string{"Read one record"},
		AcceptanceCriteria: []string{"The requested record is returned"},
	}
	contract := ApplicationServiceEndpointContract{
		Schema:   ApplicationServiceEndpointContractSchemaV1,
		Exposure: ApplicationServiceEndpointPublic, Method: ApplicationServiceEndpointGET,
		RouteTemplate: "/records/{record_id}", RequestMedia: ApplicationServiceEndpointMediaNone,
		ResponseMedia: ApplicationServiceEndpointJSON, SuccessStatus: 200,
	}
	if err := contract.ValidateFor(authority); err != nil {
		t.Fatalf("typed route/media contract was rejected: %v", err)
	}
	if err := ValidatePathFreeModelContext(
		"untyped prose", string(contract.RouteTemplate), string(contract.ResponseMedia),
	); err == nil {
		t.Fatal("untyped prose inherited the endpoint field exemption")
	}
}

func TestArtifactRedactionRetainsUnprovenSemanticDottedNames(t *testing.T) {
	t.Parallel()
	request := "Build a Node.js service with Vue.js."
	redacted, identities, err := RedactArtifactIdentities(
		request, modelcontext.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if redacted != request || len(identities) != 0 {
		t.Fatalf("semantic dotted names were treated as artifact identities: %q %#v", redacted, identities)
	}
}

func TestFragmentBoundaryRetainsDottedSemanticsAndParserProvenSourceGrammar(t *testing.T) {
	t.Parallel()
	if _, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function runtimeLabel()",
		Behavior: "Return the Node.js and Vue.js runtime label.",
	}); err != nil {
		t.Fatalf("semantic dotted names were rejected: %v", err)
	}
	if err := ValidatePathFreeSourceModelContext(
		"parser-proven JavaScript", `function ratio(left, right) {
  const fraction = left / right;
  return /\d+\/\d+/.test(String(fraction));
}`,
	); err != nil {
		t.Fatalf("division or regular-expression source grammar was rejected: %v", err)
	}
}

func TestFragmentSourceBoundaryRejectsPathBearingLiteral(t *testing.T) {
	t.Parallel()
	err := ValidatePathFreeSourceModelContext(
		"parser-proven JavaScript", `function value() { return "../private/value"; }`,
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("path-bearing source literal error=%v", err)
	}
}

func TestFragmentSourceBoundaryReportsEncodedPathFromRawSource(t *testing.T) {
	t.Parallel()
	rawIdentity := `\x2fprivate\x2fvalue`
	err := ValidatePathFreeSourceModelContext(
		"parser-proven JavaScript", `function value() { return "`+rawIdentity+`"; }`,
	)
	if err == nil || !strings.Contains(err.Error(), strconv.Quote(rawIdentity)) {
		t.Fatalf("encoded source path did not retain its raw diagnostic span: %v", err)
	}
}
