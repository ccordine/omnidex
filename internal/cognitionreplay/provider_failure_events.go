package cognitionreplay

import "fmt"

func validatePreEpisodeEvents(
	events []Event,
	terminal PreEpisodeBrainBootstrapFailureTerminal,
	evidence ProviderIdentityEvidenceReplay,
	authoritySource SourceRecord,
	receiptSource SourceRecord,
	evidenceSource SourceRecord,
	sources map[uint64]SourceRecord,
	blobs map[string]Blob,
) error {
	if len(events) != 6 {
		return fmt.Errorf("pre-episode replay requires five provider dispositions and one failure")
	}
	for index, operation := range evidence.Operations {
		event := events[index]
		if event.Kind != EventProviderRequestDisposition || event.Revision != nil || event.Timing != nil {
			return fmt.Errorf("pre-episode replay provider event %d changed", index+1)
		}
		wantSources := []SourceRef{evidenceSource.Ref()}
		if operation.Request.Source != nil {
			wantSources = append(wantSources, *operation.Request.Source)
		}
		if operation.Response.Source != nil {
			wantSources = append(wantSources, *operation.Response.Source)
		}
		wantSources = sortedSourceRefs(wantSources...)
		if !sourceRefsEqual(event.Sources, wantSources) {
			return fmt.Errorf("pre-episode replay provider event %d sources changed", index+1)
		}
		payloadBlob, exists := blobs[event.Payload.SHA256]
		if !exists || !event.Payload.matches(payloadBlob) {
			return fmt.Errorf("pre-episode replay provider event payload is unavailable")
		}
		var payload ProviderRequestDispositionReplay
		if err := decodeCanonical(
			payloadBlob.Data, &payload, "provider request disposition event",
		); err != nil || payload.Validate() != nil ||
			payload.EvidenceID != evidence.Ref.ID || payload.OperationIndex != index ||
			payload.Operation != operation.Operation ||
			payload.RequestDisposition != operation.RequestDisposition ||
			payload.Disposition != operation.Disposition || payload.HTTPStatus != operation.HTTPStatus ||
			payload.ResponseComplete != operation.ResponseComplete {
			return fmt.Errorf("pre-episode replay provider event %d payload changed: %v", index+1, err)
		}
	}
	failure := events[5]
	wantFailureSources := sortedSourceRefs(
		terminal.PublicRunAuthority, authoritySource.Ref(), receiptSource.Ref(), evidenceSource.Ref(),
	)
	if failure.Kind != EventFailureRecorded || failure.Revision != nil || failure.Timing != nil ||
		!sourceRefsEqual(failure.Sources, wantFailureSources) || failure.Payload != receiptSource.Payload {
		return fmt.Errorf("pre-episode replay terminal failure event changed")
	}
	for _, ref := range failure.Sources {
		if source, exists := sources[ref.Ordinal]; !exists || source.Ref() != ref {
			return fmt.Errorf("pre-episode replay failure event cites an unknown source")
		}
	}
	return nil
}

func validatePreEpisodeCheckpoints(
	values []KnowledgeCheckpoint,
	terminal PreEpisodeBrainBootstrapFailureTerminal,
	receipt BlobRef,
) error {
	if len(values) != 2 {
		return fmt.Errorf("pre-episode replay requires exactly two knowledge checkpoints")
	}
	want := KnowledgeEntry{
		Kind: KnowledgeFailure, Ref: "failure://" + terminal.RecordID,
		Status: KnowledgeFailed, Authority: AuthorityTool,
		Content: receipt, SourceEvents: []uint64{6},
	}
	final := values[1]
	if final.AfterEvent != 6 || final.State.Revision != nil || len(final.State.Entries) != 1 ||
		final.State.Entries[0].Kind != want.Kind || final.State.Entries[0].Ref != want.Ref ||
		final.State.Entries[0].Status != want.Status ||
		final.State.Entries[0].Authority != want.Authority ||
		final.State.Entries[0].Content != want.Content ||
		!uint64SlicesEqual(final.State.Entries[0].SourceEvents, want.SourceEvents) ||
		final.Delta == nil || final.Delta.SetRevision != nil ||
		len(final.Delta.Upserts) != 1 || len(final.Delta.Releases) != 0 ||
		final.Delta.Upserts[0].Ref != want.Ref {
		return fmt.Errorf("pre-episode replay final failure knowledge changed")
	}
	return nil
}

func uint64SlicesEqual(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
