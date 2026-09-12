package assemblyline

import (
	"os"
	"strings"
	"testing"
)

func TestRejectedCandidateScopeAnnotationStationIsUnavailable(t *testing.T) {
	const removed WorkKind = "application_requirement_candidate_scope_relation"
	job := PortableJob{Schema: PortableJobSchemaV2, Kind: removed, Payload: []byte(`{}`)}
	if err := job.Validate(); err == nil {
		t.Fatal("removed scope annotation station still validates")
	}
	if _, err := RenderPortableJob(job); err == nil {
		t.Fatal("removed scope annotation station still renders")
	}
	if _, err := PortableResponseFramingForWorkKind(removed); err == nil {
		t.Fatal("removed scope annotation station retains response framing")
	}
	if _, err := PortableResponseMaximumBytesForJob(job); err == nil {
		t.Fatal("removed scope annotation station retains a response limit")
	}
	if _, err := SemanticUncertaintyContractForWorkKind(removed); err == nil {
		t.Fatal("removed scope annotation station retains a semantic uncertainty contract")
	}
	for _, kind := range AllWorkKinds() {
		if kind == removed {
			t.Fatal("removed scope annotation station remains registered")
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "ApplicationRequirementCandidateScope") {
			t.Errorf("%s retains the removed scope annotation station", entry.Name())
		}
	}
}
