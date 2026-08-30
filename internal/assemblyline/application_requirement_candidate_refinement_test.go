package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateCardinalityIsCandidateBound(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Show a status board and send an alert.",
	}
	job, err := NewApplicationRequirementCandidateCardinalityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.Candidate) != 1 ||
		strings.Contains(prompt, "USER REQUEST") ||
		!strings.Contains(prompt, "A second required response meaning is a separate outcome") {
		t.Fatalf("cardinality projection exceeded one candidate:\n%s", prompt)
	}
	for _, relation := range []string{
		ApplicationRequirementOneRuntimeOutcome,
		ApplicationRequirementMultipleRuntimeOutcomes,
	} {
		result, err := DecodeApplicationRequirementCandidateCardinalityResult(input, relation)
		if err != nil {
			t.Fatal(err)
		}
		if err := result.ValidateFor(input); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplicationRequirementCandidatePartitionAcceptsOneExactDefectAuthority(t *testing.T) {
	t.Parallel()
	parent := "Build a status board and send an alert using React."
	kind := applicationRequirementCandidateKindFixture(
		t,
		parent,
		ApplicationRequirementCandidateMixed,
	)
	mixedInput := ApplicationRequirementCandidatePartitionInput{
		Candidate: parent,
		Kind:      &kind,
	}
	assertApplicationRequirementPartition(
		t,
		mixedInput,
		"Show a status board and send an alert.\nBuild the application using React.",
	)
	if _, err := DecodeApplicationRequirementCandidatePartition(
		mixedInput,
		"Show a status board.\nBuild the application using React.\nSend an alert.",
	); err == nil {
		t.Fatal("mixed-kind partition accepted more than its exact two semantic components")
	}

	runtimeParent := "Show a status board and send an alert."
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: runtimeParent,
	}
	cardinality, err := DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput,
		ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	multipleInput := ApplicationRequirementCandidatePartitionInput{
		Candidate:   runtimeParent,
		Cardinality: &cardinality,
	}
	assertApplicationRequirementPartition(
		t,
		multipleInput,
		"Show a status board.\nSend an alert.",
	)
}

func TestApplicationRequirementCandidatePartitionRejectsLossyOrUnboundResults(t *testing.T) {
	t.Parallel()
	parent := "Show a status board and send an alert."
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{Candidate: parent}
	cardinality, err := DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput,
		ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationRequirementCandidatePartitionInput{
		Candidate:   parent,
		Cardinality: &cardinality,
	}
	for _, raw := range []string{
		"Show a status board.",
		parent + "\nSend an alert.",
		"Show a status board.\nShow a status board.",
		"Show a status board.\n\nSend an alert.",
	} {
		if _, err := DecodeApplicationRequirementCandidatePartition(input, raw); err == nil {
			t.Fatalf("accepted invalid partition %q", raw)
		}
	}
	partition, err := DecodeApplicationRequirementCandidatePartition(
		input,
		"Show a status board.\nSend an alert.",
	)
	if err != nil {
		t.Fatal(err)
	}
	changed := partition
	changed.Candidates = append([]string(nil), partition.Candidates...)
	changed.Candidates[0] = "Show another board."
	if err := changed.ValidateFor(input); err == nil {
		t.Fatal("mutated partition retained its raw receipt")
	}
	if _, err := NewApplicationRequirementCandidatePartitionJob(
		ApplicationRequirementCandidatePartitionInput{Candidate: parent},
	); err == nil {
		t.Fatal("partition accepted no defect receipt")
	}
	kind := applicationRequirementCandidateKindFixture(
		t,
		parent,
		ApplicationRequirementCandidateMixed,
	)
	if _, err := NewApplicationRequirementCandidatePartitionJob(
		ApplicationRequirementCandidatePartitionInput{
			Candidate:   parent,
			Kind:        &kind,
			Cardinality: &cardinality,
		},
	); err == nil {
		t.Fatal("partition accepted two defect receipts")
	}
}

func assertApplicationRequirementPartition(
	t *testing.T,
	input ApplicationRequirementCandidatePartitionInput,
	raw string,
) {
	t.Helper()
	job, err := NewApplicationRequirementCandidatePartitionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"one complete lossless proper refinement",
		"strictly narrower semantic children",
		"Preserve every explicit meaning exactly once",
		"Do not propose alternative implementations, technologies, architectures, APIs, algorithms",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("partition prompt omitted %q:\n%s", required, prompt)
		}
	}
	if input.Kind != nil {
		for _, required := range []string{
			"Return exactly two raw child lines",
			"all and only the task-local runtime-outcome meaning",
			"one complete declarative runtime outcome with an explicit running-software or user subject",
			"all and only the non-runtime constraint meaning",
			"runtime first and non-runtime second",
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("mixed-kind partition prompt omitted %q:\n%s", required, prompt)
			}
		}
	} else if !strings.Contains(
		prompt,
		"Keep one runtime outcome's operands, condition, determining rule, and resulting output together",
	) {
		t.Fatalf("multi-outcome partition prompt lost outcome cohesion:\n%s", prompt)
	}
	for _, forbidden := range []string{"workflow", "accept a child", "later runtime/non-runtime classification"} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("partition prompt exposed downstream control language %q:\n%s", forbidden, prompt)
		}
	}
	partition, err := DecodeApplicationRequirementCandidatePartition(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := partition.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	if len(partition.Candidates) != 2 ||
		partition.RawSHA256 != ExactObjectiveContextSHA(raw) {
		t.Fatalf("partition=%+v", partition)
	}
}
