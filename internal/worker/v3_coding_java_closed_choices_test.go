package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaSoleMethodCorrectionIsAppliedWithoutModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static int Value(Integer value)",
		Behavior:  "Return the integer value.",
	}
	body := "return value.missing();"
	candidate, validatedBody, correction, err := validateDirectCodingLanguageBody(
		assemblyline.ArtifactIdentityProvenance{},
		directCodingLanguageGenerationJob{
			Subject: "source.java-sole-method", Input: input,
			Validate: validateDirectCodingJavaFragment,
		},
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	if correction != nil {
		t.Fatalf("sole code-owned method reached model correction: %#v", correction)
	}
	if validatedBody != "return value.intValue();" {
		t.Fatalf("validated body=%q", validatedBody)
	}
	if !strings.Contains(candidate, validatedBody) {
		t.Fatalf("assembled candidate=%q; want corrected body", candidate)
	}
}

func TestJavaMethodCorrectionExposesOnlyExactMethodName(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static Object Value(String value)",
		Behavior:  "Return one property of the value.",
	}
	body := "return value.missing();"
	_, validationErr := validateDirectCodingJavaFragment(input, body)
	correction := requireSourceBodyCorrection(t, body, validationErr)
	if correction.Mutable() != "missing" {
		t.Fatalf("mutable=%q; want method name", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	assertExactTokenChoiceInput(t, modelInput, body, input.Signature, "value.", "()")
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(corrected, "return value.") ||
		!strings.HasSuffix(corrected, "();") {
		t.Fatalf("method splice changed accepted invocation bytes: %q", corrected)
	}
}

func TestJavaReceiverCorrectionExposesOnlyExactReceiver(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static int Value(String left, String right)",
		Behavior:  "Return one supplied value length.",
	}
	body := "return missing.length();"
	_, validationErr := validateDirectCodingJavaFragment(input, body)
	correction := requireSourceBodyCorrection(t, body, validationErr)
	if correction.Mutable() != "missing" {
		t.Fatalf("mutable=%q; want receiver", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	assertExactTokenChoiceInput(t, modelInput, body, input.Signature, ".length", "()")
	corrected, err := correction.Apply("B")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "return right.length();" {
		t.Fatalf("receiver splice=%q", corrected)
	}
}

func TestJavaTypeCorrectionUsesOnlyEnumeratedExactToken(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static Object Value()",
		Behavior:  "Return a local value.",
	}
	body := "Unknown local = null;\nreturn local;"
	_, validationErr := validateDirectCodingJavaFragment(input, body)
	correction := requireSourceBodyCorrection(t, body, validationErr)
	if correction.Mutable() != "Unknown" {
		t.Fatalf("mutable=%q; want type token", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	assertExactTokenChoiceInput(t, modelInput, body, input.Signature, "local", "return")
}

func TestJavaInvocationWithoutAuthorizedCandidateFailsLoudly(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static int Value()",
		Behavior:  "Return an integer.",
	}
	_, err := validateDirectCodingJavaFragment(input, "return missing(1);")
	if err == nil {
		t.Fatal("unavailable invocation unexpectedly passed")
	}
	var defect *assemblyline.SourceBodyDefect
	if errors.As(err, &defect) {
		t.Fatalf("zero candidates authorized raw model correction: %v", err)
	}
	if !strings.Contains(err.Error(), "no authorized replacement") {
		t.Fatalf("error=%v; want loud zero-candidate failure", err)
	}
}

func TestJavaCompositeReceiverFailsWithoutBroadCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static int Value(String value)",
		Behavior:  "Return one supplied value length.",
	}
	_, err := validateDirectCodingJavaFragment(
		input, "return missing.trim().length();",
	)
	if err == nil {
		t.Fatal("unresolved composite receiver unexpectedly passed")
	}
	var defect *assemblyline.SourceBodyDefect
	if errors.As(err, &defect) {
		correction, correctionErr := defect.Correction("return missing.trim().length();")
		if correctionErr != nil {
			t.Fatal(correctionErr)
		}
		t.Fatalf("composite receiver authorized broad span %q", correction.Mutable())
	}
	if !strings.Contains(err.Error(), "exact receiver token") {
		t.Fatalf("error=%v; want loud exact-receiver failure", err)
	}
}

func TestJavaForbiddenDeclarationFailsWithoutBroadCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21",
		Signature: "static void Value()",
		Behavior:  "Perform the bounded behavior.",
	}
	_, err := validateDirectCodingJavaFragment(input, "class Nested {}")
	if err == nil {
		t.Fatal("nested declaration unexpectedly passed")
	}
	var defect *assemblyline.SourceBodyDefect
	if errors.As(err, &defect) {
		correction, correctionErr := defect.Correction("class Nested {}")
		if correctionErr != nil {
			t.Fatal(correctionErr)
		}
		t.Fatalf("nested declaration authorized broad span %q", correction.Mutable())
	}
	if !strings.Contains(err.Error(), "forbidden class_declaration authority") {
		t.Fatalf("error=%v; want loud declaration failure", err)
	}
}

func requireSourceBodyCorrection(
	t *testing.T,
	body string,
	validationErr error,
) assemblyline.SourceBodyCorrection {
	t.Helper()
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(validationErr, &defect) {
		t.Fatalf("validation error=%v; want exact source-body defect", validationErr)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	return correction
}

func assertExactTokenChoiceInput(
	t *testing.T,
	modelInput string,
	body string,
	signature string,
	forbidden ...string,
) {
	t.Helper()
	if !strings.Contains(modelInput, "A. ") || !strings.Contains(modelInput, "B. ") ||
		!strings.Contains(modelInput, "Answer with ") {
		t.Fatalf("correction is not an opaque choice: %q", modelInput)
	}
	forbidden = append(forbidden, body, signature)
	for _, value := range forbidden {
		if strings.Contains(modelInput, value) {
			t.Fatalf("correction input exposed accepted source %q: %q", value, modelInput)
		}
	}
}
