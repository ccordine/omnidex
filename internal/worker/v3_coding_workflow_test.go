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
		},
	}
	summary, err := runDirectCodingWorkflow(driver, false)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "workflow complete" {
		t.Fatalf("summary=%q", summary)
	}
	if strings.Join(driver.generatedPaths, ",") != "service.go,store.go" {
		t.Fatalf("generated=%v", driver.generatedPaths)
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
		verification: directCodingVerification{Diagnostic: &directCodingDiagnostic{
			Stage: "verify", Command: "go test ./...", Detail: "unexpected authoritative source change",
		}},
	}
	_, err := runDirectCodingWorkflow(driver, false)
	if err == nil || !strings.Contains(err.Error(), "unexpected authoritative source change") {
		t.Fatalf("verification error=%v", err)
	}
	if driver.verifyCalls != 1 || len(driver.generatedPaths) != 1 {
		t.Fatalf("verify_calls=%d generated=%v", driver.verifyCalls, driver.generatedPaths)
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

func TestDirectCodingWorkflowReverifiesBeforeAcceptingPersistedVerification(t *testing.T) {
	driver := &workflowDriverStub{
		assembly: directCodingAssembly{
			VersionProfileID: goCommandLineVersionProfileV1,
			Files:            []directCodingFileTask{{Path: "main.go", Content: "package main\n"}},
		},
		unchanged:              true,
		beginVerificationState: directCodingCompletionTaskAlreadyDone,
		verification: directCodingVerification{
			Passed: true, TestsPassed: true, Commands: []string{"go test ./..."}, EvidenceIDs: []int64{12},
		},
	}
	if _, err := runDirectCodingWorkflow(driver, true); err != nil {
		t.Fatal(err)
	}
	if driver.beginVerificationCalls != 1 || driver.verifyCalls != 1 ||
		driver.finalizeCalls != 1 ||
		driver.finalizeBeginState != directCodingCompletionTaskAlreadyDone {
		t.Fatalf(
			"begin=%d verify=%d finalize=%d final_state=%q",
			driver.beginVerificationCalls, driver.verifyCalls,
			driver.finalizeCalls, driver.finalizeBeginState,
		)
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
	generatedPaths              []string
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

func (d *workflowDriverStub) Delete(string) (bool, error) { return true, nil }

func (d *workflowDriverStub) EnsureDirectory(string) (bool, error) { return true, nil }

func (d *workflowDriverStub) MaterializeTask(task directCodingFileTask) (bool, error) {
	d.generatedPaths = append(d.generatedPaths, task.Path)
	return !d.unchanged, nil
}

func (d *workflowDriverStub) BeginVerification() (directCodingCompletionTaskDisposition, error) {
	d.beginVerificationCalls++
	if d.beginVerificationState == "" {
		return directCodingCompletionTaskStarted, nil
	}
	return d.beginVerificationState, nil
}

func (d *workflowDriverStub) Verify() (directCodingVerification, error) {
	d.generatedBeforeVerification = len(d.generatedPaths)
	d.verifyCalls++
	if d.verification.Passed || d.verification.Diagnostic != nil {
		return d.verification, nil
	}
	return directCodingVerification{}, fmt.Errorf("verification result is not configured")
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
