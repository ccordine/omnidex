package cognitionreplay

func cloneTerminalAuthority(value TerminalAuthority) TerminalAuthority {
	result := TerminalAuthority{Schema: value.Schema, Kind: value.Kind}
	if value.sealedEpisode != nil {
		copy := *value.sealedEpisode
		result.sealedEpisode = &copy
	}
	if value.preEpisodeFailure != nil {
		copy := *value.preEpisodeFailure
		result.preEpisodeFailure = &copy
	}
	return result
}

func cloneRevision(value *PublicRevision) *PublicRevision {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneSourceRecords(values []SourceRecord) []SourceRecord {
	return append([]SourceRecord(nil), values...)
}

func cloneEvents(values []Event) []Event {
	result := make([]Event, len(values))
	for index, value := range values {
		value.Revision = cloneRevision(value.Revision)
		if value.Timing != nil {
			timing := *value.Timing
			value.Timing = &timing
		}
		value.Sources = append([]SourceRef(nil), value.Sources...)
		result[index] = value
	}
	return result
}

func cloneKnowledgeEntry(value KnowledgeEntry) KnowledgeEntry {
	value.SourceEvents = append([]uint64(nil), value.SourceEvents...)
	return value
}

func cloneKnowledgeState(value KnowledgeState) KnowledgeState {
	value.Revision = cloneRevision(value.Revision)
	if value.Entries != nil {
		entries := make([]KnowledgeEntry, len(value.Entries))
		for index, entry := range value.Entries {
			entries[index] = cloneKnowledgeEntry(entry)
		}
		value.Entries = entries
	}
	return value
}

func cloneKnowledgeDelta(value *KnowledgeDelta) *KnowledgeDelta {
	if value == nil {
		return nil
	}
	result := *value
	result.SetRevision = cloneRevision(value.SetRevision)
	if value.Upserts != nil {
		result.Upserts = make([]KnowledgeEntry, len(value.Upserts))
		for index, entry := range value.Upserts {
			result.Upserts[index] = cloneKnowledgeEntry(entry)
		}
	}
	if value.Releases != nil {
		result.Releases = make([]KnowledgeRelease, len(value.Releases))
		copy(result.Releases, value.Releases)
	}
	return &result
}

func cloneKnowledgeCheckpoints(values []KnowledgeCheckpoint) []KnowledgeCheckpoint {
	result := make([]KnowledgeCheckpoint, len(values))
	for index, value := range values {
		value.State = cloneKnowledgeState(value.State)
		value.Delta = cloneKnowledgeDelta(value.Delta)
		result[index] = value
	}
	return result
}

func clonePrivateSources(values []PrivateSource) []PrivateSource {
	return append([]PrivateSource(nil), values...)
}

func clonePrivateEvents(values []PrivateEvent) []PrivateEvent {
	result := make([]PrivateEvent, len(values))
	for index, value := range values {
		value.Sources = append([]PrivateSourceRef(nil), value.Sources...)
		result[index] = value
	}
	return result
}

func clonePrivateFrames(values []PrivateFrame) []PrivateFrame {
	result := make([]PrivateFrame, len(values))
	for index, value := range values {
		if value.Delta != nil {
			delta := *value.Delta
			value.Delta = &delta
		}
		result[index] = value
	}
	return result
}
