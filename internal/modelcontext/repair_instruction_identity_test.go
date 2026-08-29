package modelcontext

import "testing"

func TestRepairInstructionPathLexerAcceptsQuotedSourceEscapesAcrossDomains(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		`Replace the displayed status with "Ready\nWaiting" and preserve the control.`,
		"Set the placard to `Gallery [temporary]\nHours:\n  09:00-17:00` and preserve all other output.",
		`Change the notification to fmt.Sprintf("Queued\t%d", count).`,
	} {
		if identities := RepairInstructionPathIdentities(
			value, ArtifactIdentityProvenance{},
		); len(identities) != 0 {
			t.Fatalf("quoted source escapes in %q produced identities %+v", value, identities)
		}
	}
}

func TestRepairInstructionPathLexerRejectsRealPathsInsideSourceQuotes(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		`Replace the destination with "/srv/private/value".`,
		`Replace the destination with "../private/value".`,
		`Replace the destination with "C:\\ProgramData\\private\\value".`,
		`Replace the destination with "\\\\server\\share\\value".`,
		`Replace the destination with "foo\/private".`,
		`Replace the destination with "\x2fprivate\x2fvalue".`,
		"Replace the Go raw string with `private\\new\\value`.",
		"Replace the Go raw string with `C:\\new`.",
		`Set the returned PHP single-quoted string to '\new'.`,
		`Set the returned Rust raw string to r"\new".`,
		`Set the returned Rust hash raw string to r#"private\new"#.`,
	} {
		if identities := RepairInstructionPathIdentities(
			value, ArtifactIdentityProvenance{},
		); len(identities) == 0 {
			t.Fatalf("quoted path %q was accepted", value)
		}
	}
}

func TestRepairInstructionPathLexerKeepsProvenanceAndUnquotedBoundary(t *testing.T) {
	t.Parallel()
	provenance, err := NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		`Move the value into "transport.go".`,
		`Move the value into "transport\u002ego".`,
		`Describe the line break as \n outside a source quote.`,
	} {
		if identities := RepairInstructionPathIdentities(value, provenance); len(identities) == 0 {
			t.Fatalf("forbidden repair-instruction identity in %q was accepted", value)
		}
	}
}
