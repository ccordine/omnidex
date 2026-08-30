package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementInventoryIsUntrustedAtomicRuntimeCandidateData(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryFixture(t)
	job, err := NewApplicationRequirementInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationRequirementInventory {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := BuildApplicationRequirementInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"atomic finished-software runtime-outcome candidates",
		"Split only independent outcomes",
		"purpose-denoting product or category name",
		"literal core operation or governed result",
		"simplest corresponding action and governed object",
		"minimal independently verifiable governed result",
		"Verifiable does not authorize a presentation or delivery channel",
		"Do not invent customary controls",
		"Do not add a generic trigger frame",
		ApplicationNoRuntimeRequirementCandidates,
		"between 1 and",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("inventory prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"task queue", "workflow decision", "completion claim", "Downstream code",
		"accept or reject", "permission to continue", "review",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("inventory prompt exposed orchestration language %q:\n%s", forbidden, prompt)
		}
	}
	raw := "The finished software transforms the user-provided value.\nThe finished software returns the value produced by the requested transformation.\nThe finished software remains available after the initiating request ends."
	inventory, err := DecodeApplicationRequirementInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	if inventory.Schema != ApplicationRequirementInventorySchemaV4 {
		t.Fatalf("schema=%q", inventory.Schema)
	}
	if len(inventory.Candidates) != 3 ||
		inventory.Candidates[1] != "The finished software returns the value produced by the requested transformation." {
		t.Fatalf("candidates=%q", inventory.Candidates)
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		t.Fatalf("raw hash=%q", inventory.RawSHA256)
	}
	inventory.Candidates[0] = "mutated"
	redecoded, err := DecodeApplicationRequirementInventory(input, raw)
	if err != nil || redecoded.Candidates[0] == inventory.Candidates[0] {
		t.Fatalf("decoded inventory did not own candidates: %+v error=%v", redecoded, err)
	}
}

func TestApplicationRequirementInventoryRepresentsExactSemanticAbsence(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryFixtureFor(t, "Use React.")
	if _, err := NewApplicationRequirementInventoryJob(input); err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationRequirementInventoryPrompt(input)
	if err != nil || !strings.Contains(prompt, ApplicationNoRuntimeRequirementCandidates) {
		t.Fatalf("inventory prompt lost absence contract: error=%v\n%s", err, prompt)
	}
	inventory, err := DecodeApplicationRequirementInventory(
		input,
		ApplicationNoRuntimeRequirementCandidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Candidates == nil || len(inventory.Candidates) != 0 ||
		inventory.RawSHA256 != ExactObjectiveContextSHA(
			ApplicationNoRuntimeRequirementCandidates,
		) {
		t.Fatalf("absence inventory=%+v", inventory)
	}
	if _, err := DecodeApplicationRequirementInventory(
		input,
		"Result: "+ApplicationNoRuntimeRequirementCandidates,
	); err == nil {
		t.Fatal("accepted wrapped absence value")
	}
}

func TestApplicationRequirementInventoryPromptSeesImmutableRequestAndValidatedContext(t *testing.T) {
	t.Parallel()
	const request = "Add a summary panel to the existing application."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceExisting)
	if err != nil {
		t.Fatal(err)
	}
	need, err := NewApplicationRepositoryContextNeed(1, "Which declaration owns the current summary?")
	if err != nil {
		t.Fatal(err)
	}
	const fact = "SummaryController owns the current summary calculation."
	context, err = AppendApplicationContextEvidence(context, need, []ApplicationContextEvidence{{
		Value: fact, SourceID: "symbol:SummaryController", SourceSHA256: ExactObjectiveContextSHA(fact),
	}})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationRequirementInventoryPrompt(ApplicationRequirementInventoryInput{
		UserRequest: request,
		Context:     context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{request, fact, "WORKSPACE STATE:\nexisting"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("inventory prompt omitted bound authority %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"symbol:SummaryController", need.ID} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("inventory prompt exposed context provenance %q:\n%s", forbidden, prompt)
		}
	}
}

func TestApplicationRequirementInventoryBindsAuthorityRawOrderAndDuplicates(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryFixture(t)
	raw := "Show the current value.\nShow the current value."
	inventory, err := DecodeApplicationRequirementInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 2 {
		t.Fatalf("exact candidate proposals were prematurely deduplicated: %+v", inventory)
	}

	changedRaw := inventory
	changedRaw.Candidates = append([]string(nil), inventory.Candidates...)
	changedRaw.Candidates[1] = "Reset the current value."
	if err := changedRaw.ValidateFor(input); err == nil {
		t.Fatal("mutated candidate list retained raw authority")
	}

	changedInput := applicationRequirementInventoryFixtureFor(
		t,
		"Build a browser transformer that resets its value.",
	)
	if err := inventory.ValidateFor(changedInput); err == nil {
		t.Fatal("inventory receipt validated for different request authority")
	}

	reordered := inventory
	reordered.Candidates = []string{inventory.Candidates[1], inventory.Candidates[0]}
	if err := reordered.ValidateFor(input); err != nil {
		t.Fatal("equal duplicate order should remain byte-identical", err)
	}
}

func TestApplicationRequirementInventoryRejectsInvalidFramingAndBounds(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryFixture(t)
	for _, raw := range []string{
		"",
		" first",
		"first ",
		"first\n\nsecond",
		"first\r\nsecond",
		"first\x00second",
		string([]byte{0xff}),
		`["first","second"]`,
		"no_runtime_requirement_candidates",
	} {
		if _, err := DecodeApplicationRequirementInventory(input, raw); err == nil {
			t.Fatalf("accepted invalid inventory %q", raw)
		}
	}
	tooMany := strings.Repeat(
		"candidate\n",
		MaxApplicationRequirementInventoryCandidates,
	) + "candidate"
	if _, err := DecodeApplicationRequirementInventory(input, tooMany); err == nil {
		t.Fatal("accepted inventory above candidate bound")
	}
	overlong := strings.Repeat("x", maxRequirementQuoteBytes+1)
	if _, err := DecodeApplicationRequirementInventory(input, overlong); err == nil {
		t.Fatal("accepted overlong inventory candidate")
	}
	if _, err := DecodeApplicationRequirementInventory(input, "first"); err != nil {
		t.Fatalf("rejected one positive inventory candidate: %v", err)
	}
	if _, err := DecodeApplicationRequirementInventory(
		input,
		ApplicationNoRuntimeRequirementCandidates+"\nfirst",
	); err == nil {
		t.Fatal("accepted semantic absence mixed with a positive candidate")
	}
}

func applicationRequirementInventoryFixture(t *testing.T) ApplicationRequirementInventoryInput {
	t.Helper()
	return applicationRequirementInventoryFixtureFor(
		t,
		"Build a browser transformer, verify it, and keep it running.",
	)
}

func applicationRequirementInventoryFixtureFor(
	t *testing.T,
	request string,
) ApplicationRequirementInventoryInput {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementInventoryInput{
		UserRequest: request,
		Context:     context,
	}
}
