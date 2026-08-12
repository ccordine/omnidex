package repositoryobjective

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRunRejectsMoreThanEightSemanticCandidates(t *testing.T) {
	files := map[string]string{"go.mod": "module example.test/many\n\ngo 1.24\n"}
	for index := 0; index < maxSubjectCandidates+1; index++ {
		path := fmt.Sprintf("p%d/value.go", index)
		files[path] = fmt.Sprintf("package p%d\n\nfunc Resolve() int { return %d }\n", index, index)
	}
	result, err := Run(t.Context(), Objective{
		ID: "objective.too-many", Root: newCommittedGoFixture(t, files), Question: "Which resolution applies?",
		Subject:    SubjectLookup{Kind: LookupName, Value: "Resolve"},
		Acceptance: fullAcceptance(),
	}, nil)
	if !errors.Is(err, ErrSubjectAmbiguous) || result.SelectorCalls != 0 || result.Complete {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunRejectsOversizedExactDeclaration(t *testing.T) {
	root := newCommittedGoFixture(t, map[string]string{
		"go.mod": "module example.test/large\n\ngo 1.24\n",
		"large.go": "package large\n\nfunc Resolve() string { return \"" +
			strings.Repeat("x", maxDeclarationBytes+1) + "\" }\n",
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.large", Root: root,
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/large.Resolve"},
		Acceptance: fullAcceptance(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") || result.Complete {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunRejectsUnboundedDirectRelations(t *testing.T) {
	var source strings.Builder
	source.WriteString("package relations\n\n")
	for index := 0; index < maxDirectRelations+1; index++ {
		fmt.Fprintf(&source, "func helper%d() int { return %d }\n", index, index)
	}
	source.WriteString("\nfunc Subject() int { return ")
	for index := 0; index < maxDirectRelations+1; index++ {
		if index > 0 {
			source.WriteString(" + ")
		}
		fmt.Fprintf(&source, "helper%d()", index)
	}
	source.WriteString(" }\n")
	root := newCommittedGoFixture(t, map[string]string{
		"go.mod": "module example.test/relations\n\ngo 1.24\n", "relations.go": source.String(),
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.relations", Root: root,
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/relations.Subject"},
		Acceptance: fullAcceptance(),
	}, nil)
	if !errors.Is(err, ErrRelationBound) || result.Complete || result.Subject.Symbol.SymbolID == "" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunRejectsIncompleteCompilerAnalysis(t *testing.T) {
	root := newCommittedGoFixture(t, map[string]string{
		"go.mod":    "module example.test/broken\n\ngo 1.24\n",
		"broken.go": "package broken\n\nfunc Resolve() Missing { return Missing{} }\n",
	})
	result, err := Run(t.Context(), Objective{
		ID: "objective.broken", Root: root,
		Subject:    SubjectLookup{Kind: LookupQualifiedName, Value: "example.test/broken.Resolve"},
		Acceptance: fullAcceptance(),
	}, nil)
	if !errors.Is(err, ErrRepositoryAuthority) || result.Complete {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
