package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestDirectCodingWorkflowWritesOnlyACompletedAssemblyThenVerifiesOnce(t *testing.T) {
	driver := &workflowDriverStub{
		assembly: directCodingAssembly{VersionProfileID: goCommandLineVersionProfileV1, Files: []directCodingFileTask{
			{Path: "service.go", Content: "package main\n"},
			{Path: "store.go", Content: "package main\n"},
		}},
		verification: directCodingVerification{
			Passed: true, TestsPassed: true, Commands: []string{"go test ./..."}, EvidenceIDs: []int64{11},
			MutationOperationID: "workspace_mutation_" + strings.Repeat("a", 64), MutationReceiptSHA256: strings.Repeat("b", 64),
		},
	}
	summary, err := runDirectCodingWorkflow(driver, false)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "workflow complete" {
		t.Fatalf("summary=%q", summary)
	}
	if strings.Join(driver.preparedPaths, ",") != "service.go,store.go" {
		t.Fatalf("prepared=%v", driver.preparedPaths)
	}
	if driver.generatedBeforeVerification != 2 || driver.beginVerificationCalls != 1 ||
		driver.verifyCalls != 1 || driver.finalizeCalls != 1 {
		t.Fatalf("generated_before_verify=%d begin_calls=%d verify_calls=%d finalize_calls=%d", driver.generatedBeforeVerification, driver.beginVerificationCalls, driver.verifyCalls, driver.finalizeCalls)
	}
	if got := strings.Join(driver.phases, ","); got != "assembling,constructing,verifying,completed" {
		t.Fatalf("phases=%q", got)
	}
}

func TestDirectCodingWorkflowFailsLoudlyInsteadOfStartingAFileRepairAgent(t *testing.T) {
	driver := &workflowDriverStub{
		assembly: directCodingAssembly{VersionProfileID: goCommandLineVersionProfileV1, Files: []directCodingFileTask{{
			Path: "main.go", Content: "package main\n",
		}}},
		verification: directCodingVerification{
			MutationOperationID: "workspace_mutation_" + strings.Repeat("a", 64), MutationReceiptSHA256: strings.Repeat("b", 64),
			Diagnostic: &directCodingDiagnostic{
				Stage: "verify", Command: "go test ./...", Detail: "unexpected authoritative source change",
			},
		},
	}
	_, err := runDirectCodingWorkflow(driver, false)
	if err == nil || !strings.Contains(err.Error(), "unexpected authoritative source change") {
		t.Fatalf("verification error=%v", err)
	}
	if driver.verifyCalls != 1 || len(driver.preparedPaths) != 1 {
		t.Fatalf("verify_calls=%d prepared=%v", driver.verifyCalls, driver.preparedPaths)
	}
	if got := strings.Join(driver.phases, ","); got != "assembling,constructing,verifying,failed" {
		t.Fatalf("phases=%q", got)
	}
}

func TestDirectCodingWorkflowRejectsFreshNoMutationAssembly(t *testing.T) {
	driver := &workflowDriverStub{
		assembly: directCodingAssembly{VersionProfileID: goCommandLineVersionProfileV1, Files: []directCodingFileTask{{
			Path: "main.go", Content: "package main\n",
		}}},
		unchanged: true,
	}
	_, err := runDirectCodingWorkflow(driver, false)
	if err == nil || !strings.Contains(err.Error(), "no workspace mutation") {
		t.Fatalf("fresh no-mutation error=%v", err)
	}
	if driver.verifyCalls != 0 {
		t.Fatalf("fresh no-op unexpectedly verified %d times", driver.verifyCalls)
	}
}

func TestDirectCodingWorkflowRejectsUnjournaledPersistedNoOp(t *testing.T) {
	driver := &workflowDriverStub{
		assembly: directCodingAssembly{
			VersionProfileID: goCommandLineVersionProfileV1,
			Files:            []directCodingFileTask{{Path: "main.go", Content: "package main\n"}},
		},
		unchanged:              true,
		beginVerificationState: directCodingCompletionTaskAlreadyDone,
		verification: directCodingVerification{
			Passed: true, TestsPassed: true, Commands: []string{"go test ./..."}, EvidenceIDs: []int64{12},
			MutationOperationID: "workspace_mutation_" + strings.Repeat("a", 64), MutationReceiptSHA256: strings.Repeat("b", 64),
		},
	}
	if _, err := runDirectCodingWorkflow(driver, true); err == nil ||
		!strings.Contains(err.Error(), "requires one journaled workspace mutation") {
		t.Fatalf("persisted no-op error=%v", err)
	}
	if driver.beginVerificationCalls != 0 || driver.verifyCalls != 0 || driver.finalizeCalls != 0 {
		t.Fatalf("begin=%d verify=%d finalize=%d", driver.beginVerificationCalls, driver.verifyCalls, driver.finalizeCalls)
	}
}

func TestDirectCodingEventTokenPreservesCompleteCommandAsOneField(t *testing.T) {
	if got := directCodingEventToken("go test ./...", "unknown"); got != "go_test_./..." {
		t.Fatalf("event token=%q", got)
	}
}

func TestDirectCodingVerificationRequiresExactOrderedEvidenceIdentities(t *testing.T) {
	diagnostic := &directCodingDiagnostic{Stage: "verify", Detail: "failed"}
	for _, invalid := range []directCodingVerification{
		{Passed: true, Commands: []string{"go test"}},
		{Commands: []string{"go test"}, EvidenceIDs: nil, Diagnostic: diagnostic},
		{Commands: []string{"go test", "go vet"}, EvidenceIDs: []int64{12, 11}, Diagnostic: diagnostic},
	} {
		if err := invalid.validate(); err == nil {
			t.Fatalf("verification accepted non-exact evidence identities: %+v", invalid)
		}
	}
	valid := directCodingVerification{
		Commands: []string{"go test"}, EvidenceIDs: []int64{11}, Diagnostic: diagnostic,
		MutationOperationID: "workspace_mutation_" + strings.Repeat("a", 64), MutationReceiptSHA256: strings.Repeat("b", 64),
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("exact failed verification proof error=%v", err)
	}
}

type workflowDriverStub struct {
	assembly                    directCodingAssembly
	verification                directCodingVerification
	unchanged                   bool
	phases                      []string
	preparedPaths               []string
	generatedBeforeVerification int
	beginVerificationCalls      int
	beginVerificationState      directCodingCompletionTaskDisposition
	verifyCalls                 int
	finalizeCalls               int
	finalizeBeginState          directCodingCompletionTaskDisposition
}

func (d *workflowDriverStub) Phase(phase directCodingPhase, _ string) {
	d.phases = append(d.phases, string(phase))
}

func (d *workflowDriverStub) Assemble() (directCodingAssembly, error) {
	return d.assembly, nil
}

func (d *workflowDriverStub) PrepareAssembly(
	assembly directCodingAssembly,
) (*directCodingPreparedMutation, error) {
	for _, task := range assembly.Files {
		d.preparedPaths = append(d.preparedPaths, task.Path)
	}
	mutations := len(assembly.Files) + len(assembly.DeletePaths)
	if d.unchanged {
		mutations = 0
	}
	return &directCodingPreparedMutation{mutationCount: mutations}, nil
}

func (d *workflowDriverStub) ApplyAndVerify(
	_ *directCodingPreparedMutation,
) (directCodingVerification, directCodingCompletionTaskDisposition, error) {
	d.generatedBeforeVerification = len(d.preparedPaths)
	d.beginVerificationCalls++
	d.verifyCalls++
	beginState := d.beginVerificationState
	if beginState == "" {
		beginState = directCodingCompletionTaskStarted
	}
	if d.verification.Passed || d.verification.Diagnostic != nil {
		return d.verification, beginState, nil
	}
	return directCodingVerification{}, "", fmt.Errorf("verification result is not configured")
}

func (d *workflowDriverStub) FinalizeVerified(
	verification directCodingVerification,
	beginState directCodingCompletionTaskDisposition,
) error {
	if !verification.Passed {
		return fmt.Errorf("cannot finalize failed verification")
	}
	d.finalizeCalls++
	d.finalizeBeginState = beginState
	return nil
}

func (d *workflowDriverStub) Complete(directCodingVerification) (string, error) {
	return "workflow complete", nil
}
