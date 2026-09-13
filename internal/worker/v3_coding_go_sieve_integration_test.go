package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaExactGoProgramPassesRecordedFocusedAndFinalSieves(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for recorded Go sieve coverage")
	}
	rejectNativeCompiledLanguageTools(t)
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	hostRoot := t.TempDir()
	job, err := repository.EnqueueCodingJob(ctx, "exercise exact assembled Go sieve", hostRoot)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "go-sieve-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	program := testExactGoSieveProgram(t)
	runtime := &nativeRuntimeV3{
		svc: &Service{repo: repository}, ctx: ctx, claim: claim,
	}
	session := &directCodingSession{runtime: runtime, root: hostRoot, program: &program}
	workspace, err := newDirectCodingGoStageWorkspace(session, program)
	if err != nil {
		t.Fatalf("open exact Go stage: %v", err)
	}
	defer func() {
		if err := workspace.Close(); err != nil {
			t.Errorf("close exact Go stage: %v", err)
		}
	}()
	contextAuthority, err := assemblyline.ProjectApplicationTaskContext(
		program.Workload, program.Workload.Tasks[0].ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, contextAuthority)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.VerifyTask(&stage, contextAuthority, "TestFeature001"); err != nil {
		t.Fatalf("run focused Go sieve: %v", err)
	}
	if err := workspace.VerifyFinal(&program); err != nil {
		t.Fatalf("run final Go sieve: %v", err)
	}
	evidence, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		phase queue.VerificationCommandPhase
		argv  []string
	}{
		{queue.VerificationIsolatedInstall, []string{"go", "version"}},
		{queue.VerificationIsolatedTask, directCodingGoSourceResetCommand().Argv},
		{queue.VerificationIsolatedTask, []string{"gofmt", "-d", "--", "feature001.go", "feature001_test.go", "runtime.go"}},
		{queue.VerificationIsolatedTask, []string{"go", "test", "-count=1", "-run", "^TestFeature001$", "./..."}},
		{queue.VerificationIsolatedFinal, directCodingGoSourceResetCommand().Argv},
		{queue.VerificationIsolatedFinal, []string{"gofmt", "-d", "--", "feature001.go", "feature001_test.go", "main.go", "runtime.go"}},
		{queue.VerificationIsolatedFinal, []string{"go", "test", "-count=1", "./..."}},
		{queue.VerificationIsolatedFinal, []string{"go", "vet", "./..."}},
		{queue.VerificationIsolatedFinal, []string{"go", "build", "-o", filepath.Join(workspace.outputRoot, "application"), "./..."}},
	}
	if len(evidence) != len(want) {
		t.Fatalf("recorded Go commands=%d; want %d: %#v", len(evidence), len(want), evidence)
	}
	for index, expected := range want {
		actual := evidence[index]
		if actual.ContainerID == "" || actual.ContainerImageID == "" || actual.ContainerExecID == "" || actual.WorkingDirectory != "/workspace" {
			t.Fatalf("Go command lacks Docker authority: %+v", actual)
		}
		if actual.Phase != expected.phase || !sameExactStrings(actual.Argv, expected.argv) ||
			actual.Status != queue.VerificationCommandSucceeded {
			t.Fatalf("Go command %d=%#v; want phase=%s argv=%v success", index+1, actual, expected.phase, expected.argv)
		}
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	assertCompiledLanguageContainersRemoved(t, evidence)
}

func testExactGoSieveProgram(t *testing.T) directCodingProgram {
	t.Helper()
	const requirement = "Write ready to standard output."
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "one exact Go command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: requirement,
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	var profile directCodingProjectVersionProfile
	for _, candidate := range registeredDirectCodingProjectVersionProfiles() {
		if candidate.ID == goCommandLineVersionProfileV1 {
			profile = candidate
			break
		}
	}
	if profile.ID == "" {
		t.Fatal("registered Go version profile is absent")
	}
	target, coverage, err := resolveDirectCodingTargetTree(
		specification, workload, stack, nil, directCodingTargetTreeOccupation{},
	)
	if err != nil {
		t.Fatal(err)
	}
	dialect, err := directCodingProjectSourceDialect(profile)
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(
		specification, workload,
		directCodingCapabilityGraph{"requirement_001": nil},
		directCodingProjectSelection{Stack: stack, Profile: profile, Dialect: dialect, ResultValueKinds: directCodingResultValueKindPlan{"requirement_001": assemblyline.ApplicationResultText}, InputSources: directCodingInputSourcePlan{"requirement_001": assemblyline.ApplicationInputArguments}},
		target, coverage, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	relationAuthority := directCodingResultRelationAuthorityFixture(t, requirement)
	derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
		assemblyline.ApplicationRequirementCandidateResultPresenceInput{
			Candidate: relationAuthority.Candidate,
			Kind:      relationAuthority.Kind, Cardinality: relationAuthority.Cardinality,
			Dimension: assemblyline.ApplicationRequirementDerivedValueDimension,
		},
		"B",
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
		relationAuthority, derived, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.RequirementRelations, err = newDirectCodingApplicationTaskResultRelationPlan(
		workload,
		[]assemblyline.ApplicationRequirement{{
			ID: "requirement_001", Statement: requirement,
			ResultRelation: receipt,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated["feature.001"] = `func Feature001(arguments []string) string { return "ready" }`
	program.Generated["acceptance.input.001"] = `func MakeExampleInputFeature001() []string { return []string{} }`
	program.Generated["acceptance.001"] = `func ExpectedFeature001(arguments []string) string { return "ready" }`
	return program
}
