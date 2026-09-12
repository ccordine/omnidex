package worker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonEmptyInventorySkipsCandidateCalls(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	var subjects []string
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode roleplayCanonRawLeafDecoder,
	) (any, int, error) {
		subjects = append(subjects, subject)
		if subject != "roleplay_canon_fact_inventory" {
			return nil, 1, fmt.Errorf(
				"absence opened forbidden canon leaf %q",
				subject,
			)
		}
		value, err := decode("NO_CANON_FACT_CANDIDATES")
		return value, 1, err
	}

	decision, dispatches, err := resolveRoleplayCanonCandidateQueueWithCall(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subjects, []string{"roleplay_canon_fact_inventory"}) {
		t.Fatalf("canon absence subjects=%q", subjects)
	}
	if len(decision.Facts) != 0 || decision.Facts == nil {
		t.Fatalf("canon absence decision=%+v", decision)
	}
	if dispatches != 1 {
		t.Fatalf("canon absence dispatches=%+v", dispatches)
	}
}

func TestRoleplayCanonInventoryDirectlyEntersTheOrdinarySieve(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	var subjects []string
	authorizationCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode roleplayCanonRawLeafDecoder,
	) (any, int, error) {
		subjects = append(subjects, subject)
		raw := ""
		switch subject {
		case "roleplay_canon_fact_inventory":
			raw = "Mara locks the observatory door.\nMara pockets the brass key."
		case "roleplay_canon_fact_candidate_authorization":
			authorizationCalls++
			raw = "A"
		case "roleplay_canon_fact_candidate_relation":
			raw = "B"
		default:
			return nil, 1, fmt.Errorf(
				"unexpected canon leaf %q",
				subject,
			)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	decision, dispatches, err := resolveRoleplayCanonCandidateQueueWithCall(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjects := []string{
		"roleplay_canon_fact_inventory",
		"roleplay_canon_fact_candidate_authorization",
		"roleplay_canon_fact_candidate_authorization",
		"roleplay_canon_fact_candidate_relation",
	}
	if !reflect.DeepEqual(subjects, wantSubjects) {
		t.Fatalf("canon inventory subjects=%q want=%q", subjects, wantSubjects)
	}
	if authorizationCalls != 2 || dispatches != len(wantSubjects) ||
		len(decision.Facts) != 2 {
		t.Fatalf("canon inventory decision=%+v dispatches=%+v", decision, dispatches)
	}
}

func TestRoleplayCanonMalformedInventoryStopsWithoutAnotherCall(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	for _, raw := range []string{"", "NO_CANON_FACT_CANDIDATES\nMara locks the observatory door."} {
		calls := 0
		call := func(_ context.Context, subject string, _ assemblyline.PortableJob, decode roleplayCanonRawLeafDecoder) (any, int, error) {
			calls++
			if subject != "roleplay_canon_fact_inventory" {
				t.Errorf("malformed inventory opened unexpected leaf %q", subject)
			}
			value, err := decode(raw)
			return value, 1, err
		}
		decision, dispatches, err := resolveRoleplayCanonCandidateQueueWithCall(context.Background(), input, call)
		if err == nil || calls != 1 || dispatches != 1 || !reflect.DeepEqual(decision, assemblyline.RoleplayCanonExtractionDecision{}) {
			t.Fatalf("raw=%q decision=%+v dispatches=%+v calls=%d error=%v", raw, decision, dispatches, calls, err)
		}
	}
}

func TestRoleplayCanonCallCountBudgetsOnlyOneInventory(t *testing.T) {
	count := assemblyline.MaxRoleplayCanonFactsPerTurn
	maximum := (1 + count + count*(count-1)/2) * exactSemanticLeafCalls
	for _, dispatches := range []int{0, 1, maximum} {
		if err := validateRoleplayCanonExtractionCalls(dispatches); err != nil {
			t.Fatalf("valid dispatches=%+v: %v", dispatches, err)
		}
	}
	for _, dispatches := range []int{-1, maximum + 1} {
		if err := validateRoleplayCanonExtractionCalls(dispatches); err == nil {
			t.Fatalf("invalid dispatches accepted: %+v", dispatches)
		}
	}
}

func TestRoleplayTypedDirectionsAndCommandsSkipCanonSource(t *testing.T) {
	for _, authority := range []roleplay.UserTurnAuthority{
		{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionDirection,
			ExactText:        "Have Mara open the observatory door.",
		},
		{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
			ExactText:        "/leave",
		},
	} {
		if _, present, err := assemblyline.ProjectRoleplayUserCanonSource(authority); err != nil || present {
			t.Fatalf("typed %s source present=%t error=%v", authority.ContributionKind, present, err)
		}
	}
}

func roleplayCanonWorkerTestInput() assemblyline.RoleplayCanonExtractionInput {
	return assemblyline.RoleplayCanonExtractionInput{
		Source: assemblyline.RoleplayCanonSource{
			Kind:                  assemblyline.RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Mara",
			ExactContribution:     "I lock the observatory door and pocket the brass key.",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionDialogue,
		},
		Context: assemblyline.ObjectiveContext{},
	}
}
