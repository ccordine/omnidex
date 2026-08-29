package contextbuilder

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestProjectionValidationCrossChecksRenderedMaterial(t *testing.T) {
	t.Parallel()
	projection := testProjection(t)
	projection.Selected[0].ContentSHA256 = strings.Repeat("f", 64)
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); err == nil {
		t.Fatal("Validate accepted selected content hash that disagrees with rendered material")
	}
}

func TestProjectionRecordsSelectedAndOmittedSourceFreshness(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 3, MaxBytes: 4096})
	user := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	repository := acquireContextItem(t, set, "repository", workingset.RoleRepositoryEvidence, 80, "b")
	historical := acquireContextItem(t, set, "historical", workingset.RoleHistorical, 70, "c")
	spec := testSpec()
	spec.MaxItems = 1
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}
	spec.Optional = []Selector{{ID: "repository", Role: workingset.RoleRepositoryEvidence, MaxItems: 1}}

	projection, err := Build(BuildInput{
		WorkID: "work-1", Spec: spec, WorkingSet: set,
		Materials: []Material{
			contextMaterial(user, taskstate.AuthorityUser, "direct authority"),
			contextMaterial(repository, taskstate.AuthorityToolEvidence, "repository evidence"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Selected[0].SourceFreshness != SourceFreshnessValidatedCurrent {
		t.Fatalf("selected freshness=%q", projection.Selected[0].SourceFreshness)
	}
	omissions := make(map[workingset.ItemID]Omission, len(projection.Omitted))
	for _, omission := range projection.Omitted {
		omissions[omission.ItemID] = omission
	}
	if got := omissions[repository.ID]; got.Authority != taskstate.AuthorityToolEvidence ||
		got.SourceFreshness != SourceFreshnessValidatedCurrent || got.Reason != OmittedItemBudget {
		t.Fatalf("validated omission=%+v", got)
	}
	if got := omissions[historical.ID]; got.Authority != "" ||
		got.SourceFreshness != SourceFreshnessUnresolved || got.Reason != OmittedRoleNotSelected {
		t.Fatalf("unresolved omission=%+v", got)
	}
}

func TestProjectionBindsExactSelectedSourceLineage(t *testing.T) {
	t.Parallel()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 2, MaxBytes: 2048})
	item := acquireContextItem(t, set, "fact", workingset.RoleFact, 90, "a")
	material := contextMaterial(item, taskstate.AuthorityCode, "code-owned compact fact")
	source := taskstate.Ref{
		URI: "cognition:episode/e1/observation/o1", Version: "3",
		Hash: strings.Repeat("a", 64), Relation: taskstate.RefEvidence,
	}
	material.SourceRefs = []taskstate.Ref{source}
	spec := testSpec()
	spec.Required = []Selector{{ID: "fact", Role: workingset.RoleFact, MinItems: 1, MaxItems: 1}}
	projection, err := Build(BuildInput{WorkID: "lineage", Spec: spec, WorkingSet: set, Materials: []Material{material}})
	if err != nil {
		t.Fatal(err)
	}
	material.SourceRefs[0].Hash = strings.Repeat("b", 64)
	if len(projection.Selected) != 1 || len(projection.Selected[0].SourceRefs) != 1 ||
		projection.Selected[0].SourceRefs[0] != source {
		t.Fatalf("selected source lineage = %#v", projection.Selected)
	}
	projection.Selected[0].SourceRefs = nil
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("nil source lineage error = %v", err)
	}
}

func TestProjectionRejectsContradictoryOmissionFreshness(t *testing.T) {
	t.Parallel()
	projection := testProjection(t)
	projection.Omitted = append(projection.Omitted, Omission{
		ItemID: "missing", Ref: taskstate.Ref{
			URI: "repo:snapshot/symbol/missing", Version: "v1",
			Hash: strings.Repeat("d", 64), Relation: taskstate.RefSource,
		},
		Role: workingset.RoleRepositoryEvidence, SelectorID: "repository",
		Reason: OmittedMissingMaterial, Authority: taskstate.AuthorityToolEvidence,
		SourceFreshness: SourceFreshnessValidatedCurrent,
	})
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("contradictory omission error=%v, want ErrInvalidProjection", err)
	}
}

func TestProjectionRejectsTooManyEvidenceRecords(t *testing.T) {
	t.Parallel()
	projection := testProjection(t)
	projection.Omitted = make([]Omission, maxProjectionRecords)
	for index := range projection.Omitted {
		identity := strconv.Itoa(index)
		projection.Omitted[index] = Omission{
			ItemID: workingset.ItemID("omitted-" + identity),
			Ref: taskstate.Ref{
				URI: "repo:snapshot/symbol/" + identity, Version: "v1",
				Hash: strings.Repeat("d", 64), Relation: taskstate.RefSource,
			},
			Role: workingset.RoleHistorical, Reason: OmittedRoleNotSelected,
			SourceFreshness: SourceFreshnessUnresolved,
		}
	}
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("oversized evidence error=%v, want ErrInvalidProjection", err)
	}
}

func TestProjectionValidationRejectsDuplicateOrUnknownRecords(t *testing.T) {
	t.Parallel()
	projection := testProjection(t)
	projection.Selected = append(projection.Selected, projection.Selected[0])
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); err == nil {
		t.Fatal("Validate accepted duplicate selected item")
	}

	projection = testProjection(t)
	projection.Omitted = append(projection.Omitted, Omission{
		ItemID: "omitted", Ref: projection.Selected[0].Ref,
		Role: workingset.RoleHistorical, Reason: "unknown",
	})
	projection.ID = mustProjectionID(t, projection)
	if err := projection.Validate(); err == nil {
		t.Fatal("Validate accepted unknown omission reason")
	}
}

func testProjection(t *testing.T) Projection {
	t.Helper()
	set := testWorkingSet(t, workingset.Budget{MaxItems: 2, MaxBytes: 2048})
	item := acquireContextItem(t, set, "user", workingset.RoleUserAuthority, 100, "a")
	spec := testSpec()
	spec.Required = []Selector{{ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1}}
	projection, err := Build(BuildInput{
		WorkID: "work-1", Spec: spec, WorkingSet: set,
		Materials: []Material{contextMaterial(item, taskstate.AuthorityUser, "direct user authority")},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return projection
}

func mustProjectionID(t *testing.T, projection Projection) string {
	t.Helper()
	id, err := projectionID(projection)
	if err != nil {
		t.Fatalf("projectionID: %v", err)
	}
	return id
}
