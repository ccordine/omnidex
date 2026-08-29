package roleplay

import (
	"reflect"
	"testing"
)

const (
	initiativeA = "rpc_11111111111111111111111111111111"
	initiativeB = "rpc_22222222222222222222222222222222"
	initiativeC = "rpc_33333333333333333333333333333333"
)

func TestSimulationResponderOrderStartsAtInitiativeCursorAndSkipsActingCharacter(t *testing.T) {
	participants := []string{initiativeA, initiativeB, initiativeC}
	tests := []struct {
		name     string
		active   string
		userTurn UserTurnAuthority
		want     []string
	}{
		{name: "first cursor narrator", active: initiativeA, userTurn: narratorDirectionTurnForInitiative(), want: []string{initiativeA, initiativeB, initiativeC}},
		{name: "middle cursor narrator", active: initiativeB, userTurn: narratorDirectionTurnForInitiative(), want: []string{initiativeB, initiativeC, initiativeA}},
		{name: "last cursor narrator", active: initiativeC, userTurn: narratorDirectionTurnForInitiative(), want: []string{initiativeC, initiativeA, initiativeB}},
		{name: "active acting character skipped", active: initiativeA, userTurn: characterTurnForInitiative(initiativeA), want: []string{initiativeB, initiativeC}},
		{name: "acting character skipped after cursor rotation", active: initiativeC, userTurn: characterTurnForInitiative(initiativeA), want: []string{initiativeC, initiativeB}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := simulationResponderIDs(participants, test.active, test.userTurn)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("responders=%#v want %#v", got, test.want)
			}
		})
	}
	if _, err := simulationResponderIDs(participants, "rpc_ffffffffffffffffffffffffffffffff", narratorDirectionTurnForInitiative()); err == nil {
		t.Fatal("absent initiative cursor unexpectedly produced a response order")
	}
	if _, err := simulationResponderIDs(
		[]string{initiativeA}, initiativeA, characterTurnForInitiative(initiativeA),
	); err == nil {
		t.Fatal("sole acting character unexpectedly produced an empty response round")
	}
}

func TestSimulationInitiativeAdvancesOneCursorAndMonotonicTickPerCompletedRound(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{
			ActiveCharacterID: initiativeA,
			Initiative:        SimulationInitiativeClock{Round: 1, Turn: 1, FictionalTimeTick: 0},
		},
		Participants: []SceneParticipantProjection{
			{CharacterID: initiativeA, TurnPosition: 0},
			{CharacterID: initiativeB, TurnPosition: 1},
			{CharacterID: initiativeC, TurnPosition: 2},
		},
	}
	wantCharacters := []string{initiativeB, initiativeC, initiativeA}
	wantClocks := []SimulationInitiativeClock{
		{Round: 1, Turn: 2, FictionalTimeTick: 1},
		{Round: 1, Turn: 3, FictionalTimeTick: 2},
		{Round: 2, Turn: 4, FictionalTimeTick: 3},
	}
	for index := range wantCharacters {
		next, clock, err := advanceSimulationInitiative(scene, "")
		if err != nil {
			t.Fatal(err)
		}
		if next != wantCharacters[index] || clock != wantClocks[index] {
			t.Fatalf("advance %d character/clock=%q/%+v want %q/%+v", index, next, clock, wantCharacters[index], wantClocks[index])
		}
		scene.Sheet.ActiveCharacterID = next
		scene.Sheet.Initiative = clock
	}
}

func TestSimulationInitiativeSkipsActingCharacterAndFailsAtClockBound(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{
			ActiveCharacterID: initiativeA,
			Initiative:        SimulationInitiativeClock{Round: 4, Turn: 9, FictionalTimeTick: 8},
		},
		Participants: []SceneParticipantProjection{
			{CharacterID: initiativeA, TurnPosition: 0},
			{CharacterID: initiativeB, TurnPosition: 1},
			{CharacterID: initiativeC, TurnPosition: 2},
		},
	}
	next, clock, err := advanceSimulationInitiative(scene, initiativeB)
	if err != nil {
		t.Fatal(err)
	}
	if next != initiativeC || clock != (SimulationInitiativeClock{Round: 4, Turn: 10, FictionalTimeTick: 9}) {
		t.Fatalf("skipped initiative=%q/%+v", next, clock)
	}
	scene.Sheet.Initiative.Turn = MaxSimulationInitiativeValue
	scene.Sheet.Initiative.FictionalTimeTick = MaxSimulationInitiativeValue - 1
	if _, _, err := advanceSimulationInitiative(scene, ""); err == nil {
		t.Fatal("bounded initiative clock unexpectedly overflowed")
	}
}

func TestSimulationInitiativeAdvancesPastActiveActingCharacterExactlyOnce(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{
			ActiveCharacterID: initiativeA,
			Initiative:        SimulationInitiativeClock{Round: 3, Turn: 7, FictionalTimeTick: 6},
		},
		Participants: []SceneParticipantProjection{
			{CharacterID: initiativeA, TurnPosition: 0},
			{CharacterID: initiativeB, TurnPosition: 1},
			{CharacterID: initiativeC, TurnPosition: 2},
		},
	}
	next, clock, err := advanceSimulationInitiative(scene, initiativeA)
	if err != nil {
		t.Fatal(err)
	}
	if next != initiativeB || clock != (SimulationInitiativeClock{
		Round: 3, Turn: 8, FictionalTimeTick: 7,
	}) {
		t.Fatalf("active-actor advance=%q/%+v", next, clock)
	}
}

func TestNextSceneCharacterNeverTransfersResponderOwnershipToUserPersona(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{ActiveCharacterID: initiativeA},
		Participants: []SceneParticipantProjection{
			{CharacterID: initiativeA, TurnPosition: 0},
			{CharacterID: initiativeB, TurnPosition: 1},
		},
	}

	got, err := nextSceneCharacterExcept(scene, initiativeB)
	if err != nil {
		t.Fatal(err)
	}
	if got != scene.Sheet.ActiveCharacterID {
		t.Fatalf("next responder=%q want current responder %q", got, scene.Sheet.ActiveCharacterID)
	}
}

func TestNextSceneCharacterRotatesAcrossNonUserParticipants(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{ActiveCharacterID: initiativeA},
		Participants: []SceneParticipantProjection{
			{CharacterID: initiativeA, TurnPosition: 0},
			{CharacterID: initiativeB, TurnPosition: 1},
			{CharacterID: initiativeC, TurnPosition: 2},
		},
	}

	got, err := nextSceneCharacterExcept(scene, initiativeB)
	if err != nil {
		t.Fatal(err)
	}
	if got != initiativeC {
		t.Fatalf("next responder=%q", got)
	}
}

func narratorDirectionTurnForInitiative() UserTurnAuthority {
	return UserTurnAuthority{
		PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
		ContributionKind: UserContributionDirection, ExactText: "Continue.",
	}
}

func characterTurnForInitiative(characterID string) UserTurnAuthority {
	return UserTurnAuthority{
		PersonaKind: UserPersonaCharacter, CharacterID: characterID,
		PersonaName: "Acting character", PersonaSummary: "A current scene participant.",
		ContributionKind: UserContributionDialogue, ExactText: "Continue.",
	}
}
