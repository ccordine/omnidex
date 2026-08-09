package modelgauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLoadCapabilityRelationCasesAndLabelsRequireExactSchemas(t *testing.T) {
	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	writeFixture(t, casesPath, `{
		"schema":"omnidex.model-gauntlet.capability-relation-cases.v1",
		"cases":[{"id":"one","input":{"local_context":"cart","left_need":"show total","right_need":"change items"}}]
	}`)
	writeFixture(t, labelsPath, `{
		"schema":"omnidex.model-gauntlet.capability-relation-labels.v1",
		"labels":[{"case_id":"one","relation":"left_reads_right"}]
	}`)

	cases, err := LoadCapabilityRelationCases(casesPath)
	if err != nil || len(cases) != 1 || cases[0].ID != "one" {
		t.Fatalf("cases=%#v error=%v", cases, err)
	}
	labels, hash, err := LoadCapabilityRelationLabels(labelsPath, cases)
	if err != nil || len(labels) != 1 || labels[0].Relation != assemblyline.CapabilityLeftReadsRight || len(hash) != 64 {
		t.Fatalf("labels=%#v hash=%q error=%v", labels, hash, err)
	}

	writeFixture(t, filepath.Join(directory, "bad.json"), `{"schema":"wrong","cases":[]}`)
	_, err = LoadCapabilityRelationCases(filepath.Join(directory, "bad.json"))
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("wrong schema error=%v", err)
	}
}

func TestLoadCapabilityRelationLabelsRejectsMissingAndUnknownCases(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "labels.json")
	cases := []CapabilityRelationCase{
		capabilityCase("one", "cart", "show total", "change items"),
		capabilityCase("two", "account", "choose account", "show history"),
	}
	writeFixture(t, path, `{
		"schema":"omnidex.model-gauntlet.capability-relation-labels.v1",
		"labels":[{"case_id":"unknown","relation":"independent"}]
	}`)
	_, _, err := LoadCapabilityRelationLabels(path, cases)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown label error=%v", err)
	}

	writeFixture(t, path, `{
		"schema":"omnidex.model-gauntlet.capability-relation-labels.v1",
		"labels":[{"case_id":"one","relation":"left_reads_right"}]
	}`)
	_, _, err = LoadCapabilityRelationLabels(path, cases)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing label error=%v", err)
	}
}

func TestWriteCapabilityRelationResultRefusesToOverwriteEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	result := CapabilityRelationResult{
		Schema: CapabilityRelationResultSchemaV1,
		Report: CapabilityRelationReport{Schema: CapabilityRelationReportSchemaV1},
	}
	if err := WriteCapabilityRelationResult(path, result); err != nil {
		t.Fatal(err)
	}
	if err := WriteCapabilityRelationResult(path, result); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error=%v", err)
	}
}

func TestCheckedInCapabilityRelationGauntletIsBalancedAndComplete(t *testing.T) {
	root := filepath.Join("..", "..", "gauntlets", "capability_relation")
	cases, err := LoadCapabilityRelationCases(filepath.Join(root, "cases.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, _, err := LoadCapabilityRelationLabels(filepath.Join(root, "labels.v1.json"), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 12 || len(labels) != 12 {
		t.Fatalf("cases=%d labels=%d want 12 each", len(cases), len(labels))
	}
	counts := make(map[assemblyline.CapabilityRelation]int)
	for _, label := range labels {
		counts[label.Relation]++
	}
	for _, relation := range []assemblyline.CapabilityRelation{
		assemblyline.CapabilityIndependent, assemblyline.CapabilityLeftReadsRight,
		assemblyline.CapabilityRightReadsLeft, assemblyline.CapabilityBidirectional,
	} {
		if counts[relation] != 3 {
			t.Fatalf("relation %q count=%d want 3", relation, counts[relation])
		}
	}
}

func TestCheckedInRequirementPartitionGauntletHasBothModesAndCompleteLabels(t *testing.T) {
	root := filepath.Join("..", "..", "gauntlets", "requirement_partition")
	cases, err := LoadRequirementPartitionCases(filepath.Join(root, "cases.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, hash, err := LoadRequirementPartitionLabels(filepath.Join(root, "labels.v1.json"), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 8 || len(labels) != 8 || len(hash) != 64 {
		t.Fatalf("cases=%d labels=%d hash=%q", len(cases), len(labels), hash)
	}
	modes := make(map[assemblyline.RequirementPartitionMode]int)
	for _, fixture := range cases {
		modes[fixture.Input.Mode]++
	}
	if modes[assemblyline.RequirementExtractFeatures] != 4 || modes[assemblyline.RequirementSplitFeature] != 4 {
		t.Fatalf("mode counts=%#v", modes)
	}
}

func TestWriteRequirementPartitionResultRefusesToOverwriteEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	result := RequirementPartitionResult{
		Schema: RequirementPartitionResultSchemaV1,
		Report: RequirementPartitionReport{Schema: RequirementPartitionReportSchemaV1},
	}
	if err := WriteRequirementPartitionResult(path, result); err != nil {
		t.Fatal(err)
	}
	if err := WriteRequirementPartitionResult(path, result); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error=%v", err)
	}
}

func TestCheckedInRepositoryRetrievalGauntletCoversEveryRegisteredOperation(t *testing.T) {
	root := filepath.Join("..", "..", "gauntlets", "repository_retrieval")
	cases, err := LoadRepositoryRetrievalCases(filepath.Join(root, "cases.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, hash, err := LoadRepositoryRetrievalLabels(filepath.Join(root, "labels.v1.json"), cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 12 || len(labels) != 12 || len(hash) != 64 {
		t.Fatalf("cases=%d labels=%d hash=%q", len(cases), len(labels), hash)
	}
	counts := make(map[assemblyline.RepositoryRetrievalOperation]int)
	for _, label := range labels {
		counts[label.Operation]++
	}
	for _, operation := range []assemblyline.RepositoryRetrievalOperation{
		assemblyline.RetrievalSemanticExcerpts, assemblyline.RetrievalSymbolDeclaration,
		assemblyline.RetrievalDirectReferences, assemblyline.RetrievalDiagnosticContext,
		assemblyline.RetrievalDependencyMetadata,
	} {
		if counts[operation] < 2 {
			t.Fatalf("operation %q count=%d want at least 2", operation, counts[operation])
		}
	}
}

func TestWriteRepositoryRetrievalResultRefusesToOverwriteEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	result := RepositoryRetrievalResult{
		Schema: RepositoryRetrievalResultSchemaV1,
		Report: RepositoryRetrievalReport{Schema: RepositoryRetrievalReportSchemaV1},
	}
	if err := WriteRepositoryRetrievalResult(path, result); err != nil {
		t.Fatal(err)
	}
	if err := WriteRepositoryRetrievalResult(path, result); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error=%v", err)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
