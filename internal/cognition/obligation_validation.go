package cognition

import (
	"encoding/json"
	"fmt"
)

func (obligation Obligation) Validate() error {
	if err := validateIdentity(string(obligation.ID), "obligation ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObligation, err)
	}
	if obligation.ParentID != "" {
		if err := validateIdentity(string(obligation.ParentID), "obligation parent ID"); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidObligation, err)
		}
		if obligation.ParentID == obligation.ID {
			return fmt.Errorf("%w: obligation cannot parent itself", ErrInvalidObligation)
		}
	}
	if err := obligation.Desired.Validate(); err != nil {
		return fmt.Errorf("%w: desired predicate: %v", ErrInvalidObligation, err)
	}
	if len(obligation.DependsOn) > MaxObligationDependencies {
		return fmt.Errorf("%w: dependency count exceeds %d", ErrInvalidObligation, MaxObligationDependencies)
	}
	seen := make(map[ObligationID]struct{}, len(obligation.DependsOn))
	for index, dependency := range obligation.DependsOn {
		if err := validateIdentity(string(dependency), "dependency obligation ID"); err != nil {
			return fmt.Errorf("%w: dependency %d: %v", ErrInvalidObligation, index, err)
		}
		if dependency == obligation.ID {
			return fmt.Errorf("%w: obligation cannot depend on itself", ErrInvalidObligation)
		}
		if _, duplicate := seen[dependency]; duplicate {
			return fmt.Errorf("%w: dependency %q is duplicated", ErrInvalidObligation, dependency)
		}
		seen[dependency] = struct{}{}
	}
	if err := validateEvidenceRefs(obligation.SupportingRefs); err != nil {
		return fmt.Errorf("%w: supporting evidence: %v", ErrInvalidObligation, err)
	}
	if err := obligation.CompletionCheck.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligation, err)
	}
	if obligation.CreatedGeneration == 0 {
		return fmt.Errorf("%w: created generation must be positive", ErrInvalidObligation)
	}
	if !registeredObligationStatus(obligation.Status) {
		return fmt.Errorf("%w: status %q is not registered", ErrInvalidObligation, obligation.Status)
	}
	if obligation.Status == ObligationSuperseded {
		if obligation.SupersededGeneration <= obligation.CreatedGeneration {
			return fmt.Errorf("%w: superseded generation must follow creation", ErrInvalidObligation)
		}
	} else if obligation.SupersededGeneration != 0 {
		return fmt.Errorf("%w: only superseded obligations may record a superseded generation", ErrInvalidObligation)
	}
	if obligation.Status == ObligationSatisfied {
		if obligation.Completion == nil || obligation.Completion.Outcome != CompletionSatisfied {
			return fmt.Errorf("%w: satisfied obligation requires a satisfied completion result", ErrInvalidObligation)
		}
		if err := obligation.Completion.ValidateFor(
			obligation, obligation.Completion.Revision, obligation.SupportingRefs,
		); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidObligation, err)
		}
	} else if obligation.Completion != nil {
		return fmt.Errorf("%w: only satisfied obligations may retain a completion result", ErrInvalidObligation)
	}
	return nil
}

func registeredObligationStatus(status ObligationStatus) bool {
	switch status {
	case ObligationProposed, ObligationReady, ObligationBlocked, ObligationActive,
		ObligationSatisfied, ObligationFailed, ObligationSuperseded:
		return true
	default:
		return false
	}
}

func (snapshot ObligationGraphSnapshot) Validate() error {
	if snapshot.Schema != ObligationGraphSchemaV1 {
		return fmt.Errorf("%w: schema %q is not registered", ErrInvalidObligationGraph, snapshot.Schema)
	}
	if snapshot.Generation == 0 {
		return fmt.Errorf("%w: generation must be positive", ErrInvalidObligationGraph)
	}
	if err := validateIdentity(string(snapshot.RootID), "obligation graph root ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObligationGraph, err)
	}
	if len(snapshot.Obligations) == 0 || len(snapshot.Obligations) > MaxObligations {
		return fmt.Errorf("%w: obligation count must be between 1 and %d", ErrInvalidObligationGraph, MaxObligations)
	}
	items := make(map[ObligationID]Obligation, len(snapshot.Obligations))
	previous := ObligationID("")
	active := 0
	for index, obligation := range snapshot.Obligations {
		if err := obligation.Validate(); err != nil {
			return fmt.Errorf("%w: obligation %d: %v", ErrInvalidObligationGraph, index, err)
		}
		if index > 0 && obligation.ID <= previous {
			return fmt.Errorf("%w: obligations must be uniquely sorted by ID", ErrInvalidObligationGraph)
		}
		if obligation.CreatedGeneration > snapshot.Generation ||
			obligation.SupersededGeneration > snapshot.Generation {
			return fmt.Errorf("%w: obligation %q refers to a future generation", ErrInvalidObligationGraph, obligation.ID)
		}
		if obligation.CreatedGeneration < snapshot.Generation &&
			!terminalOrSuperseded(obligation.Status) {
			return fmt.Errorf("%w: old obligation %q remains open", ErrInvalidObligationGraph, obligation.ID)
		}
		if obligation.Status == ObligationActive {
			active++
		}
		items[obligation.ID], previous = obligation, obligation.ID
	}
	if active > 1 {
		return fmt.Errorf("%w: more than one obligation is active", ErrInvalidObligationGraph)
	}
	if err := validateObligationLinks(snapshot, items); err != nil {
		return err
	}
	if !validSHA256(snapshot.SHA256) || obligationSnapshotSHA256(snapshot) != snapshot.SHA256 {
		return fmt.Errorf("%w: snapshot hash does not bind the exact graph", ErrInvalidObligationGraph)
	}
	return nil
}

func terminalOrSuperseded(status ObligationStatus) bool {
	return status == ObligationSatisfied || status == ObligationFailed || status == ObligationSuperseded
}

func obligationSnapshotSHA256(snapshot ObligationGraphSnapshot) string {
	payload := struct {
		Schema      string       `json:"schema"`
		Generation  uint64       `json:"generation"`
		RootID      ObligationID `json:"root_id"`
		Obligations []Obligation `json:"obligations"`
	}{snapshot.Schema, snapshot.Generation, snapshot.RootID, snapshot.Obligations}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal obligation graph identity: %v", err))
	}
	return contentSHA256(string(raw))
}
