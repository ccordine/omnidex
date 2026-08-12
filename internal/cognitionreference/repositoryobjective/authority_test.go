package repositoryobjective

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsesProjectedBytesAndRejectsLiveRepositoryDrift(t *testing.T) {
	root := storageFixture(t)
	selector := selectorFunc(func(_ context.Context, gap SemanticGap) (CandidateID, error) {
		path := filepath.Join(root, "database", "database.go")
		if err := os.WriteFile(path, []byte("package database\n\nfunc Resolve(string) string { return \"changed\" }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range gap.Candidates {
			for _, evidenceID := range candidate.EvidenceIDs {
				for _, evidence := range gap.Evidence {
					if evidence.ID == evidenceID && strings.Contains(evidence.Content, "durableRecord") {
						return candidate.ID, nil
					}
				}
			}
		}
		t.Fatal("projected candidate evidence changed with live source")
		return "", nil
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.drift", Root: root, Question: "Which declaration owns durable storage?",
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: fullAcceptance(),
	}, selector)
	if !errors.Is(err, ErrRepositoryAuthority) || result.Complete ||
		result.Subject.Symbol.QualifiedName != "example.test/storage/database.Resolve" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunIgnoresHostileAmbientGoWorkspaceControls(t *testing.T) {
	t.Setenv("GOENV", "/attacker/goenv")
	t.Setenv("GOFLAGS", "-tags=attacker")
	t.Setenv("GOWORK", "/attacker/go.work")
	t.Setenv("GOPROXY", "https://attacker.invalid")
	t.Setenv("GOSUMDB", "attacker.invalid")
	t.Setenv("GOTOOLCHAIN", "attacker")
	t.Setenv("PWD", "/attacker/root")
	result, err := Run(t.Context(), Objective{
		ID: "objective.ambient", Root: deliveryFixture(t),
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/delivery.Dispatch"},
		Acceptance: fullAcceptance(),
	}, nil)
	if err != nil || !result.Complete || result.SelectorCalls != 0 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
