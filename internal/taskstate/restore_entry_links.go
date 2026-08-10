package taskstate

import "fmt"

func (ledger *Ledger) validateRestoredEntryLinks() error {
	derived := make(map[EntryID]EntryID)
	for _, replacement := range ledger.entries {
		if replacement.SupersedesID == "" {
			continue
		}
		old, exists := ledger.entries[replacement.SupersedesID]
		if !exists {
			return fmt.Errorf("%w: entry %q supersedes missing entry %q", ErrInvalidState, replacement.ID, replacement.SupersedesID)
		}
		if err := validateGenericReplacement(old, replacement); err != nil {
			return err
		}
		if err := ledger.deriveReplacement(derived, old, replacement); err != nil {
			return err
		}
	}
	for _, accepted := range ledger.entries {
		if accepted.Kind != EntryAcceptedDecision {
			continue
		}
		candidate, err := ledger.validateAcceptedCandidate(accepted)
		if err != nil {
			return err
		}
		if err := ledger.deriveReplacement(derived, candidate, accepted); err != nil {
			return err
		}
	}
	for _, entry := range ledger.entries {
		replacementID, hasReplacement := derived[entry.ID]
		if entry.Status == EntrySuperseded && !hasReplacement {
			return fmt.Errorf("%w: superseded entry %q lacks its canonical replacement", ErrInvalidState, entry.ID)
		}
		if entry.Status != EntrySuperseded && hasReplacement {
			return fmt.Errorf("%w: replacement link targets non-superseded entry %q", ErrInvalidState, entry.ID)
		}
		if hasReplacement && entry.SupersededBy != replacementID {
			return fmt.Errorf("%w: entry %q has conflicting replacement identity", ErrInvalidState, entry.ID)
		}
	}
	return nil
}

func validateGenericReplacement(old, replacement Entry) error {
	if old.ID == replacement.ID || old.Status != EntrySuperseded {
		return fmt.Errorf("%w: entry %q does not supersede a distinct superseded entry", ErrInvalidState, replacement.ID)
	}
	if old.Kind != replacement.Kind {
		return fmt.Errorf("%w: replacement %q changes entry kind from %q to %q", ErrInvalidState, replacement.ID, old.Kind, replacement.Kind)
	}
	if authorityRank(replacement.Authority) < authorityRank(old.Authority) {
		return fmt.Errorf("%w: %w: replacement %q downgrades authority", ErrInvalidState, ErrAuthorityDenied, replacement.ID)
	}
	if old.Authority == AuthorityUser &&
		(replacement.Authority != AuthorityUser || old.DispositionBy != AuthorityUser) {
		return fmt.Errorf("%w: %w: user-authority entry %q requires a user-authority replacement", ErrInvalidState, ErrAuthorityDenied, old.ID)
	}
	if replacement.CreatedVersion >= old.UpdatedVersion || old.UpdatedVersion > replacement.UpdatedVersion {
		return fmt.Errorf("%w: replacement %q has impossible supersession versions", ErrInvalidState, replacement.ID)
	}
	return nil
}

func (ledger *Ledger) validateAcceptedCandidate(accepted Entry) (Entry, error) {
	candidate, exists := ledger.entries[accepted.Provenance.SourceEntryID]
	if !exists || candidate.ID == accepted.ID || candidate.Kind != EntryDecisionCandidate ||
		candidate.Authority != AuthorityModelProposal || candidate.CreatedBy != AuthorityModelProposal ||
		candidate.Status != EntrySuperseded {
		return Entry{}, fmt.Errorf("%w: accepted decision %q lacks its source model candidate", ErrInvalidState, accepted.ID)
	}
	if accepted.SupersedesID == candidate.ID {
		return Entry{}, fmt.Errorf("%w: accepted decision %q conflates candidate provenance with generic supersession", ErrInvalidState, accepted.ID)
	}
	if accepted.ScopeNodeID != candidate.ScopeNodeID || accepted.Content != candidate.Content ||
		accepted.ContentSHA256 != candidate.ContentSHA256 ||
		!equalFloat64Pointers(accepted.Confidence, candidate.Confidence) {
		return Entry{}, fmt.Errorf("%w: accepted decision %q does not exactly preserve its candidate", ErrInvalidState, accepted.ID)
	}
	if candidate.CreatedVersion >= accepted.CreatedVersion ||
		candidate.UpdatedVersion != accepted.CreatedVersion ||
		candidate.DispositionReason != accepted.Provenance.AcceptancePolicy ||
		candidate.DispositionBy != accepted.Provenance.AcceptedBy {
		return Entry{}, fmt.Errorf("%w: accepted decision %q has inconsistent candidate lineage", ErrInvalidState, accepted.ID)
	}
	if !hasEvidenceRef(accepted.Refs) {
		return Entry{}, fmt.Errorf("%w: %w: accepted decision %q lacks acceptance evidence", ErrInvalidState, ErrEvidenceRequired, accepted.ID)
	}
	return candidate, nil
}

func (ledger *Ledger) deriveReplacement(
	derived map[EntryID]EntryID,
	old, replacement Entry,
) error {
	if existing, exists := derived[old.ID]; exists && existing != replacement.ID {
		return fmt.Errorf("%w: entry %q has multiple canonical replacements", ErrInvalidState, old.ID)
	}
	if old.SupersededBy != "" && old.SupersededBy != replacement.ID {
		return fmt.Errorf("%w: entry %q has a conflicting derived replacement", ErrInvalidState, old.ID)
	}
	derived[old.ID] = replacement.ID
	old.SupersededBy = replacement.ID
	ledger.entries[old.ID] = old
	return nil
}
