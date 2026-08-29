package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectCodingSourceHasNoLedgerReviewerOrVersionStore(t *testing.T) {
	for _, removed := range []string{
		"v3_coding_requirements.go", "v3_coding_assembly.go", "v3_source_worker.go",
		"v3_source_prompt.go", "v3_coding_diagnostic.go", "v3_coding_go_repair_routing.go",
		"v3_coding_code_envelope.go",
		"v3_coding_deployment_evidence.go",
		"../assemblyline/semantic_contract_schema.go", "../assemblyline/semantic_contract_validation.go",
	} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("removed coding path %s still exists", removed)
		}
	}
	paths, err := filepath.Glob("v3_coding_*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("direct coding source is missing")
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ToLower(string(raw))
		for _, forbidden := range []string{
			"implementationledger", "rejectedcandidate", "repaircycles",
			"file_reviewer", "failure_triager", "content_sha256",
			"directcodingdecision", "v3_coding_action_turn_",
			"coding_manifest_ready", "routing_requirements",
			"coding_file_envelope_normalized", "markdown_fence",
			"coding_requirement_router", "coding_test_router",
			"semantic_router", "coding_repair_routed",
			"choose one concrete next action",
			"directcodingmanifest", "builddirectcodingplanprompt",
			"directcodingrepairselection", "coding_plan_attempt",
			"file_content_turn", "session_accepted_files",
			"compiledirectcodingrequirements", "directcodingclirequirementpattern",
			"directcodinggoversionrequirementpattern", "directcodingatomicpersistencerequired",
			"plandrepair", "planrepair", "completed raw source", "complete raw source",
			"current_source", "v3_source_turn", "coding_source_review", "coding_source",
			"contextpaths", "filemodel(",
			"softwarecontractresponseschema", "decodedirectcodingsoftwarecontract",
			"semantictext", "qualitykind",
			"func directcodingverificationcommands(",
			"listcurrentevidencebyjob",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s retains obsolete mechanism %q", path, forbidden)
			}
		}
	}
}
