package worker

import (
	"os"
	"strings"
	"testing"
)

func TestRepositoryStagedCorrectionHasNoNarrativeOrWorkflowRestartPath(t *testing.T) {
	t.Parallel()
	correction := readRepositoryCorrectionSource(t, "v3_repository_change_correction.go")
	for _, forbidden := range []string{
		"RequirementQuote", "ReadExactSymbolSpan", "Workspace", "Snapshot",
		"WorkFragmentModification", "runDirectCodingGoFragmentModificationWorker",
	} {
		if strings.Contains(correction, forbidden) {
			t.Fatalf("repository correction source retains forbidden context path %q", forbidden)
		}
	}
	prepare := readRepositoryCorrectionSource(t, "v3_repository_change_prepare.go")
	for _, forbidden := range []string{
		"generateExistingRepositoryChangeCandidates(",
		"partitionCodingRequirements(",
		"runExistingRepositoryWorkflow(",
		"WorkRepositoryRetrieval",
		"WorkRepositoryChangeSurface",
	} {
		if strings.Contains(prepare, forbidden) {
			t.Fatalf("repository correction loop retains forbidden workflow restart %q", forbidden)
		}
	}
	apply := readRepositoryCorrectionSource(t, "v3_repository_change_apply.go")
	for _, forbidden := range []string{"changeapply.Plan(", "repositoryVerificationStaged"} {
		if strings.Contains(apply, forbidden) {
			t.Fatalf("repository apply bypasses the sole staged-proof preparation path %q", forbidden)
		}
	}
	commands := readRepositoryCorrectionSource(t, "v3_repository_change_commands.go")
	loop := strings.Index(commands, "for _, command := range commands")
	firstExactAssertion := strings.Index(commands, "assertExact(session.runtime.ctx)")
	execution := strings.Index(commands, "executeRepositoryGoVerification(")
	exitGate := strings.Index(commands, "requireRepositoryGoOrdinaryFailure(result)")
	classification := strings.Index(commands, "classifyRepositoryGoVerificationFailure(")
	exactAssertion := strings.LastIndex(commands, "assertExact(session.runtime.ctx)")
	acceptance := strings.Index(commands, "repositoryVerificationAcceptanceEvidence(authority")
	if loop < 0 || firstExactAssertion < loop || execution < firstExactAssertion {
		t.Fatal("repository commands are not each bound to exact bytes before execution")
	}
	if exitGate < 0 || classification < 0 || exitGate > classification {
		t.Fatal("repository correction classification is not guarded by exact exit-code-one authority")
	}
	if exactAssertion < 0 || acceptance < 0 || exactAssertion > acceptance {
		t.Fatal("repository plan acceptance is not guarded by the final exact-byte assertion")
	}
}

func readRepositoryCorrectionSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
