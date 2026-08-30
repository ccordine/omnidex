package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationProductContextIsOneRawBoundedLeaf(t *testing.T) {
	t.Parallel()
	authority := applicationIntentLeafFixture(t)
	input := ApplicationProductContextInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	}
	prompt, err := BuildApplicationProductContextPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"one semantic question",
		"product or domain identity",
		"Exclude requested qualities, capabilities, behaviors",
		"tests, build or deployment constraints",
		"Return only one concise product or domain identity phrase",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("product-context prompt omitted %q:\n%s", required, prompt)
		}
	}
	product, err := DecodeApplicationProductContextLeaf(
		input,
		"A browser counter for tracking a current value.",
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
		if _, err := DecodeApplicationProductContextLeaf(input, wrapped); err == nil {
			t.Fatalf("accepted wrapped product context %q", wrapped)
		}
	}
}

func applicationIntentLeafFixture(t *testing.T) ApplicationIntentInput {
	t.Helper()
	request := "Build a browser counter that displays and increments a count."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentInput{UserRequest: request, Context: context}
}
