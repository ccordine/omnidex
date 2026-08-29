package changeapply_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestValidateDeletionCandidateAcceptsExactUnreferencedSource(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/eligible\n\ngo 1.24\n", mode: 0o644},
		"retained.go": {content: "package eligible\n\nfunc Retained() {}\n", mode: 0o644},
		"obsolete.go": {content: "package eligible\n\nfunc Obsolete() {}\n", mode: 0o644},
	})
	if err := changeapply.ValidateDeletionCandidate(
		t.Context(), fixture.snapshot, fixture.analysis, fixture.file(t, "obsolete.go").ID,
	); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDeletionCandidateRejectsMissingStaleUntrackedAndReferencedAuthority(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, fixture *fixture)
		fileID     func(fixture *fixture) string
		want       string
		ineligible bool
	}{
		{
			name:       "unknown file ID",
			fileID:     func(*fixture) string { return "file_" + strings.Repeat("0", 64) },
			want:       "not an exact indexed source member",
			ineligible: true,
		},
		{
			name: "stale snapshot",
			setup: func(t *testing.T, fixture *fixture) {
				if err := os.WriteFile(filepath.Join(fixture.root, "retained.go"), []byte("package eligibility\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			fileID: func(fixture *fixture) string { return fixture.file(t, "obsolete.go").ID },
			want:   "stale",
		},
		{
			name: "untracked source",
			setup: func(t *testing.T, fixture *fixture) {
				path := filepath.Join(fixture.root, "untracked.go")
				if err := os.WriteFile(path, []byte("package eligibility\n\nfunc Untracked() {}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				fixture.refreshUntracked(t)
			},
			fileID:     func(fixture *fixture) string { return fixture.file(t, "untracked.go").ID },
			want:       "untracked",
			ineligible: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixture(t, map[string]fixtureEntry{
				"go.mod":      {content: "module example.com/eligibility\n\ngo 1.24\n", mode: 0o644},
				"retained.go": {content: "package eligibility\n\nfunc Retained() {}\n", mode: 0o644},
				"obsolete.go": {content: "package eligibility\n\nfunc Obsolete() {}\n", mode: 0o644},
			})
			if test.setup != nil {
				test.setup(t, fixture)
			}
			err := changeapply.ValidateDeletionCandidate(t.Context(), fixture.snapshot, fixture.analysis, test.fileID(fixture))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("eligibility error=%v want %q", err, test.want)
			}
			_, typed := changeapply.DeletionCandidateIneligibilityOf(err)
			if typed != test.ineligible {
				t.Fatalf("typed ineligibility=%t want %t: %v", typed, test.ineligible, err)
			}
		})
	}
}

func (fixture *fixture) refreshUntracked(t *testing.T) {
	t.Helper()
	// Build exact facts without adding the candidate to Git. The authoritative
	// eligibility boundary must distinguish indexed presence from durable tracking.
	snapshot, analysis := exactFixtureFacts(t, fixture.root)
	fixture.snapshot = snapshot
	fixture.analysis = analysis
}
