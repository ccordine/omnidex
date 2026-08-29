package taskstate

import (
	"fmt"
	"math"
)

func (ledger *Ledger) restoreEdge(edge Edge, ledgerVersion uint64) error {
	if err := requireExactText(string(edge.ID), "edge ID"); err != nil {
		return err
	}
	if _, exists := ledger.edges[edge.ID]; exists {
		return fmt.Errorf("%w: duplicate edge %q", ErrInvalidState, edge.ID)
	}
	if err := validateEdgeKind(edge.Kind); err != nil {
		return err
	}
	if edge.CreatedVersion == 0 || edge.CreatedVersion > ledgerVersion || edge.From == edge.To {
		return fmt.Errorf("%w: edge %q has invalid version or endpoints", ErrInvalidState, edge.ID)
	}
	from, fromOK := ledger.nodes[edge.From]
	to, toOK := ledger.nodes[edge.To]
	if !fromOK || !toOK {
		return fmt.Errorf("%w: edge %q endpoints must exist", ErrInvalidState, edge.ID)
	}
	if ledger.semanticEdgeExists(edge.Kind, edge.From, edge.To) {
		return fmt.Errorf("%w: duplicate semantic edge", ErrInvalidState)
	}
	if err := validateEdgeEndpoints(edge.Kind, from, to); err != nil {
		return err
	}
	if edge.Kind == EdgeDependsOn || edge.Kind == EdgeBlocks {
		dependent, prerequisite := executionOrder(edge.Kind, edge.From, edge.To)
		if ledger.executionPathExists(prerequisite, dependent) {
			return fmt.Errorf("%w: restored execution-order edge creates a cycle", ErrInvalidState)
		}
	}
	ledger.edges[edge.ID] = edge
	return nil
}

func (ledger *Ledger) validateRestoredEntry(entry Entry, ledgerVersion uint64) error {
	if err := requireExactText(string(entry.ID), "entry ID"); err != nil {
		return err
	}
	if err := validateEntryKind(entry.Kind); err != nil {
		return err
	}
	if err := validateAuthority(entry.Authority); err != nil {
		return err
	}
	if err := validateAuthority(entry.CreatedBy); err != nil {
		return err
	}
	if entry.ScopeNodeID != "" {
		if _, exists := ledger.nodes[entry.ScopeNodeID]; !exists {
			return fmt.Errorf("%w: entry %q scope node does not exist", ErrInvalidState, entry.ID)
		}
	}
	if err := validateEntryStatus(entry.Status); err != nil {
		return err
	}
	if entry.CreatedVersion == 0 || entry.CreatedVersion > entry.UpdatedVersion || entry.UpdatedVersion > ledgerVersion {
		return fmt.Errorf("%w: entry %q has invalid materialization versions", ErrInvalidState, entry.ID)
	}
	if err := requireEntryContent(entry.Content, entry.Kind); err != nil {
		return err
	}
	if entry.ContentSHA256 != contentDigest(entry.Content) {
		return fmt.Errorf("%w: entry %q content hash mismatch", ErrInvalidState, entry.ID)
	}
	if entry.Confidence != nil && (math.IsNaN(*entry.Confidence) || math.IsInf(*entry.Confidence, 0) ||
		*entry.Confidence < 0 || *entry.Confidence > 1) {
		return fmt.Errorf("%w: entry %q confidence is outside zero to one", ErrInvalidState, entry.ID)
	}
	if err := validateOptionalStep(entry.CreatedStepID, "created step ID"); err != nil {
		return err
	}
	if err := entry.Metadata.Validate(); err != nil {
		return err
	}
	if entry.Refs == nil {
		return fmt.Errorf("%w: entry %q references must be an explicit array", ErrInvalidState, entry.ID)
	}
	if err := validateRefs(entry.Refs); err != nil {
		return err
	}
	if err := validateFeedback(entry.Kind, entry.FeedbackPurpose, entry.CreatedBy); err != nil {
		return err
	}
	if entry.Kind == EntryFact && !hasEvidenceRef(entry.Refs) {
		return fmt.Errorf("%w: %w: restored fact lacks evidence", ErrInvalidState, ErrEvidenceRequired)
	}
	if err := validateNewEntryAuthority(entry.Authority, entry.Kind); err != nil {
		return fmt.Errorf("%w: restored entry authority: %v", ErrInvalidState, err)
	}
	if entry.CreatedBy != entry.Authority {
		return fmt.Errorf("%w: entry %q creator and authority differ", ErrInvalidState, entry.ID)
	}
	if entry.SupersedesID == entry.ID || entry.SupersededBy == entry.ID {
		return fmt.Errorf("%w: entry %q cannot supersede itself", ErrInvalidState, entry.ID)
	}
	if err := validateRestoredDisposition(entry); err != nil {
		return err
	}
	if entry.Status != EntrySuperseded && entry.SupersededBy != "" {
		return fmt.Errorf("%w: non-superseded entry %q carries a superseded-by link", ErrInvalidState, entry.ID)
	}
	if entry.Status == EntryResolved {
		switch entry.Kind {
		case EntryQuestion, EntryFailure, EntryObservation, EntryNote, EntryFeedback:
		default:
			return fmt.Errorf("%w: entry kind %q cannot be resolved", ErrInvalidState, entry.Kind)
		}
		if !hasEvidenceRef(entry.Refs) {
			return fmt.Errorf("%w: %w: resolved entry %q lacks evidence", ErrInvalidState, ErrEvidenceRequired, entry.ID)
		}
	}
	if entry.Status == EntryRejected && entry.Kind == EntryHypothesis &&
		entry.DispositionBy == AuthorityCode && !hasContradictionRef(entry.Refs) {
		return fmt.Errorf("%w: %w: rejected hypothesis %q lacks contradiction evidence", ErrInvalidState, ErrEvidenceRequired, entry.ID)
	}
	return nil
}

func validateRestoredDisposition(entry Entry) error {
	if entry.Status == EntryActive {
		if entry.DispositionReason != "" || entry.DispositionBy != "" || entry.SupersededBy != "" {
			return fmt.Errorf("%w: active entry %q has terminal disposition", ErrInvalidState, entry.ID)
		}
		return nil
	}
	if entry.DispositionReason == "" || entry.DispositionBy == "" {
		return fmt.Errorf("%w: inactive entry %q requires a disposition reason and actor", ErrInvalidState, entry.ID)
	}
	if err := requireExactText(entry.DispositionReason, "entry disposition reason"); err != nil {
		return fmt.Errorf("%w: entry %q disposition reason is invalid: %v", ErrInvalidState, entry.ID, err)
	}
	if entry.DispositionBy != AuthorityCode && entry.DispositionBy != AuthorityUser {
		return fmt.Errorf("%w: %w: entry %q disposition actor %q is not authorized", ErrInvalidState, ErrAuthorityDenied, entry.ID, entry.DispositionBy)
	}
	switch entry.Status {
	case EntryResolved:
		if entry.DispositionBy != AuthorityCode {
			return fmt.Errorf("%w: %w: resolved entry %q requires code disposition authority", ErrInvalidState, ErrAuthorityDenied, entry.ID)
		}
	case EntryRejected, EntrySuperseded:
		if entry.Authority == AuthorityUser && entry.DispositionBy != AuthorityUser {
			return fmt.Errorf("%w: %w: user-authority entry %q requires user disposition authority", ErrInvalidState, ErrAuthorityDenied, entry.ID)
		}
	}
	return nil
}

func (ledger *Ledger) validateRestoredNodeLinks() error {
	visiting := make(map[NodeID]bool)
	visited := make(map[NodeID]bool)
	var visit func(NodeID) error
	visit = func(id NodeID) error {
		if visiting[id] {
			return fmt.Errorf("%w: restored node hierarchy contains a cycle at %q", ErrInvalidState, id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		node := ledger.nodes[id]
		for _, next := range []NodeID{node.ParentID, node.ObjectiveID} {
			if next != "" {
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		visiting[id], visited[id] = false, true
		return nil
	}
	for id := range ledger.nodes {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateEntryStatus(status EntryStatus) error {
	switch status {
	case EntryActive, EntryResolved, EntryRejected, EntrySuperseded:
		return nil
	default:
		return fmt.Errorf("%w: entry status %q is not registered", ErrInvalidState, status)
	}
}
