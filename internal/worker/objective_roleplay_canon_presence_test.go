package worker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonAbsenceSkipsInventory(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	var subjects []string
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode roleplayCanonRawLeafDecoder,
	) (any, objectiveStationReceipt, error) {
		subjects = append(subjects, subject)
		if subject != "roleplay_canon_fact_presence" {
			return nil, objectiveStationReceipt{Calls: 1}, fmt.Errorf(
				"absence opened forbidden canon leaf %q",
				subject,
			)
		}
		value, err := decode("B")
		return value, objectiveStationReceipt{Calls: 1}, err
	}

	decision, receipt, err := resolveRoleplayCanonCandidateQueueWithCall(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subjects, []string{"roleplay_canon_fact_presence"}) {
		t.Fatalf("canon absence subjects=%q", subjects)
	}
	if len(decision.Facts) != 0 || decision.Facts == nil {
		t.Fatalf("canon absence decision=%+v", decision)
	}
	if receipt.Calls != 1 {
		t.Fatalf("canon absence receipt=%+v", receipt)
	}
}

func TestRoleplayCanonPresenceOpensPositiveInventoryAndOrdinarySieve(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	var subjects []string
	authorizationCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode roleplayCanonRawLeafDecoder,
	) (any, objectiveStationReceipt, error) {
		subjects = append(subjects, subject)
		raw := ""
		switch subject {
		case "roleplay_canon_fact_presence":
			raw = "A"
		case "roleplay_canon_fact_inventory":
			raw = "Mara locks the observatory door.\nMara pockets the brass key."
		case "roleplay_canon_fact_candidate_authorization":
			authorizationCalls++
			raw = "A"
		case "roleplay_canon_fact_candidate_relation":
			raw = "B"
		default:
			return nil, objectiveStationReceipt{Calls: 1}, fmt.Errorf(
				"unexpected canon leaf %q",
				subject,
			)
		}
		value, err := decode(raw)
		return value, objectiveStationReceipt{Calls: 1}, err
	}

	decision, receipt, err := resolveRoleplayCanonCandidateQueueWithCall(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjects := []string{
		"roleplay_canon_fact_presence",
		"roleplay_canon_fact_inventory",
		"roleplay_canon_fact_candidate_authorization",
		"roleplay_canon_fact_candidate_authorization",
		"roleplay_canon_fact_candidate_relation",
	}
	if !reflect.DeepEqual(subjects, wantSubjects) {
		t.Fatalf("canon presence subjects=%q want=%q", subjects, wantSubjects)
	}
	if authorizationCalls != 2 || receipt.Calls != len(wantSubjects) ||
		len(decision.Facts) != 2 {
		t.Fatalf("canon presence decision=%+v receipt=%+v", decision, receipt)
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
