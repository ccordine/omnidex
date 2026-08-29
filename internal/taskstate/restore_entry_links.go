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
