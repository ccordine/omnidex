package assemblyline

import "testing"

func TestRepositoryRequirementLeavesSeparateCoverageFromOneStatement(t *testing.T) {
	t.Parallel()
	input := repositoryRequirementLeafTestInput(t)
	coverage, err := DecodeRepositoryRequirementCoverageLeaf(
		input, RepositoryRequirementRemains,
	)
	if err != nil || coverage != RepositoryRequirementRemains {
		t.Fatalf("coverage=%q err=%v", coverage, err)
	}
	requirement, err := DecodeRepositoryRequirementLeaf(input, "Add audit logging.")
	if err != nil {
		t.Fatal(err)
	}
	input.AcceptedRequirements = []string{requirement}
	if _, err := DecodeRepositoryRequirementLeaf(input, requirement); err == nil {
		t.Fatal("duplicate repository requirement was accepted")
	}
	for _, structured := range []string{
		`{"requirement":"Add CSV export."}`,
		`["Add CSV export."]`,
		`"Add CSV export."`,
	} {
		if _, err := DecodeRepositoryRequirementLeaf(input, structured); err == nil {
			t.Fatalf("structured requirement %q was accepted", structured)
		}
	}
}

func repositoryRequirementLeafTestInput(t *testing.T) RepositoryRequirementLeafInput {
	t.Helper()
	request := "Add audit logging and CSV export to the service."
	context, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	return RepositoryRequirementLeafInput{
		Authority: RepositoryRequirementInterpretationInput{
			UserRequest: request, Context: context,
		},
		AcceptedRequirements: []string{},
	}
}
