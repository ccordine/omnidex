package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTaskPublicationRetainsOnlyCompleteDependencyClosedDocuments(t *testing.T) {
	program := publicationProgramFixture(t)
	complete, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	delete(program.Generated, "feature.002")
	delete(program.Generated, "acceptance.002")
	stage, err := projectDirectCodingAcceptedTaskStage(program)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := directCodingTaskPublicationAssembly(program, stage)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"feature001.mjs", "feature001.test.mjs", "runtime.mjs", "package.json"} {
		got, err := directCodingAssemblyFile(publication, path)
		if err != nil {
			t.Fatal(err)
		}
		want, err := directCodingAssemblyFile(complete, path)
		if err != nil || string(got.Content) != string(want.Content) {
			t.Fatalf("published document %s differs from complete source: %v", path, err)
		}
	}
	for _, file := range publication.Files {
		if file.Path == "main.mjs" || strings.Contains(file.Path, "002") {
			t.Fatalf("unverified work was published: %s", file.Path)
		}
	}
	if len(publication.RequiredPaths) != 0 || len(publication.DeletePaths) != 0 {
		t.Fatal("task publication acquired unrelated filesystem obligations")
	}
}

func TestTaskPublicationDefersSharedDocumentsAndTheirDependents(t *testing.T) {
	program := publicationProgramFixture(t)
	var shared *assemblyline.SourceDocument
	for index := range program.Source.Documents {
		if program.Source.Documents[index].Path == "feature001.mjs" {
			shared = &program.Source.Documents[index]
		}
	}
	if shared == nil {
		t.Fatal("missing fixture document")
	}
	shared.Blocks = append(shared.Blocks, assemblyline.SourceBlock{
		ID: "support.second", Static: "const second = 2;", API: "const second = 2;",
		TaskID: "task_002", Role: assemblyline.SourceBlockTaskSupport,
	})
	implementation, verification := program.Generated["feature.002"], program.Generated["acceptance.002"]
	delete(program.Generated, "feature.002")
	delete(program.Generated, "acceptance.002")
	stage, err := projectDirectCodingAcceptedTaskStage(program)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := directCodingTaskPublicationAssembly(program, stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(publication.Files) != 0 {
		t.Fatalf("partial shared source or its dependent test escaped: %+v", publication.Files)
	}
	program.Generated["feature.002"], program.Generated["acceptance.002"] = implementation, verification
	stage, err = projectDirectCodingAcceptedTaskStage(program)
	if err != nil {
		t.Fatal(err)
	}
	publication, err = directCodingTaskPublicationAssembly(program, stage)
	if err != nil {
		t.Fatal(err)
	}
	file, err := directCodingAssemblyFile(publication, "feature001.mjs")
	if err != nil || !strings.Contains(string(file.Content), "const second = 2;") {
		t.Fatalf("completed shared document remained withheld: %v", err)
	}
}

func TestAcceptedTaskProjectionRejectsPartialOrUnknownGeneratedState(t *testing.T) {
	for _, defect := range []string{"partial", "unknown", "empty"} {
		t.Run(defect, func(t *testing.T) {
			program := publicationProgramFixture(t)
			switch defect {
			case "partial":
				delete(program.Generated, "acceptance.001")
			case "unknown":
				program.Generated["invented"] = "source"
			case "empty":
				program.Generated = make(map[string]string)
			}
			if _, err := projectDirectCodingAcceptedTaskStage(program); err == nil {
				t.Fatal("invalid accepted source advanced publication")
			}
		})
	}
}

func publicationProgramFixture(t *testing.T) directCodingProgram {
	t.Helper()
	return compiledLanguageBehaviorWorkloadFixture(t, genericJavaScriptCommandLineAdapter,
		[]compiledLanguageBehaviorFixture{
			{"Return the number of command-line arguments as text.",
				`return normalizeTaskResult({ output: String(input.arguments.length), error: "", exitCode: 0, state: {} });`,
				`const result = run({ arguments: ["one", "two"], standardInput: "" }, {}); assert.strictEqual(result.output, "2");`},
			{"Return standard-input text converted to uppercase.",
				`return normalizeTaskResult({ output: input.standardInput.toUpperCase(), error: "", exitCode: 0, state: {} });`,
				`const result = run({ arguments: [], standardInput: "Mixed" }, {}); assert.strictEqual(result.output, "MIXED");`},
		}, directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil})
}
