package taskstate

import "fmt"

func (command AddEntryCommand) decide(ledger *Ledger) (Event, error) {
	if err := validateNewEntryAuthority(command.Actor, command.Kind); err != nil {
		return Event{}, err
	}
	if err := requireExactText(string(command.ID), "entry ID"); err != nil {
		return Event{}, err
	}
	if _, exists := ledger.entries[command.ID]; exists {
		return Event{}, fmt.Errorf("%w: entry %q already exists", ErrInvalidState, command.ID)
	}
	if command.ScopeNodeID != "" {
		if _, exists := ledger.nodes[command.ScopeNodeID]; !exists {
			return Event{}, fmt.Errorf("%w: scope node %q", ErrNotFound, command.ScopeNodeID)
		}
		if ledger.nodeSuperseded(command.ScopeNodeID) {
			return Event{}, fmt.Errorf("%w: scope node %q is superseded", ErrInvalidState, command.ScopeNodeID)
		}
	}
	if err := requireExactText(command.Content, "entry content"); err != nil {
		return Event{}, err
	}
	if command.Confidence != nil && (*command.Confidence < 0 || *command.Confidence > 1) {
		return Event{}, fmt.Errorf("%w: confidence must be between zero and one", ErrInvalidCommand)
	}
	if err := validateOptionalStep(command.CreatedStepID, "created step ID"); err != nil {
		return Event{}, err
	}
	if err := command.Metadata.Validate(); err != nil {
		return Event{}, fmt.Errorf("%w: invalid entry metadata: %v", ErrInvalidCommand, err)
	}
	if err := validateRefs(command.Refs); err != nil {
		return Event{}, err
	}
	if err := validateFeedback(command.Kind, command.FeedbackPurpose, command.Actor); err != nil {
		return Event{}, err
	}
	if command.Kind == EntryFact && !hasEvidenceRef(command.Refs) {
		return Event{}, fmt.Errorf("%w: facts require an evidence, supports, or verifies reference", ErrEvidenceRequired)
	}
	entry := Entry{
		ID: command.ID, ScopeNodeID: command.ScopeNodeID, Kind: command.Kind,
		FeedbackPurpose: command.FeedbackPurpose, Status: EntryActive,
		Authority: command.Actor, CreatedBy: command.Actor, Content: command.Content,
		ContentSHA256: contentDigest(command.Content), Confidence: cloneFloat64(command.Confidence),
		CreatedStepID: cloneInt64(command.CreatedStepID), Metadata: cloneJSONObject(command.Metadata),
		Refs: normalizedRefs(command.Refs), CreatedVersion: ledger.version + 1, UpdatedVersion: ledger.version + 1,
	}
	return Event{Kind: EventEntryAdded, Entry: &entry, StepID: cloneInt64(command.CreatedStepID)}, nil
}

func (command RejectEntryCommand) decide(ledger *Ledger) (Event, error) {
	if command.Actor != AuthorityCode && command.Actor != AuthorityUser {
		return Event{}, fmt.Errorf("%w: only code or user may reject entries", ErrAuthorityDenied)
	}
	entry, exists := ledger.entries[command.EntryID]
	if !exists {
		return Event{}, fmt.Errorf("%w: entry %q", ErrNotFound, command.EntryID)
	}
	if entry.Status != EntryActive {
		return Event{}, fmt.Errorf("%w: only active entries may be rejected", ErrInvalidState)
	}
	if entry.Authority == AuthorityUser && command.Actor != AuthorityUser {
		return Event{}, fmt.Errorf("%w: only user authority may reject user-authority state", ErrAuthorityDenied)
	}
	if err := requireExactText(command.Reason, "rejection reason"); err != nil {
		return Event{}, err
	}
	if err := validateRefs(command.Refs); err != nil {
		return Event{}, err
	}
	if entry.Kind == EntryHypothesis && command.Actor == AuthorityCode && !hasContradictionRef(command.Refs) {
		return Event{}, fmt.Errorf("%w: code rejection of a hypothesis requires contradiction evidence", ErrEvidenceRequired)
	}
	return Event{
		Kind: EventEntryRejected, EntryID: entry.ID, Reason: command.Reason,
		VerificationRefs: cloneRefs(command.Refs),
	}, nil
}

func (command ResolveEntryCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "resolve entries"); err != nil {
		return Event{}, err
	}
	entry, exists := ledger.entries[command.EntryID]
	if !exists {
		return Event{}, fmt.Errorf("%w: entry %q", ErrNotFound, command.EntryID)
	}
	if entry.Status != EntryActive {
		return Event{}, fmt.Errorf("%w: only active entries may be resolved", ErrInvalidState)
	}
	switch entry.Kind {
	case EntryQuestion, EntryFailure, EntryObservation, EntryNote, EntryFeedback:
	default:
		return Event{}, fmt.Errorf("%w: entry kind %q cannot be resolved", ErrInvalidState, entry.Kind)
	}
	if err := requireExactText(command.Reason, "resolution reason"); err != nil {
		return Event{}, err
	}
	if err := validateRefs(command.Refs); err != nil {
		return Event{}, err
	}
	if !hasEvidenceRef(command.Refs) {
		return Event{}, fmt.Errorf("%w: resolution requires evidence", ErrEvidenceRequired)
	}
	return Event{Kind: EventEntryResolved, EntryID: entry.ID,
		Reason: command.Reason, VerificationRefs: cloneRefs(command.Refs)}, nil
}

func (command SupersedeEntryCommand) decide(ledger *Ledger) (Event, error) {
	if command.Actor != AuthorityCode && command.Actor != AuthorityUser {
		return Event{}, fmt.Errorf("%w: only code or user may supersede entries", ErrAuthorityDenied)
	}
	entry, exists := ledger.entries[command.EntryID]
	if !exists {
		return Event{}, fmt.Errorf("%w: entry %q", ErrNotFound, command.EntryID)
	}
	replacement, replacementExists := ledger.entries[command.ReplacementID]
	if !replacementExists {
		return Event{}, fmt.Errorf("%w: replacement entry %q", ErrNotFound, command.ReplacementID)
	}
	if entry.ID == replacement.ID || entry.Status != EntryActive || replacement.Status != EntryActive {
		return Event{}, fmt.Errorf("%w: supersession requires two different active entries", ErrInvalidState)
	}
	if entry.Kind != replacement.Kind {
		return Event{}, fmt.Errorf("%w: replacement must have the same semantic entry kind", ErrInvalidState)
	}
	if authorityRank(replacement.Authority) < authorityRank(entry.Authority) {
		return Event{}, fmt.Errorf("%w: replacement cannot downgrade entry authority", ErrAuthorityDenied)
	}
	if entry.Authority == AuthorityUser && (command.Actor != AuthorityUser || replacement.Authority != AuthorityUser) {
		return Event{}, fmt.Errorf("%w: only user authority may replace user-authority state", ErrAuthorityDenied)
	}
	if replacement.SupersedesID != "" {
		return Event{}, fmt.Errorf("%w: replacement entry already supersedes another entry", ErrInvalidState)
	}
	if err := requireExactText(command.Reason, "supersession reason"); err != nil {
		return Event{}, err
	}
	return Event{Kind: EventEntrySuperseded, EntryID: entry.ID,
		ReplacementID: replacement.ID, Reason: command.Reason}, nil
}

func (command AcceptDecisionCommand) decide(ledger *Ledger) (Event, error) {
	if command.Actor != AuthorityCode && command.Actor != AuthorityUser {
		return Event{}, fmt.Errorf("%w: only code or user may accept a model decision", ErrAuthorityDenied)
	}
	candidate, exists := ledger.entries[command.CandidateID]
	if !exists {
		return Event{}, fmt.Errorf("%w: decision candidate %q", ErrNotFound, command.CandidateID)
	}
	if candidate.Status != EntryActive || candidate.Kind != EntryDecisionCandidate ||
		candidate.Authority != AuthorityModelProposal {
		return Event{}, fmt.Errorf("%w: acceptance requires an active model decision candidate", ErrInvalidState)
	}
	if err := requireExactText(string(command.AcceptedEntryID), "accepted entry ID"); err != nil {
		return Event{}, err
	}
	if _, exists := ledger.entries[command.AcceptedEntryID]; exists {
		return Event{}, fmt.Errorf("%w: entry %q already exists", ErrInvalidState, command.AcceptedEntryID)
	}
	if err := requireExactText(command.AcceptancePolicy, "acceptance policy"); err != nil {
		return Event{}, err
	}
	if err := validateOptionalStep(command.CreatedStepID, "created step ID"); err != nil {
		return Event{}, err
	}
	if err := command.Metadata.Validate(); err != nil {
		return Event{}, fmt.Errorf("%w: invalid accepted decision metadata: %v", ErrInvalidCommand, err)
	}
	if err := validateRefs(command.AcceptanceRefs); err != nil {
		return Event{}, err
	}
	if !hasEvidenceRef(command.AcceptanceRefs) {
		return Event{}, fmt.Errorf("%w: decision acceptance requires code or tool evidence", ErrEvidenceRequired)
	}
	entry := Entry{
		ID: command.AcceptedEntryID, ScopeNodeID: candidate.ScopeNodeID,
		Kind: EntryAcceptedDecision, Status: EntryActive,
		Authority: AuthorityAcceptedModelDecision, CreatedBy: command.Actor,
		Content: candidate.Content, ContentSHA256: candidate.ContentSHA256,
		Confidence: cloneFloat64(candidate.Confidence), CreatedStepID: cloneInt64(command.CreatedStepID),
		Metadata: cloneJSONObject(command.Metadata), Refs: cloneRefs(command.AcceptanceRefs),
		Provenance: EntryProvenance{SourceEntryID: candidate.ID,
			AcceptancePolicy: command.AcceptancePolicy, AcceptedBy: command.Actor},
		CreatedVersion: ledger.version + 1, UpdatedVersion: ledger.version + 1,
	}
	return Event{Kind: EventDecisionAccepted, EntryID: candidate.ID,
		ReplacementID: entry.ID, Entry: &entry, StepID: cloneInt64(command.CreatedStepID),
		Reason: command.AcceptancePolicy}, nil
}

func validateNewEntryAuthority(actor Authority, kind EntryKind) error {
	if err := validateEntryKind(kind); err != nil {
		return err
	}
	if kind == EntryAcceptedDecision {
		return fmt.Errorf("%w: accepted decisions require the acceptance command", ErrAuthorityDenied)
	}
	if actor == AuthorityAcceptedModelDecision {
		return fmt.Errorf("%w: accepted-model-decision authority cannot create entries directly", ErrAuthorityDenied)
	}
	if actor == AuthorityModelProposal {
		if kind != EntryObservation && kind != EntryHypothesis && kind != EntryQuestion && kind != EntryDecisionCandidate {
			return fmt.Errorf("%w: model proposals may create only observations, hypotheses, questions, and decision candidates", ErrAuthorityDenied)
		}
	}
	return nil
}

func validateFeedback(kind EntryKind, purpose FeedbackPurpose, actor Authority) error {
	if kind != EntryFeedback {
		if purpose != "" {
			return fmt.Errorf("%w: feedback purpose is forbidden for non-feedback entries", ErrInvalidCommand)
		}
		return nil
	}
	if actor != AuthorityUser {
		return fmt.Errorf("%w: feedback entries require direct user authority", ErrAuthorityDenied)
	}
	switch purpose {
	case FeedbackReplan, FeedbackInterrupt, FeedbackInputResponse:
		return nil
	default:
		return fmt.Errorf("%w: feedback purpose %q is not registered", ErrInvalidCommand, purpose)
	}
}

func authorityRank(authority Authority) int {
	switch authority {
	case AuthorityModelProposal:
		return 1
	case AuthorityToolEvidence:
		return 2
	case AuthorityAcceptedModelDecision:
		return 3
	case AuthorityCode:
		return 4
	case AuthorityUser:
		return 5
	default:
		return 0
	}
}
