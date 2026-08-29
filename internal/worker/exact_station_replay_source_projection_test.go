package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestExactStationReplayProjectsRegisteredSourceDeclarationSpan(t *testing.T) {
	t.Parallel()
	jobs := []struct {
		name string
		job  assemblyline.PortableJob
		raw  string
		want string
		kind string
	}{
		{
			name: "plain text generation",
			job: mustReplayFragmentGenerationJob(t, assemblyline.FragmentGenerationInput{
				Language:  assemblyline.TextFragmentLanguage,
				Dialect:   assemblyline.TextFragmentDialect,
				Signature: assemblyline.TextFragmentSignature,
				Behavior:  "Return one greeting.",
			}),
			raw: "Hello from Omnidex.\n", want: "Hello from Omnidex.\n",
			kind: string(assemblyline.PortableResultProjectionExactResponse),
		},
		{
			name: "Go generation",
			job: mustReplayFragmentGenerationJob(t, assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
				Behavior: "Return one value.",
			}),
			raw:  "func Value() int { return 1 }",
			want: "func Value() int { return 1 }",
			kind: string(assemblyline.PortableResultProjectionSourceDeclaration),
		},
		{
			name: "JavaScript generation",
			job: mustReplayFragmentGenerationJob(t, assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function value()", Behavior: "Return one value.",
			}),
			raw:  "function value() {\r\n  return 1;\r\n}",
			want: "function value() {\r\n  return 1;\r\n}",
			kind: string(assemblyline.PortableResultProjectionSourceDeclaration),
		},
	}
	for _, fixture := range jobs {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			artifact, err := replayExactStationArtifact(fixture.job, fixture.raw)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Kind != fixture.kind ||
				artifact.Source != fixture.want ||
				artifact.StartByte != 0 || artifact.EndByte != len(fixture.raw) ||
				artifact.DiscardedBytes != 0 ||
				artifact.SourceSHA256 != replaySHA256(fixture.want) {
				t.Fatalf("artifact=%+v", artifact)
			}
		})
	}
}

func TestExactStationReplayRejectsPlainTextWithoutTerminalLF(t *testing.T) {
	t.Parallel()
	job := mustReplayFragmentGenerationJob(t, assemblyline.FragmentGenerationInput{
		Language:  assemblyline.TextFragmentLanguage,
		Dialect:   assemblyline.TextFragmentDialect,
		Signature: assemblyline.TextFragmentSignature,
		Behavior:  "Return one greeting.",
	})
	if _, err := replayExactStationArtifact(job, "Hello from Omnidex."); err == nil {
		t.Fatal("plain-text replay accepted a response without its terminal LF")
	}
}

func TestExactStationReplayProjectsGoModificationDeclaration(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewFragmentModificationJob(assemblyline.FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "Return two.",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := "func Value() int { return 2 }"
	artifact, err := replayExactStationArtifact(job, raw)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != string(assemblyline.PortableResultProjectionSourceDeclaration) ||
		artifact.Source != "func Value() int { return 2 }" || !artifact.ChangedFromBase ||
		!strings.Contains(raw, artifact.Source) {
		t.Fatalf("artifact=%+v", artifact)
	}
}

func TestExactStationReplayUsesPersistedLanguageBlindCorrectionProjection(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		projection string
		current    string
		raw        string
		want       string
	}{
		{"Go", "go", "func Value() int { return 1 }", "func Value() int { return 2 }", "func Value() int { return 2 }"},
		{"JavaScript", "javascript", "function summarize(values) { return 0; }", "function summarize(values) { return values.length; }", "function summarize(values) { return values.length; }"},
		{"Java", "java", "public int summarize(int value) { return 0; }", "public int summarize(int value) { return value; }", "public int summarize(int value) { return value; }"},
		{"Rust", "rust", "pub fn summarize(value: i32) -> i32 { 0 }", "pub fn summarize(value: i32) -> i32 { value }", "pub fn summarize(value: i32) -> i32 { value }"},
		{"PHP", "php", "function summarize(int $value): int { return 0; }", "function summarize(int $value): int { return $value; }", "function summarize(int $value): int { return $value; }"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			job, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
				assemblyline.FragmentCorrectionInput{
					CurrentDeclaration: fixture.current,
					RepairGuidance:     "Replace the returned literal with two.",
				},
				fixture.projection,
			)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := replayExactStationArtifact(job, fixture.raw)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Kind != string(assemblyline.PortableResultProjectionSourceDeclaration) ||
				artifact.Source != fixture.want || !artifact.ChangedFromBase ||
				artifact.StartByte != 0 || artifact.EndByte != len(fixture.raw) ||
				artifact.DiscardedBytes != 0 {
				t.Fatalf("artifact=%+v", artifact)
			}
		})
	}
}

func TestStationReplayLoadsPersistedLanguageBlindCorrectionProjection(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
		assemblyline.FragmentCorrectionInput{
			CurrentDeclaration: "func Value() int { return missing() }",
			RepairGuidance:     "Replace the missing call with the integer two.",
		},
		"go",
	)
	if err != nil {
		t.Fatal(err)
	}
	gap := replayTestGap(t, job)
	loaded, err := validateExactStationReplayPoint(queue.StationCallReplayPoint{
		Call: replayTestCall(t, gap),
		Gap:  gap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Job.ID != job.ID || loaded.Job.SourceProjection != "go" ||
		string(loaded.Job.Payload) != string(job.Payload) {
		t.Fatalf("loaded job=%+v, want %+v", loaded.Job, job)
	}
}

func TestExactStationReplayRejectsUnidentifiedLanguageBlindCorrection(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(assemblyline.FragmentCorrectionInput{
		CurrentDeclaration: "func Value() int { return missing() }",
		RepairGuidance:     "Replace the missing call with a local expression.",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = replayExactStationArtifact(assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV2,
		Kind:    assemblyline.WorkFragmentCorrection,
		Payload: payload,
	}, "func Value() int { return 2 }")
	if err == nil || !strings.Contains(err.Error(), "id does not match") {
		t.Fatalf("unbound correction replay error=%v", err)
	}
}

func TestExactStationReplayPreservesTypeScriptCodingArtifactKinds(t *testing.T) {
	t.Parallel()

	generation := mustReplayFragmentGenerationJob(t, assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript 5",
		Signature: "function value(): number", Behavior: "Return one value.",
	})
	functionRaw := "function value(): number { return 1; }"
	functionArtifact, err := replayExactStationArtifact(generation, functionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if functionArtifact.Kind != "typescript_function" ||
		functionArtifact.Source != "function value(): number { return 1; }" ||
		functionArtifact.Source != functionRaw[functionArtifact.StartByte:functionArtifact.EndByte] {
		t.Fatalf("function artifact=%+v", functionArtifact)
	}

	guidanceJob, err := assemblyline.NewTypeScriptRepairGuidanceJob(
		assemblyline.TypeScriptRepairGuidanceInput{
			Language: "typescript", Dialect: "TypeScript 5",
			Signature:          "function value(): number",
			CurrentDeclaration: "function value(): number { return missing; }",
			Diagnostic:         "error TS2304: Cannot find name 'missing'.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const instruction = "Replace the unresolved identifier with the numeric literal 1."
	guidanceRaw := instruction
	guidanceArtifact, err := replayExactStationArtifact(guidanceJob, guidanceRaw)
	if err != nil {
		t.Fatal(err)
	}
	if guidanceArtifact.Kind != "typescript_repair_guidance" ||
		guidanceArtifact.Source != instruction || guidanceArtifact.StartByte != 0 ||
		guidanceArtifact.EndByte != len(instruction) {
		t.Fatalf("guidance artifact=%+v", guidanceArtifact)
	}

	region := assemblyline.TypeScriptFragmentRepairRegion{
		Kind:      assemblyline.TypeScriptRepairRegionSyntaxWindow,
		StartLine: 2, EndLine: 2, Source: "  return missing;",
	}
	correction, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function value(): number",
		RepairRegion: &region, RepairGuidance: instruction,
	})
	if err != nil {
		t.Fatal(err)
	}
	regionRaw := "  return 1;"
	regionArtifact, err := replayExactStationArtifact(correction, regionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if regionArtifact.Kind != "typescript_repair_region" ||
		regionArtifact.Source != regionRaw || regionArtifact.StartByte != 0 ||
		regionArtifact.EndByte != len(regionRaw) || regionArtifact.DiscardedBytes != 0 ||
		!regionArtifact.ChangedFromBase {
		t.Fatalf("region artifact=%+v", regionArtifact)
	}
}

func TestExactStationReplayRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	_, err := replayExactStationArtifact(assemblyline.PortableJob{
		Schema: assemblyline.PortableJobSchemaV2,
		Kind:   assemblyline.WorkKind("future_station_kind"),
	}, "opaque response")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown-kind replay error=%v", err)
	}
}

func mustReplayFragmentGenerationJob(
	t *testing.T,
	input assemblyline.FragmentGenerationInput,
) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
