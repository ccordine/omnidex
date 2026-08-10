package cognition

import (
	"errors"
	"strings"
	"testing"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testRevision(number uint64) WorldRevision {
	return WorldRevision{EpisodeID: "episode-1", Number: number, SHA256: testDigest}
}

func TestPublicIdentityAndRevisionContracts(t *testing.T) {
	t.Parallel()
	scenario := ScenarioRef{ID: "scenario-1", SHA256: testDigest}
	if err := scenario.Validate(); err != nil {
		t.Fatalf("validate scenario: %v", err)
	}
	if err := (EpisodeRef{ID: "episode-1"}).Validate(); err != nil {
		t.Fatalf("validate episode: %v", err)
	}
	if err := testRevision(1).Validate(); err != nil {
		t.Fatalf("validate revision: %v", err)
	}

	invalidUTF8 := string([]byte{'i', 'd', 0xff})
	for name, candidate := range map[string]ScenarioRef{
		"empty ID":       {SHA256: testDigest},
		"whitespace ID":  {ID: "scenario 1", SHA256: testDigest},
		"NUL ID":         {ID: "scenario\x00one", SHA256: testDigest},
		"invalid UTF-8":  {ID: ScenarioID(invalidUTF8), SHA256: testDigest},
		"oversized ID":   {ID: ScenarioID(strings.Repeat("x", MaxIdentityBytes+1)), SHA256: testDigest},
		"uppercase hash": {ID: "scenario-1", SHA256: strings.ToUpper(testDigest)},
		"short hash":     {ID: "scenario-1", SHA256: "abc"},
	} {
		candidate := candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidScenario) {
				t.Fatalf("error = %v, want ErrInvalidScenario", err)
			}
		})
	}

	for name, revision := range map[string]WorldRevision{
		"empty episode": {Number: 1, SHA256: testDigest},
		"zero number":   {EpisodeID: "episode-1", SHA256: testDigest},
		"invalid hash":  {EpisodeID: "episode-1", Number: 1, SHA256: "bad"},
	} {
		revision := revision
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := revision.Validate(); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("error = %v, want ErrInvalidRevision", err)
			}
		})
	}
}

func TestReferenceConstructorsRejectInvalidValues(t *testing.T) {
	t.Parallel()
	if _, err := NewScenarioRef("scenario-1", "bad"); !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("scenario constructor error = %v, want ErrInvalidScenario", err)
	}
	if _, err := NewEpisodeRef(""); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("episode constructor error = %v, want ErrInvalidIdentity", err)
	}
	if _, err := NewWorldRevision("episode-1", 0, testDigest); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("revision constructor error = %v, want ErrInvalidRevision", err)
	}
}

func TestPredicateAndGoalAreBoundedAndDefensivelyCopied(t *testing.T) {
	t.Parallel()
	args := []string{"subject", "value"}
	predicate, err := NewPredicate("state.matches", args)
	if err != nil {
		t.Fatalf("new predicate: %v", err)
	}
	args[0] = "mutated"
	if predicate.Args[0] != "subject" {
		t.Fatal("predicate retained caller-owned argument storage")
	}
	goal, err := NewGoalExpression([]Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	if err := goal.Validate(); err != nil {
		t.Fatalf("validate goal: %v", err)
	}

	invalidUTF8 := string([]byte{'x', 0xff})
	for name, candidate := range map[string]Predicate{
		"empty name":       {Args: []string{"x"}},
		"NUL name":         {Name: "state\x00matches", Args: []string{"x"}},
		"invalid argument": {Name: "state.matches", Args: []string{invalidUTF8}},
		"NUL argument":     {Name: "state.matches", Args: []string{"x\x00y"}},
		"too many args":    {Name: "state.matches", Args: make([]string, MaxPredicateArgs+1)},
	} {
		candidate := candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidPredicate) {
				t.Fatalf("error = %v, want ErrInvalidPredicate", err)
			}
		})
	}

	duplicate := GoalExpression{All: []Predicate{predicate, predicate}}
	if err := duplicate.Validate(); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("duplicate goal error = %v, want ErrInvalidGoal", err)
	}
	contradiction := GoalExpression{All: []Predicate{predicate}, Not: []Predicate{predicate}}
	if err := contradiction.Validate(); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("contradictory goal error = %v, want ErrInvalidGoal", err)
	}
	if err := (GoalExpression{}).Validate(); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("empty goal error = %v, want ErrInvalidGoal", err)
	}
}

func TestObservationBindsContentToRevisionAndEvidence(t *testing.T) {
	t.Parallel()
	revision := testRevision(2)
	observation, err := NewObservation("observation-1", revision, "document", "bounded public content")
	if err != nil {
		t.Fatalf("new observation: %v", err)
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("validate observation: %v", err)
	}
	ref := observation.EvidenceRef()
	if err := ref.Validate(); err != nil {
		t.Fatalf("validate evidence ref: %v", err)
	}
	if ref.ObservationID != observation.ID || ref.Revision != revision || ref.SHA256 != observation.ContentSHA256 {
		t.Fatalf("evidence ref does not bind observation: %#v", ref)
	}

	badHash := observation
	badHash.ContentSHA256 = strings.Repeat("b", 64)
	if err := badHash.Validate(); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("content hash error = %v, want ErrInvalidObservation", err)
	}
	badText := observation
	badText.Content = "public\x00content"
	if err := badText.Validate(); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("NUL content error = %v, want ErrInvalidObservation", err)
	}
	tooLarge := observation
	tooLarge.Content = strings.Repeat("x", MaxObservationBytes+1)
	if err := tooLarge.Validate(); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("oversized content error = %v, want ErrInvalidObservation", err)
	}
	badRef := ref
	badRef.SHA256 = "bad"
	if err := badRef.Validate(); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("evidence hash error = %v, want ErrInvalidEvidence", err)
	}
}
