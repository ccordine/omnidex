package cognition

import "fmt"

func NewObservation(
	id ObservationID,
	revision WorldRevision,
	kind ObservationKind,
	content string,
) (Observation, error) {
	return newObservation(id, "", revision, kind, content)
}

func NewActionObservation(
	id ObservationID,
	actionID ActionID,
	revision WorldRevision,
	kind ObservationKind,
	content string,
) (Observation, error) {
	return newObservation(id, actionID, revision, kind, content)
}

func newObservation(
	id ObservationID,
	actionID ActionID,
	revision WorldRevision,
	kind ObservationKind,
	content string,
) (Observation, error) {
	observation := Observation{
		ID: id, ActionID: actionID, Revision: revision, Kind: kind,
		Content: content, ContentSHA256: contentSHA256(content),
	}
	if err := observation.Validate(); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func (observation Observation) Validate() error {
	if err := validateIdentity(string(observation.ID), "observation ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservation, err)
	}
	if observation.ActionID != "" {
		if err := validateIdentity(string(observation.ActionID), "producing action ID"); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidObservation, err)
		}
	}
	if err := observation.Revision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObservation, err)
	}
	if err := validateIdentity(string(observation.Kind), "observation kind"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservation, err)
	}
	if err := validateContent(observation.Content, "observation content", MaxObservationBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObservation, err)
	}
	if !validSHA256(observation.ContentSHA256) || contentSHA256(observation.Content) != observation.ContentSHA256 {
		return fmt.Errorf("%w: content hash does not bind the exact observation", ErrInvalidObservation)
	}
	return nil
}

func (observation Observation) EvidenceRef() EvidenceRef {
	return EvidenceRef{
		ObservationID: observation.ID,
		Revision:      observation.Revision,
		SHA256:        observation.ContentSHA256,
	}
}
