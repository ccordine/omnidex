package cognition

import "fmt"

func NewScenarioRef(id ScenarioID, sha256 string) (ScenarioRef, error) {
	ref := ScenarioRef{ID: id, SHA256: sha256}
	if err := ref.Validate(); err != nil {
		return ScenarioRef{}, err
	}
	return ref, nil
}

func (ref ScenarioRef) Validate() error {
	if err := validateIdentity(string(ref.ID), "scenario ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidScenario, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: public scenario hash must be 64 lowercase hex characters", ErrInvalidScenario)
	}
	return nil
}

func NewEpisodeRef(id EpisodeID) (EpisodeRef, error) {
	ref := EpisodeRef{ID: id}
	if err := ref.Validate(); err != nil {
		return EpisodeRef{}, err
	}
	return ref, nil
}

func (ref EpisodeRef) Validate() error {
	if err := validateIdentity(string(ref.ID), "episode ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidIdentity, err)
	}
	return nil
}

func NewWorldRevision(episodeID EpisodeID, number uint64, sha256 string) (WorldRevision, error) {
	revision := WorldRevision{EpisodeID: episodeID, Number: number, SHA256: sha256}
	if err := revision.Validate(); err != nil {
		return WorldRevision{}, err
	}
	return revision, nil
}

func (revision WorldRevision) Validate() error {
	if err := validateIdentity(string(revision.EpisodeID), "revision episode ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRevision, err)
	}
	if revision.Number == 0 {
		return fmt.Errorf("%w: revision number must be positive", ErrInvalidRevision)
	}
	if !validSHA256(revision.SHA256) {
		return fmt.Errorf("%w: revision hash must be 64 lowercase hex characters", ErrInvalidRevision)
	}
	return nil
}

func (ref EvidenceRef) Validate() error {
	if err := validateIdentity(string(ref.ObservationID), "evidence observation ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvidence, err)
	}
	if err := ref.Revision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: evidence hash must be 64 lowercase hex characters", ErrInvalidEvidence)
	}
	return nil
}

func evidenceIdentity(ref EvidenceRef) string {
	return string(ref.ObservationID) + "\x00" + string(ref.Revision.EpisodeID) + "\x00" +
		fmt.Sprint(ref.Revision.Number) + "\x00" + ref.SHA256
}

func validateEvidenceRefs(refs []EvidenceRef) error {
	if len(refs) > MaxEvidenceRefs {
		return fmt.Errorf("%w: evidence reference count exceeds %d", ErrInvalidEvidence, MaxEvidenceRefs)
	}
	seen := make(map[string]struct{}, len(refs))
	var episode EpisodeID
	for index, ref := range refs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("%w: reference %d: %v", ErrInvalidEvidence, index, err)
		}
		if index == 0 {
			episode = ref.Revision.EpisodeID
		} else if ref.Revision.EpisodeID != episode {
			return fmt.Errorf("%w: references span multiple episodes", ErrInvalidEvidence)
		}
		key := evidenceIdentity(ref)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate reference at index %d", ErrInvalidEvidence, index)
		}
		seen[key] = struct{}{}
	}
	return nil
}
