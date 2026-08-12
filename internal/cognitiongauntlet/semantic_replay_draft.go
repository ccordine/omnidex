package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

type semanticEventDraft struct {
	Kind             cognitionreplay.EventKind
	Revision         *cognitionreplay.PublicRevision
	Payload          cognitionreplay.BlobRef
	Source           *cognitionreplay.SourceRecord
	Knowledge        *semanticKnowledgeChange
	KnowledgeChanges []*semanticKnowledgeChange
}

func (draft semanticEventDraft) withSource(
	source cognitionreplay.SourceRecord,
) semanticEventDraft {
	copy := source
	draft.Source = &copy
	return draft
}

type semanticKnowledgeChange struct {
	Kind      cognitionreplay.KnowledgeKind
	Ref       string
	Status    cognitionreplay.KnowledgeStatus
	Authority cognitionreplay.KnowledgeAuthority
}

func sourceDraft(
	kind cognitionreplay.EventKind,
	source cognitionreplay.SourceRecord,
) semanticEventDraft {
	return semanticEventDraft{Kind: kind, Payload: source.Payload}
}

func (state *semanticReplayState) typedDraft(
	kind cognitionreplay.EventKind,
	source cognitionreplay.SourceRecord,
	payload any,
	revision *cognitionreplay.PublicRevision,
	knowledge *semanticKnowledgeChange,
) (semanticEventDraft, error) {
	blob, err := cognitionreplay.NewCanonicalJSONBlob(payload)
	if err != nil {
		return semanticEventDraft{}, err
	}
	state.eventBlobs = append(state.eventBlobs, blob)
	return semanticEventDraft{
		Kind: kind, Revision: revision, Payload: blob.Ref(), Knowledge: knowledge,
	}, nil
}

func knowledgeChange(
	kind cognitionreplay.KnowledgeKind,
	ref string,
	status cognitionreplay.KnowledgeStatus,
	authority cognitionreplay.KnowledgeAuthority,
) *semanticKnowledgeChange {
	return &semanticKnowledgeChange{Kind: kind, Ref: ref, Status: status, Authority: authority}
}

func sourceKnowledgeDraft(
	kind cognitionreplay.EventKind,
	source cognitionreplay.SourceRecord,
	knowledgeKind cognitionreplay.KnowledgeKind,
	status cognitionreplay.KnowledgeStatus,
	authority cognitionreplay.KnowledgeAuthority,
) semanticEventDraft {
	draft := sourceDraft(kind, source)
	draft.Knowledge = knowledgeChange(
		knowledgeKind, "source://"+source.Kind+"/"+source.ID, status, authority,
	)
	return draft
}

func appendTypedDraft(
	state *semanticReplayState,
	values []semanticEventDraft,
	kind cognitionreplay.EventKind,
	source cognitionreplay.SourceRecord,
	payload any,
	revision *cognitionreplay.PublicRevision,
	knowledge *semanticKnowledgeChange,
) ([]semanticEventDraft, error) {
	draft, err := state.typedDraft(kind, source, payload, revision, knowledge)
	if err != nil {
		return nil, fmt.Errorf("derive %s event payload: %w", kind, err)
	}
	return append(values, draft), nil
}
