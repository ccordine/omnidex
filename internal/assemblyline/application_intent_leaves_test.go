package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationIntentLeavesReturnOneRawValueAtATime(t *testing.T) {
	authority := applicationIntentLeafFixture(t)
	productInput := ApplicationProductContextInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
	}
	prompt, err := BuildApplicationProductContextPrompt(productInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "one semantic question") ||
		!strings.Contains(prompt, "Return only that product context") ||
		!strings.Contains(prompt, "JSON") {
		t.Fatalf("product-context prompt is not one raw leaf: %s", prompt)
	}
	product, err := DecodeApplicationProductContextLeaf(
		productInput, "A browser counter for tracking a current value.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if product != "A browser counter for tracking a current value." {
		t.Fatalf("product=%q", product)
	}
	for _, wrapped := range []string{
		`"A browser counter."`,
		`{"product_context":"A browser counter."}`,
	} {
		if _, err := DecodeApplicationProductContextLeaf(productInput, wrapped); err == nil {
			t.Fatalf("accepted wrapped product context %q", wrapped)
		}
	}
}

func TestApplicationRequirementLeavesSeparateCoverageFromGeneration(t *testing.T) {
	authority := applicationIntentLeafFixture(t)
	input := ApplicationRequirementLeafInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
		ProductContext: "A browser counter.", AcceptedRequirements: []string{},
	}
	coveragePrompt, err := BuildApplicationRequirementCoveragePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	requirementPrompt, err := BuildApplicationRequirementPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(coveragePrompt, "Return only the requirement as raw prose") ||
		strings.Contains(requirementPrompt, "NO_UNCOVERED_REQUIREMENT") {
		t.Fatalf("coverage and generation responsibilities were combined")
	}
	coverage, err := DecodeApplicationRequirementCoverageLeaf(
		input, ApplicationRequirementRemains,
	)
	if err != nil || coverage != ApplicationRequirementRemains {
		t.Fatalf("coverage=%q err=%v", coverage, err)
	}
	requirement, err := DecodeApplicationRequirementLeaf(
		input, "The user can increment the current count.",
	)
	if err != nil {
		t.Fatal(err)
	}
	input.AcceptedRequirements = []string{requirement}
	if _, err := DecodeApplicationRequirementLeaf(input, requirement); err == nil ||
		!strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate requirement error=%v", err)
	}
}

func applicationIntentLeafFixture(t *testing.T) ApplicationIntentInput {
	t.Helper()
	request := "Build a browser counter that displays and increments a count."
	context, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentInput{
		UserRequest: request,
		Context:     context,
	}
}
