package contextbuilder

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestBuildSelectsRequiredAuthorityBeforeBoundedOptionalContext(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 8, MaxBytes: 4096})
	user := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	constraint := acquireContextItem(t, set, "constraint", workingset.RoleConstraint, 90, "b")
	repository := acquireContextItem(t, set, "repository", workingset.RoleRepositoryEvidence, 80, "c")
	historical := acquireContextItem(t, set, "historical", workingset.RoleHistorical, 70, "d")

	spec := testSpec()
	spec.Required = []Selector{
		{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1},
		{ID: "constraint", Role: workingset.RoleConstraint, MinItems: 1, MaxItems: 1},
	}
	spec.Optional = []Selector{
		{ID: "repository", Role: workingset.RoleRepositoryEvidence, MaxItems: 1},
		{ID: "historical", Role: workingset.RoleHistorical, MaxItems: 1},
	}
	materials := []Material{
		contextMaterial(historical, taskstate.AuthorityCode, "old project note"),
		contextMaterial(repository, taskstate.AuthorityToolEvidence, "exact symbol evidence"),
		contextMaterial(constraint, taskstate.AuthorityUser, "preserve compatibility"),
		contextMaterial(user, taskstate.AuthorityUser, "change the invitation job"),
	}

	before := set.Version()
	projection, err := Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: materials})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if set.Version() != before {
		t.Fatalf("context build mutated working set: before=%d after=%d", before, set.Version())
	}
	want := []workingset.ItemID{"user", "constraint", "repository", "historical"}
	if len(projection.Selected) != len(want) {
		t.Fatalf("selected=%#v", projection.Selected)
	}
	for index, id := range want {
		if projection.Selected[index].ItemID != id {
			t.Fatalf("selected[%d]=%q, want %q", index, projection.Selected[index].ItemID, id)
		}
	}
	if projection.RenderedBytes != len([]byte(projection.Rendered)) || projection.RenderedSHA256 == "" {
		t.Fatalf("render evidence is inconsistent: %#v", projection)
	}
	if err := projection.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	reversed := append([]Material(nil), materials...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: reversed})
	if err != nil {
		t.Fatalf("Build reversed: %v", err)
	}
	if again.ID != projection.ID || again.Rendered != projection.Rendered {
		t.Fatalf("material order changed projection identity: %q != %q", again.ID, projection.ID)
	}
}

func TestBuildFailsWhenRequiredSelectorCannotResolve(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 2, MaxBytes: 1024})
	item := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	spec := testSpec()
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}

	_, err := Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set})
	if !errors.Is(err, ErrRequiredSelector) {
		t.Fatalf("missing material error=%v, want ErrRequiredSelector", err)
	}

	material := contextMaterial(item, taskstate.AuthorityModelProposal, "unaccepted proposal")
	_, err = Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: []Material{material}})
	if !errors.Is(err, ErrRequiredSelector) {
		t.Fatalf("disallowed authority error=%v, want ErrRequiredSelector", err)
	}
}

func TestBuildRejectsStaleOrInjectedMaterial(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 2, MaxBytes: 1024})
	item := acquireContextItem(t, set, "repository", workingset.RoleRepositoryEvidence, 80, "a")
	spec := testSpec()
	spec.Required = []Selector{{ID: "repository", Role: workingset.RoleRepositoryEvidence, MinItems: 1, MaxItems: 1}}
	material := contextMaterial(item, taskstate.AuthorityToolEvidence, "source")
	material.CurrentRef.Hash = strings.Repeat("f", 64)

	_, err := Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: []Material{material}})
	if !errors.Is(err, ErrStaleReference) {
		t.Fatalf("stale error=%v, want ErrStaleReference", err)
	}

	material = contextMaterial(item, taskstate.AuthorityToolEvidence, "source")
	material.ItemID = "not-resident"
	_, err = Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: []Material{material}})
	if !errors.Is(err, ErrMaterialMismatch) {
		t.Fatalf("injected error=%v, want ErrMaterialMismatch", err)
	}
}

func TestBuildRejectsMaterialPostgreSQLCannotPersist(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 1, MaxBytes: 1024})
	item := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	spec := testSpec()
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}
	for _, content := range []string{"forbidden\x00content", string([]byte{0xff})} {
		material := contextMaterial(item, taskstate.AuthorityUser, content)
		_, err := Build(BuildInput{
			WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: []Material{material},
		})
		if !errors.Is(err, ErrMaterialMismatch) {
			t.Fatalf("content %q error=%v, want ErrMaterialMismatch", content, err)
		}
	}
}

func TestRequiredContentNeverFallsOutOfBudget(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 2, MaxBytes: 4096})
	item := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	spec := testSpec()
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}
	spec.MaxBytes = 128
	material := contextMaterial(item, taskstate.AuthorityUser, strings.Repeat("x", 512))
	material.ByteCost = 512

	_, err := Build(BuildInput{WorkID: "work-1", Spec: spec, WorkingSet: set, Materials: []Material{material}})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("budget error=%v, want ErrBudgetExceeded", err)
	}
}

func TestRequiredSelectorReservesOnlyItsMinimumBeforeOptionalExtras(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 3, MaxBytes: 4096})
	primary := acquireContextItem(t, set, "user-primary", workingset.RoleUserAuthority, 100, "a")
	extra := acquireContextItem(t, set, "user-extra", workingset.RoleUserAuthority, 90, "b")
	constraint := acquireContextItem(t, set, "constraint", workingset.RoleConstraint, 80, "c")
	spec := testSpec()
	spec.MaxItems = 2
	spec.Required = []Selector{
		{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 2},
		{ID: "constraint", Role: workingset.RoleConstraint, MinItems: 1, MaxItems: 1},
	}
	projection, err := Build(BuildInput{
		WorkID: "work-1", Spec: spec, WorkingSet: set,
		Materials: []Material{
			contextMaterial(primary, taskstate.AuthorityUser, "primary authority"),
			contextMaterial(extra, taskstate.AuthorityUser, "secondary authority"),
			contextMaterial(constraint, taskstate.AuthorityUser, "required constraint"),
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(projection.Selected) != 2 || projection.Selected[0].ItemID != primary.ID ||
		projection.Selected[1].ItemID != constraint.ID {
		t.Fatalf("required minima were not reserved: %#v", projection.Selected)
	}
	if len(projection.Omitted) != 1 || projection.Omitted[0].ItemID != extra.ID ||
		projection.Omitted[0].Reason != OmittedItemBudget {
		t.Fatalf("required extra omission=%#v", projection.Omitted)
	}
}

func TestOptionalItemsAreOmittedWithTypedReasons(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 3, MaxBytes: 4096})
	user := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	repository := acquireContextItem(t, set, "repository", workingset.RoleRepositoryEvidence, 80, "b")
	historical := acquireContextItem(t, set, "historical", workingset.RoleHistorical, 70, "c")
	spec := testSpec()
	spec.MaxItems = 1
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}
	spec.Optional = []Selector{
		{ID: "repository", Role: workingset.RoleRepositoryEvidence, MaxItems: 1},
		{ID: "historical", Role: workingset.RoleHistorical, MaxItems: 1},
	}
	projection, err := Build(BuildInput{
		WorkID: "work-1", Spec: spec, WorkingSet: set,
		Materials: []Material{
			contextMaterial(user, taskstate.AuthorityUser, "user authority"),
			contextMaterial(repository, taskstate.AuthorityToolEvidence, "repository evidence"),
			contextMaterial(historical, taskstate.AuthorityModelProposal, "unaccepted history"),
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(projection.Selected) != 1 || len(projection.Omitted) != 2 {
		t.Fatalf("projection selections=%#v omissions=%#v", projection.Selected, projection.Omitted)
	}
	got := map[workingset.ItemID]OmissionReason{}
	for _, omission := range projection.Omitted {
		got[omission.ItemID] = omission.Reason
	}
	if got["repository"] != OmittedItemBudget || got["historical"] != OmittedAuthority {
		t.Fatalf("omission reasons=%#v", got)
	}
	if strings.Contains(projection.Rendered, "repository evidence") || strings.Contains(projection.Rendered, "unaccepted history") {
		t.Fatal("omitted material leaked into rendered context")
	}
}

func testWorkingSet(t *testing.T, budget workingset.Budget) *workingset.Set {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 1,
		RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.New(workingset.Owner{LedgerID: ledgerID, JobID: 1, Generation: 1}, budget)
	if err != nil {
		t.Fatalf("new working set: %v", err)
	}
	return set
}

func acquireContextItem(
	t *testing.T,
	set *workingset.Set,
	id workingset.ItemID,
	role workingset.Role,
	priority int,
	hashCharacter string,
) workingset.Item {
	t.Helper()
	request := workingset.AcquireRequest{
		ID: id,
		Ref: taskstate.Ref{
			URI: "task:job/1/entry/" + string(id), Version: "v1",
			Hash: strings.Repeat(hashCharacter, 64), Relation: taskstate.RefSource,
		},
		Role: role, Retention: workingset.RetentionJob,
		Scope:    set.Scope(),
		Priority: priority, ByteCost: 1024,
		Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: "op-" + string(id), Reason: "test acquisition",
		},
	}
	result, err := set.Acquire(request)
	if err != nil {
		t.Fatalf("acquire %s: %v", id, err)
	}
	return result.Item
}

func contextMaterial(item workingset.Item, authority taskstate.Authority, content string) Material {
	return Material{
		ItemID: item.ID, CurrentRef: item.Ref, SourceRefs: []taskstate.Ref{}, Authority: authority,
		Content: content, ByteCost: len([]byte(content)),
	}
}

func testSpec() ContextSpec {
	return ContextSpec{
		Name: "repository-investigation", Version: "v1",
		ScopeRef: taskstate.Ref{
			URI: "task:job/1/node/task-1", Version: "v1",
			Hash: strings.Repeat("e", 64), Relation: taskstate.RefConcerns,
		},
		AllowedAuthorities: []taskstate.Authority{
			taskstate.AuthorityUser,
			taskstate.AuthorityCode,
			taskstate.AuthorityToolEvidence,
			taskstate.AuthorityAcceptedModelDecision,
		},
		MaxItems: 8, MaxBytes: 4096, MaxAcquisitionRounds: 2,
	}
}
