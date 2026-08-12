package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func buildAblationSemanticReplay(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	evidence ablationEvidenceArtifact,
	bootstrapRaw []byte,
	activationRaw []byte,
	class AblationReplayClass,
) (cognitionreplay.AblationSemanticProjectionInput, error) {
	if err := verifyAblationSemanticAuthorities(
		bundle, episode, evidence, bootstrapRaw, activationRaw, class,
	); err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	authorityRaw, err := encodeAblationEvidenceArtifact(evidence)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	builder := newAblationSemanticBuild()
	units, err := ablationSemanticUnits(evidence.Root)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	for _, unit := range units {
		if err := builder.appendUnit(unit, digestExactBytes(authorityRaw)); err != nil {
			return cognitionreplay.AblationSemanticProjectionInput{}, err
		}
	}
	builder.finishCheckpoint(true)
	publicBundle, publicChunked, publicBlobs, err := semanticReplayProjectionContent(
		"ablation-public-inference-bundle", bundle,
	)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	sealedEpisode, episodeChunked, episodeBlobs, err := semanticReplayProjectionContent(
		"ablation-sealed-episode", episode,
	)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	evidenceContent, evidenceChunked, evidenceBlobs, err := cognitionreplay.NewPublicProjectionContent(
		"ablation-evidence", "application/json", authorityRaw,
	)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	bootstrap, bootstrapChunked, bootstrapBlobs, err := cognitionreplay.NewPublicProjectionContent(
		"ablation-runtime-brain-bootstrap", "application/json", bootstrapRaw,
	)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	activation, activationChunked, activationBlobs, err := cognitionreplay.NewPublicProjectionContent(
		"ablation-runtime-provider-activation", "application/json", activationRaw,
	)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	blobs := append(builder.blobs, publicBlobs...)
	blobs = append(blobs, episodeBlobs...)
	blobs = append(blobs, evidenceBlobs...)
	blobs = append(blobs, bootstrapBlobs...)
	blobs = append(blobs, activationBlobs...)
	blobs, err = uniqueReplayBlobs(blobs)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	chunked := append(publicChunked, episodeChunked...)
	chunked = append(chunked, evidenceChunked...)
	chunked = append(chunked, bootstrapChunked...)
	chunked = append(chunked, activationChunked...)
	projectionClass, err := ablationProjectionClass(class)
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	terminal, err := cognitionreplay.NewSealedEpisodeTerminal(cognitionreplay.SealedEpisodeTerminal{
		EpisodeID: string(episode.Manifest.EpisodeID), EpisodeSealSHA256: episode.SealSHA256,
		TraceSHA256: episode.Manifest.TraceSHA256, EpisodeStartedAt: episode.Manifest.EpisodeStartedAt,
		SealedAt: episode.Manifest.SealedAt,
	})
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return cognitionreplay.AblationSemanticProjectionInput{}, err
	}
	return cognitionreplay.AblationSemanticProjectionInput{
		TerminalAuthority: terminal, PublicWorldSHA256: bundle.Authority.Scenario.SHA256,
		PublicWorldSchema:     semanticReplayPublicWorldSchema,
		PublicAuthoritySHA256: publicSHA,
		ClaimedClass:          projectionClass, PrivateOverlayRequired: false,
		PublicBundleAuthority: publicBundle, SealedEpisodeAuthority: sealedEpisode,
		AblationEvidenceAuthority: evidenceContent, BrainBootstrapAuthority: bootstrap,
		ProviderActivationAuthority: activation, Sidecars: []cognitionreplay.ProjectionSidecarAuthority{},
		Sources: builder.sources, Events: builder.events, Checkpoints: builder.checkpoints,
		ChunkedBlobs: chunked, Blobs: blobs,
	}, nil
}

func newAblationSemanticBuild() *ablationSemanticBuild {
	return &ablationSemanticBuild{
		entries:         make(map[string]cognitionreplay.KnowledgeEntry),
		intervalUpserts: make(map[string]cognitionreplay.KnowledgeEntry),
		checkpoints: []cognitionreplay.KnowledgeCheckpoint{{
			Sequence: 1, AfterEvent: 0,
			State: cognitionreplay.KnowledgeState{
				Schema:  cognitionreplay.KnowledgeStateSchemaV1,
				Entries: []cognitionreplay.KnowledgeEntry{},
			},
		}},
	}
}

func ablationProjectionClass(class AblationReplayClass) (cognitionreplay.AblationProjectionClass, error) {
	switch class {
	case AblationReplaySerious:
		return cognitionreplay.AblationProjectionSerious, nil
	case AblationReplayBenchmarkOnly:
		return cognitionreplay.AblationProjectionBenchmarkOnly, nil
	default:
		return "", fmt.Errorf("ablation replay class requires a private overlay")
	}
}

func (builder *ablationSemanticBuild) appendUnit(
	unit ablationSemanticUnit,
	evidenceSHA string,
) error {
	unitSHA, err := digestJSON(unit.value)
	if err != nil {
		return err
	}
	payload := ablationSemanticSourcePayload{
		Schema: ablationSemanticSourceSchemaV1, EvidenceRootSHA256: evidenceSHA,
		Kind: unit.kind, ID: unit.id, UnitSHA256: unitSHA, Revision: unit.revision,
	}
	blob, err := cognitionreplay.NewCanonicalJSONBlob(payload)
	if err != nil {
		return err
	}
	source := cognitionreplay.SourceRecord{
		Ordinal: uint64(len(builder.sources) + 1), CallOrdinal: unit.callOrdinal,
		Phase: unit.phase, Sequence: unit.sequence, Kind: unit.kind, ID: unit.id,
		Payload: blob.Ref(),
	}
	if source.Validate() != nil || len(unit.events) == 0 {
		return fmt.Errorf("ablation semantic source %q is invalid", unit.id)
	}
	builder.sources = append(builder.sources, source)
	builder.blobs = append(builder.blobs, blob)
	for eventIndex, kind := range unit.events {
		event := cognitionreplay.Event{
			Sequence: uint64(len(builder.events) + 1), Kind: kind,
			MappingSchema: cognitionreplay.AblationSemanticMappingSchemaV1,
			Sources:       []cognitionreplay.SourceRef{source.Ref()}, Payload: blob.Ref(),
		}
		if unit.revision > 0 {
			event.Revision = &cognitionreplay.PublicRevision{Number: unit.revision, SHA256: unit.revisionSHA}
		}
		builder.events = append(builder.events, event)
		if len(unit.knowledge) > eventIndex {
			builder.applyKnowledgeChange(event, unit.knowledge[eventIndex])
		} else {
			builder.applyKnowledge(event, source)
		}
		if len(builder.events)-int(builder.checkpoints[len(builder.checkpoints)-1].AfterEvent) ==
			cognitionreplay.MaxCheckpointInterval {
			builder.finishCheckpoint(false)
		}
	}
	return nil
}

func (builder *ablationSemanticBuild) applyKnowledge(
	event cognitionreplay.Event,
	source cognitionreplay.SourceRecord,
) {
	kind, status, authority, ok := knowledgeDisposition(event.Kind)
	if !ok {
		return
	}
	ref := "ablation://" + source.Kind + "/" + source.ID
	key := string(kind) + "\x00" + ref
	provenance := []uint64{event.Sequence}
	if previous, exists := builder.entries[key]; exists {
		provenance = append(previous.SourceEvents, event.Sequence)
	}
	builder.entries[key] = cognitionreplay.KnowledgeEntry{
		Kind: kind, Ref: ref, Status: status, Authority: authority,
		Content: event.Payload, SourceEvents: provenance,
	}
	builder.intervalUpserts[key] = builder.entries[key]
}

func (builder *ablationSemanticBuild) applyKnowledgeChange(
	event cognitionreplay.Event,
	change *semanticKnowledgeChange,
) {
	if change == nil {
		return
	}
	key := string(change.Kind) + "\x00" + change.Ref
	provenance := []uint64{event.Sequence}
	if previous, exists := builder.entries[key]; exists {
		provenance = append(previous.SourceEvents, event.Sequence)
	}
	builder.entries[key] = cognitionreplay.KnowledgeEntry{
		Kind: change.Kind, Ref: change.Ref, Status: change.Status,
		Authority: change.Authority, Content: event.Payload, SourceEvents: provenance,
	}
	builder.intervalUpserts[key] = builder.entries[key]
}

func (builder *ablationSemanticBuild) finishCheckpoint(final bool) {
	previous := builder.checkpoints[len(builder.checkpoints)-1]
	after := uint64(len(builder.events))
	if after == previous.AfterEvent {
		return
	}
	if !final && after-previous.AfterEvent < cognitionreplay.MaxCheckpointInterval {
		return
	}
	entries := semanticReplaySortedEntries(builder.entries)
	revision := semanticReplayLatestRevision(builder.events)
	upserts := semanticReplaySortedEntries(builder.intervalUpserts)
	builder.checkpoints = append(builder.checkpoints, cognitionreplay.KnowledgeCheckpoint{
		Sequence: uint64(len(builder.checkpoints) + 1), AfterEvent: after,
		State: cognitionreplay.KnowledgeState{
			Schema: cognitionreplay.KnowledgeStateSchemaV1, Revision: revision, Entries: entries,
		}, Delta: &cognitionreplay.KnowledgeDelta{
			Schema: cognitionreplay.KnowledgeDeltaSchemaV1, FromEvent: previous.AfterEvent + 1,
			ThroughEvent: after, SetRevision: revision,
			Upserts: upserts, Releases: []cognitionreplay.KnowledgeRelease{},
		},
	})
	builder.intervalUpserts = make(map[string]cognitionreplay.KnowledgeEntry)
	sort.Slice(builder.sources, func(left, right int) bool {
		return builder.sources[left].Ordinal < builder.sources[right].Ordinal
	})
}
