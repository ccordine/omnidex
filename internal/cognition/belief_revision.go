package cognition

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// EpistemicRef is a code-issued opaque reference to model-visible belief
// state. A model may echo it but cannot assign or mutate its authority.
type EpistemicRef struct {
	URI     string `json:"uri"`
	Version string `json:"version"`
	SHA256  string `json:"content_sha256"`
}

func (ref EpistemicRef) Validate() error {
	if ref.URI == "" || len(ref.URI) > MaxEpistemicRefURIBytes || !utf8.ValidString(ref.URI) ||
		strings.ContainsRune(ref.URI, 0) {
		return fmt.Errorf("%w: target URI must be nonempty bounded UTF-8", ErrInvalidBeliefRevision)
	}
	for index, character := range ref.URI {
		if unicode.IsSpace(character) || !validIdentityRune(character, index == 0) {
			return fmt.Errorf("%w: target URI contains an unregistered character", ErrInvalidBeliefRevision)
		}
	}
	if err := validateVersion(ref.Version, "epistemic reference version"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBeliefRevision, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: target hash must be 64 lowercase hex characters", ErrInvalidBeliefRevision)
	}
	return nil
}

// BeliefRevisionProposal asks code to reject one exact active hypothesis using
// current evidence. It carries no disposition text or ledger mutation fields.
type BeliefRevisionProposal struct {
	TargetRef    EpistemicRef  `json:"target_ref"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`
}

func (proposal BeliefRevisionProposal) Validate() error {
	if err := proposal.TargetRef.Validate(); err != nil {
		return err
	}
	if len(proposal.EvidenceRefs) == 0 {
		return fmt.Errorf("%w: contradiction evidence is required", ErrInvalidBeliefRevision)
	}
	if err := validateEvidenceRefs(proposal.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: contradiction evidence: %v", ErrInvalidBeliefRevision, err)
	}
	return nil
}

func (proposal BeliefRevisionProposal) Clone() BeliefRevisionProposal {
	proposal.EvidenceRefs = cloneSlice(proposal.EvidenceRefs)
	return proposal
}
