package assemblyline

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRegisteredSourceBodyAdaptersOwnTheirDeclarations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		signature string
		body      string
		validate  func(string, string) (string, error)
	}{
		{
			name: "javascript", signature: "function Sum(left, right)",
			body: "return left + right;", validate: ValidateJavaScriptFragment,
		},
		{
			name: "java", signature: "static int Sum(int left, int right)",
			body: "return left + right;", validate: ValidateJavaFragment,
		},
		{
			name: "rust", signature: "fn sum(left: i32, right: i32) -> i32",
			body: "left + right", validate: ValidateRustFragment,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source, err := test.validate(test.signature, test.body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(source, test.signature) != 1 ||
				!strings.Contains(source, test.body) {
				t.Fatalf("code-assembled source=%q", source)
			}
			repeated, err := test.validate(
				test.signature,
				test.signature+" {\n"+test.body+"\n}",
			)
			if err != nil {
				t.Fatalf("extract complete %s declaration: %v", test.name, err)
			}
			if repeated != source {
				t.Fatalf("complete %s declaration extracted source=%q; want %q", test.name, repeated, source)
			}
		})
	}

	t.Run("typescript", func(t *testing.T) {
		t.Parallel()
		contract := TypeScriptFunctionContract{
			Signature: "function Sum(left: number, right: number): number",
		}
		body := "return left + right;"
		fragment, err := ParseTypeScriptFunctionBody(contract, body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(fragment.Source, contract.Signature) != 1 ||
			!strings.Contains(fragment.Source, body) {
			t.Fatalf("code-assembled TypeScript source=%q", fragment.Source)
		}
		repeated, err := ParseTypeScriptFunctionBody(
			contract,
			contract.Signature+" {\n"+body+"\n}",
		)
		if err != nil {
			t.Fatalf("extract complete TypeScript declaration: %v", err)
		}
		if repeated.Source != fragment.Source {
			t.Fatalf("complete TypeScript declaration extracted source=%q; want %q", repeated.Source, fragment.Source)
		}
	})
}

func TestRegisteredSourceBodyAdaptersExtractOneFencedDeclaration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		signature string
		response  string
		body      string
		validate  func(string, string) (string, error)
	}{
		{
			name: "javascript", signature: "function Sum(left, right)",
			response: "Here is the implementation.\n```javascript\nfunction Other(unused) { return left + right; }\n```",
			body:     "return left + right;", validate: ValidateJavaScriptFragment,
		},
		{
			name: "java", signature: "static int Sum(int left, int right)",
			response: "Here is the implementation.\n```java\nprivate String Other(String unused) { return left + right; }\n```",
			body:     "return left + right;", validate: ValidateJavaFragment,
		},
		{
			name: "rust", signature: "fn sum(left: i32, right: i32) -> i32",
			response: "Here is the implementation.\n```rust\npub fn other(unused: i32) -> i32 { left + right }\n```",
			body:     "left + right", validate: ValidateRustFragment,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source, err := test.validate(test.signature, test.response)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(source, test.signature) != 1 || !strings.Contains(source, test.body) ||
				strings.Contains(source, "Other") || strings.Contains(source, "other") {
				t.Fatalf("code-owned declaration extraction=%q", source)
			}
		})
	}

	t.Run("typescript", func(t *testing.T) {
		t.Parallel()
		contract := TypeScriptFunctionContract{
			Signature: "function Sum(left: number, right: number): number",
		}
		fragment, err := ParseTypeScriptFunctionBody(
			contract,
			"Here is the implementation.\n```typescript\nexport function Other(unused: string): string { return left + right; }\n```",
		)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(fragment.Source, contract.Signature) != 1 ||
			!strings.Contains(fragment.Source, "return left + right;") ||
			strings.Contains(fragment.Source, "Other") || strings.Contains(fragment.Source, "export") {
			t.Fatalf("code-owned TypeScript declaration extraction=%q", fragment.Source)
		}
	})
}

func TestSourceBodyExtractionRejectsAmbiguousFencesWithoutCorrectionAuthority(t *testing.T) {
	t.Parallel()
	response := "```javascript\nfunction One() { return 1; }\n```\n```javascript\nfunction Two() { return 2; }\n```"
	_, err := ValidateJavaScriptFragment("function Value()", response)
	if err == nil || !strings.Contains(err.Error(), "2 fenced regions") {
		t.Fatalf("ambiguous extraction error=%v", err)
	}
	var defect *SourceBodyDefect
	if errors.As(err, &defect) {
		t.Fatalf("ambiguous extraction authorized correction: %v", err)
	}

	_, err = ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{Signature: "function Value(): number"},
		"Explanation.\n```typescript\nThis is not source.\n```",
	)
	if err == nil || !strings.Contains(err.Error(), "neither one declaration nor one parseable implementation body") {
		t.Fatalf("prose extraction error=%v", err)
	}
	if errors.As(err, &defect) {
		t.Fatalf("fenced prose authorized correction: %v", err)
	}
}

func TestSourceBodyExtractionAppliesBodyLimitAfterGrossResponseProjection(t *testing.T) {
	t.Parallel()
	response := strings.Repeat("Ordinary explanation outside the source region.\n", 900) +
		"```javascript\nfunction Wrong(unused) { return left + right; }\n```"
	if len(response) <= MaxSourceBodyResponseBytes || len(response) >= MaxPortableRawCandidateBytes {
		t.Fatalf("gross response bytes=%d", len(response))
	}
	job, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)", Behavior: "Return the sum.",
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := NewExactPortableResultProjection(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := (PortableResult{
		JobID: job.ID, Candidate: response, Projection: &projection,
	}).ValidateFor(job); err != nil {
		t.Fatalf("gross ordinary response was rejected before extraction: %v", err)
	}
	source, err := ValidateJavaScriptFragment("function Sum(left, right)", response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "return left + right;") || strings.Contains(source, "explanation") {
		t.Fatalf("large-response extraction=%q", source)
	}
}

func TestOpenSourceCorrectionExtractsOneFencedReplacement(t *testing.T) {
	t.Parallel()
	const body = "return @;"
	defect, err := NewSourceBodyDefect(
		body,
		len("return "),
		len("return @"),
		"What expression belongs here?",
		fmt.Errorf("invalid expression"),
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := correction.Apply(
		"The replacement follows.\n```typescript\nleft + right\n```",
	)
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "return left + right;" {
		t.Fatalf("fenced replacement splice=%q", corrected)
	}
	_, err = correction.Apply("```typescript\nleft\n```\n```typescript\nright\n```")
	if err == nil || !strings.Contains(err.Error(), "2 fenced regions") {
		t.Fatalf("ambiguous replacement extraction error=%v", err)
	}
}

func TestSourceBodyNormalizationIsCodeOwned(t *testing.T) {
	t.Parallel()
	source, err := ComposeSourceDeclaration(
		"function Lines()",
		"const first = 1;\rreturn first;\r\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(source, '\r') || source !=
		"function Lines() {\nconst first = 1;\nreturn first;\n}" {
		t.Fatalf("normalized source=%q", source)
	}
}

func TestSourceBodyDefectRejectsCompletePreviousBody(t *testing.T) {
	t.Parallel()
	const body = "missing"
	if _, err := NewSourceBodyDefect(
		body,
		0,
		len(body),
		"Which available value belongs here?",
		fmt.Errorf("unresolved identifier"),
	); err == nil || !strings.Contains(err.Error(), "complete previously returned body") {
		t.Fatalf("complete-body correction error = %v", err)
	}
}

func TestSourceBodyCorrectionEvidenceRejectsCompletePreviousBody(t *testing.T) {
	t.Parallel()
	const body = "unsafe { 1 }"
	question := "What safe expression belongs here?"
	evidence := SourceBodyCorrectionEvidence{
		BaseCandidate:  body,
		BaseSHA256:     sourceBodySHA256(body),
		StartByte:      0,
		EndByte:        len(body),
		Question:       question,
		QuestionSHA256: sourceBodySHA256(question),
	}
	if err := evidence.Validate(question + "\n\n" + body); err == nil ||
		!strings.Contains(err.Error(), "complete previously returned body") {
		t.Fatalf("complete-body evidence error=%v", err)
	}
}

func TestSourceIdentifierCorrectionUsesSoleCodeOwnedReplacementWithoutModelText(t *testing.T) {
	t.Parallel()
	choice, err := NewOpaqueModelChoice("JavaScript function parameter named \"value\"", "value")
	if err != nil {
		t.Fatal(err)
	}
	body := "return missing;"
	defect, err := NewSourceBodyIdentifierDefect(
		body, len("return "), len("return missing"),
		"Which available value belongs here?",
		fmt.Errorf("unresolved identifier"),
		[]OpaqueModelChoice{choice},
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := correction.ModelInput(); err == nil ||
		!strings.Contains(err.Error(), "forbids a model call") {
		t.Fatalf("sole replacement model input error = %v", err)
	}
	if _, err := correction.Apply("value"); err == nil ||
		!strings.Contains(err.Error(), "forbids a model response") {
		t.Fatalf("sole replacement model response error = %v", err)
	}
	corrected, resolved, err := correction.ApplySoleReplacement()
	if err != nil {
		t.Fatal(err)
	}
	if !resolved || corrected != "return value;" {
		t.Fatalf("sole replacement resolved=%v body=%q", resolved, corrected)
	}
}

func TestSourceIdentifierCorrectionMapsOpaqueLetterToCodeOwnedReplacement(t *testing.T) {
	t.Parallel()
	left, err := NewOpaqueModelChoice("JavaScript first function parameter named \"left\"", "left")
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewOpaqueModelChoice("JavaScript second function parameter named \"right\"", "right")
	if err != nil {
		t.Fatal(err)
	}
	body := "return missing + right;"
	defect, err := NewSourceBodyIdentifierDefect(
		body, len("return "), len("return missing"),
		"Which available value belongs here?",
		fmt.Errorf("unresolved identifier"),
		[]OpaqueModelChoice{left, right},
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(modelInput, "A. JavaScript first function parameter") ||
		!strings.Contains(modelInput, "B. JavaScript second function parameter") ||
		!strings.HasSuffix(modelInput, "\n\nmissing") || strings.Contains(modelInput, body) {
		t.Fatalf("opaque correction input=%q", modelInput)
	}
	maximum, opaque, err := correction.OpaqueResponseMaximumBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !opaque || maximum != 1 {
		t.Fatalf("opaque response maximum=(%d,%v)", maximum, opaque)
	}
	evidence, err := correction.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(modelInput); err != nil {
		t.Fatalf("opaque correction evidence: %v", err)
	}
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "return left + right;" {
		t.Fatalf("opaque correction body=%q", corrected)
	}
	if _, err := correction.Apply("left"); err == nil {
		t.Fatal("typed code-owned identifier unexpectedly decoded as an opaque choice")
	}
}

func TestRegisteredFragmentPromptsDoNotDefineAResponsePacket(t *testing.T) {
	t.Parallel()
	inputs := []FragmentGenerationInput{
		{
			Language: "go", Dialect: "Go 1.23", Signature: "func Sum(left, right int) int",
			Behavior: "Return the sum of left and right.",
		},
		{
			Language: "typescript", Dialect: "TypeScript", Signature: "function Sum(left: number, right: number): number",
			Behavior: "Return the sum of left and right.",
		},
		{
			Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function Sum(left, right)",
			Behavior: "Return the sum of left and right.",
		},
		{
			Language: "java", Dialect: "Java 21", Signature: "static int Sum(int left, int right)",
			Behavior: "Return the sum of left and right.",
		},
		{
			Language: "rust", Dialect: "Rust 2021", Signature: "fn sum(left: i32, right: i32) -> i32",
			Behavior: "Return the sum of left and right.",
		},
		{
			Language: TextFragmentLanguage, Dialect: TextFragmentDialect,
			Signature: TextFragmentSignature, Behavior: "Write one short status sentence.",
		},
	}
	for _, input := range inputs {
		job, err := NewFragmentGenerationJob(input)
		if err != nil {
			t.Fatalf("%s job: %v", input.Language, err)
		}
		prompt, err := RenderPortableJob(job)
		if err != nil {
			t.Fatalf("%s prompt: %v", input.Language, err)
		}
		lower := strings.ToLower(prompt)
		for _, forbidden := range []string{
			"return json", "response schema", "response packet", "ast node",
			"preserve the signature", "exact_signature", "current_declaration_json",
			"implementation body",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s prompt contains response protocol %q: %q", input.Language, forbidden, prompt)
			}
		}
		if !strings.Contains(prompt, input.Behavior) {
			t.Fatalf("%s prompt omits its one semantic responsibility: %q", input.Language, prompt)
		}
		if input.Language != TextFragmentLanguage && !strings.Contains(prompt, input.Signature) {
			t.Fatalf("%s prompt omits its required lexical scope: %q", input.Language, prompt)
		}
	}
}
