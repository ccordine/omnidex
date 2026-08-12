package cognitionreplay

import "fmt"

func validatePreparedBase(
	manifest BaseManifest,
	sources []SourceRecord,
	events []Event,
	checkpoints []KnowledgeCheckpoint,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	if err := validateBaseManifestHeader(manifest); err != nil {
		return err
	}
	if len(sources) == 0 || len(sources) > maxSources || len(events) == 0 ||
		len(events) > maxEvents || len(blobs) == 0 || len(blobs) > maxBlobs {
		return fmt.Errorf("public replay record or blob count is invalid")
	}
	if manifest.SourceCount != len(sources) || manifest.EventCount != len(events) ||
		manifest.CheckpointCount != len(checkpoints) || manifest.ChunkedBlobCount != len(chunked) ||
		manifest.BlobCount != len(blobs) {
		return fmt.Errorf("public replay manifest counts changed")
	}
	if err := validateChunkedBlobBindings(
		chunked, manifest.ChunkedBlobs, ChunkedBlobPublicAgentKnowledge,
	); err != nil {
		return err
	}
	if err := validateSources(sources); err != nil {
		return err
	}
	if err := validateEvents(events, sources, manifest.SemanticStatus); err != nil {
		return err
	}
	if err := validateKnowledgeCheckpoints(checkpoints, len(events)); err != nil {
		return err
	}
	if err := validateBaseIndexes(manifest, sources, events, checkpoints, chunked); err != nil {
		return err
	}
	var wantMappings []SourceMapping
	var err error
	switch manifest.SemanticStatus {
	case SemanticProjection:
		wantMappings, err = deriveSemanticMappings(sources, events)
	case SemanticAblationProjection:
		wantMappings, err = deriveAblationMappings(sources, events)
	default:
		wantMappings, err = deriveStructuralMappings(sources, events)
	}
	if err != nil || !equalSourceMappings(manifest.SourceMappings, wantMappings) {
		return fmt.Errorf("public replay source mappings are not exact and exhaustive")
	}
	if err := validateBaseBlobClosure(manifest, sources, events, checkpoints, chunked, blobs); err != nil {
		return err
	}
	return validateTerminalSemantics(manifest, sources, events, checkpoints, chunked, blobs)
}

func validateProjectionAuthority(value *ProjectionAuthority) error {
	if value == nil || value.Schema != ProjectionAuthoritySchemaV2 ||
		value.PublicBundle.Validate() != nil || value.SealedEpisode.Validate() != nil ||
		value.ProductionTrace.Validate() != nil || validateProjectionSidecars(value.Sidecars) != nil ||
		value.PublicBundle.MediaType != "application/json" ||
		value.SealedEpisode.MediaType != "application/json" ||
		value.ProductionTrace.MediaType != "application/json" ||
		value.PublicBundle.ContentSHA256 == value.SealedEpisode.ContentSHA256 ||
		value.PublicBundle.ContentSHA256 == value.ProductionTrace.ContentSHA256 ||
		value.SealedEpisode.ContentSHA256 == value.ProductionTrace.ContentSHA256 {
		return fmt.Errorf("semantic projection authority is invalid")
	}
	return nil
}

func validateAblationProjectionAuthority(value *AblationProjectionAuthority) error {
	if value == nil || value.Schema != AblationProjectionAuthoritySchemaV1 ||
		value.RegistryID != AblationSourceRegistryIDV1 ||
		value.RegistrySHA256 != ablationSourceRegistrySHA256V1 {
		return fmt.Errorf("ablation semantic projection authority is invalid")
	}
	role, validClass := value.ClaimedClass.publicContentRole()
	contents := ablationProjectionContentValues(value)
	if !validClass || role != ChunkedBlobPublicAgentKnowledge ||
		(value.ClaimedClass == AblationProjectionContaminated) != value.PrivateOverlayRequired ||
		validateProjectionContentsForRole(contents, role) != nil ||
		validateProjectionSidecarsForRole(value.Sidecars, role) != nil ||
		value.PublicBundle.MediaType != "application/json" ||
		value.SealedEpisode.MediaType != "application/json" ||
		value.AblationEvidence.MediaType != "application/json" ||
		value.BrainBootstrap.MediaType != "application/json" ||
		value.ProviderActivation.MediaType != "application/json" {
		return fmt.Errorf("ablation semantic projection authority is invalid")
	}
	seen := make(map[string]struct{}, 5)
	for _, content := range contents[:5] {
		if _, duplicate := seen[content.ContentSHA256]; duplicate {
			return fmt.Errorf("ablation semantic projection reuses one authority body")
		}
		seen[content.ContentSHA256] = struct{}{}
	}
	return nil
}

func validateProjectionContentsForRole(
	values []ProjectionContentAuthority,
	role ChunkedBlobRole,
) error {
	for _, value := range values {
		if value.ValidateForRole(role) != nil {
			return fmt.Errorf("semantic projection content has the wrong privacy role")
		}
	}
	return nil
}

func validateProjectionSidecarsForRole(
	values []ProjectionSidecarAuthority,
	role ChunkedBlobRole,
) error {
	if err := validateProjectionSidecars(values); err != nil {
		return err
	}
	for _, value := range values {
		if value.Content.ValidateForRole(role) != nil {
			return fmt.Errorf("semantic projection sidecar has the wrong privacy role")
		}
	}
	return nil
}

func validateProjectionSidecars(values []ProjectionSidecarAuthority) error {
	if values == nil || len(values) > maxSources {
		return fmt.Errorf("semantic projection sidecar authorities are invalid")
	}
	previous := ""
	for _, value := range values {
		key := value.Kind + "\x00" + value.ID
		if requireExact(value.Kind, "projection sidecar kind") != nil ||
			requireExact(value.ID, "projection sidecar ID") != nil || value.Content.Validate() != nil ||
			(previous != "" && key <= previous) {
			return fmt.Errorf("semantic projection sidecar authorities are invalid or reordered")
		}
		previous = key
	}
	return nil
}

func validateBaseManifestHeader(manifest BaseManifest) error {
	terminalSHA, terminalErr := digestCanonical(manifest.TerminalAuthority)
	if manifest.Schema != BaseSchemaV2 || manifest.Container != containerBase ||
		(manifest.SemanticStatus != SemanticStructural && manifest.SemanticStatus != SemanticProjection &&
			manifest.SemanticStatus != SemanticAblationProjection) ||
		manifest.PrivateData ||
		terminalErr != nil || manifest.TerminalAuthority.Validate() != nil ||
		!validDigest(manifest.TerminalAuthoritySHA256) ||
		manifest.TerminalAuthoritySHA256 != terminalSHA ||
		!validDigest(manifest.PublicWorldSHA256) || !validDigest(manifest.PublicAuthoritySHA256) ||
		requireExact(manifest.PublicWorldSchema, "public world schema") != nil ||
		!validDigest(manifest.SourceIndexSHA256) || !validDigest(manifest.EventIndexSHA256) ||
		!validDigest(manifest.CheckpointIndexSHA256) || !validDigest(manifest.ChunkedBlobIndexSHA256) ||
		manifest.SourceMappings == nil || manifest.ChunkedBlobs == nil || manifest.Entries == nil {
		return fmt.Errorf("public replay manifest authority is invalid")
	}
	switch manifest.SemanticStatus {
	case SemanticProjection:
		if _, sealed := manifest.TerminalAuthority.SealedEpisode(); !sealed {
			return fmt.Errorf("semantic replay requires one sealed episode terminal authority")
		}
		if manifest.AblationProjectionAuthority != nil {
			return fmt.Errorf("production semantic replay contains ablation authority")
		}
		if err := validateProjectionAuthority(manifest.ProjectionAuthority); err != nil {
			return err
		}
	case SemanticAblationProjection:
		if _, sealed := manifest.TerminalAuthority.SealedEpisode(); !sealed {
			return fmt.Errorf("ablation semantic replay requires one sealed episode terminal authority")
		}
		if manifest.ProjectionAuthority != nil {
			return fmt.Errorf("ablation semantic replay contains production authority")
		}
		if err := validateAblationProjectionAuthority(manifest.AblationProjectionAuthority); err != nil {
			return err
		}
	default:
		if manifest.ProjectionAuthority != nil || manifest.AblationProjectionAuthority != nil {
			return fmt.Errorf("structural replay cannot claim a semantic projection authority")
		}
	}
	return nil
}

func validateSources(values []SourceRecord) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value.Ordinal != uint64(index+1) || value.Validate() != nil {
			return fmt.Errorf("replay source record %d is invalid", index+1)
		}
		if index > 0 && !sourceRecordLess(values[index-1], value) {
			return fmt.Errorf("replay source records are reordered or duplicated")
		}
		key := value.Kind + "\x00" + value.ID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("replay source record identity is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateEvents(values []Event, sources []SourceRecord, status SemanticStatus) error {
	byOrdinal := make(map[uint64]SourceRef, len(sources))
	cited := make(map[uint64]struct{}, len(sources))
	for _, source := range sources {
		byOrdinal[source.Ordinal] = source.Ref()
	}
	previous := ""
	wantMappingSchema := StructuralMappingSchemaV1
	switch status {
	case SemanticProjection:
		wantMappingSchema = SemanticMappingSchemaV1
	case SemanticAblationProjection:
		wantMappingSchema = AblationSemanticMappingSchemaV1
	}
	for index, event := range values {
		if event.Sequence != uint64(index+1) || !validPublicEventKind(event.Kind) ||
			event.MappingSchema != wantMappingSchema || event.Payload.Validate() != nil ||
			len(event.Sources) == 0 || len(event.Sources) > maxSources ||
			event.PreviousSHA256 != previous || !validDigest(event.EventSHA256) || event.Timing != nil {
			return fmt.Errorf("public replay event %d authority is invalid", index+1)
		}
		if event.Revision != nil && event.Revision.Validate() != nil {
			return fmt.Errorf("public replay event %d revision is invalid", index+1)
		}
		for sourceIndex, ref := range event.Sources {
			if ref.Validate() != nil || byOrdinal[ref.Ordinal] != ref ||
				(sourceIndex > 0 && ref.Ordinal <= event.Sources[sourceIndex-1].Ordinal) {
				return fmt.Errorf("public replay event %d source binding is invalid", index+1)
			}
			cited[ref.Ordinal] = struct{}{}
		}
		want, err := eventDigest(event)
		if err != nil || want != event.EventSHA256 {
			return fmt.Errorf("public replay event %d hash changed", index+1)
		}
		previous = event.EventSHA256
	}
	if len(cited) != len(sources) {
		return fmt.Errorf("public replay contains an orphan sealed source record")
	}
	return nil
}

func validateBaseIndexes(
	manifest BaseManifest,
	sources []SourceRecord,
	events []Event,
	checkpoints []KnowledgeCheckpoint,
	chunked []ChunkedBlobBinding,
) error {
	sourceSHA, sourceErr := digestCanonical(sources)
	eventSHA, eventErr := digestCanonical(events)
	checkpointSHA, checkpointErr := digestCanonical(checkpoints)
	chunkedSHA, chunkedErr := digestCanonical(chunked)
	if sourceErr != nil || eventErr != nil || checkpointErr != nil || chunkedErr != nil ||
		manifest.SourceIndexSHA256 != sourceSHA || manifest.EventIndexSHA256 != eventSHA ||
		manifest.CheckpointIndexSHA256 != checkpointSHA ||
		manifest.ChunkedBlobIndexSHA256 != chunkedSHA {
		return fmt.Errorf("public replay ordered index digest changed")
	}
	return nil
}
