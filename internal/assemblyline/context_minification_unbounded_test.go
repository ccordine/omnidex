package assemblyline

import (
	"strings"
	"testing"
)

func TestContextMinificationKeepsNecessaryTextBeyondFormerCeilings(t *testing.T) {
	input := ContextMinificationInput{ExactInstruction: "Retain every necessary fact.", KnownArtifactPaths: []string{},
		SelectedAuthorities: []ContextCandidateAuthority{{Namespace: "session_coding", CandidateID: "CTX_1", Content: "Required task facts."}}}
	job, err := NewContextMinificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil || maximum != 0 {
		t.Fatalf("minifier still has a response ceiling: %d %v", maximum, err)
	}
	for _, content := range []string{"Keep the failure/result and read/write details.", strings.Repeat("Required fact. ", 200), strings.Repeat("Necessary fact. ", 10000)} {
		content = strings.TrimSpace(content)
		if err := (PortableResult{Candidate: content}).ValidateFor(job); err != nil {
			t.Fatal(err)
		}
		decision, err := DecodeContextMinificationDecision(input, content)
		if err != nil || decision.MinimalContext != content {
			t.Fatalf("response rejected or altered: %v", err)
		}
		compiled := ObjectiveContext{Capsules: []ObjectiveContextCapsule{{Sources: []ObjectiveContextSource{{Namespace: "session_coding", CandidateID: "CTX_1"}}, Content: content}}}
		if err := compiled.Validate(); err != nil {
			t.Fatalf("compiled context retained a ceiling: %v", err)
		}
	}
	for _, invalid := range []string{"", "invalid\x00fact", string([]byte{0xff})} {
		if _, err := DecodeContextMinificationDecision(input, invalid); err == nil {
			t.Fatalf("invalid text accepted: %q", invalid)
		}
	}
	input.KnownArtifactPaths = []string{"/opt/private/config.toml"}
	if _, err := DecodeContextMinificationDecision(input, "Use /opt/private/config.toml directly."); err == nil {
		t.Fatal("proven artifact identity escaped its code-owned binding")
	}
}
