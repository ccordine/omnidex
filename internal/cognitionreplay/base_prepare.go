package cognitionreplay

import (
	"fmt"
	"sort"
)

type preparedBase struct {
	manifest    BaseManifest
	sources     []SourceRecord
	events      []Event
	checkpoints []KnowledgeCheckpoint
	chunked     []ChunkedBlobBinding
	blobs       []Blob
}

func prepareBase(input BaseInput) (preparedBase, error) {
	return prepareBaseForStatus(input, SemanticStructural, nil, nil)
}

func prepareSemanticProjection(input SemanticProjectionInput) (preparedBase, error) {
	if input.PublicBundleAuthority.Validate() != nil || input.SealedEpisodeAuthority.Validate() != nil ||
		input.ProductionTraceAuthority.Validate() != nil ||
		input.PublicBundleAuthority.MediaType != "application/json" ||
		input.SealedEpisodeAuthority.MediaType != "application/json" ||
		input.ProductionTraceAuthority.MediaType != "application/json" ||
		validateProjectionSidecars(input.Sidecars) != nil {
		return preparedBase{}, fmt.Errorf("semantic projection authorities are invalid")
	}
	authority := &ProjectionAuthority{
		Schema:          ProjectionAuthoritySchemaV2,
		PublicBundle:    cloneProjectionContent(input.PublicBundleAuthority),
		SealedEpisode:   cloneProjectionContent(input.SealedEpisodeAuthority),
		ProductionTrace: cloneProjectionContent(input.ProductionTraceAuthority),
		Sidecars:        cloneProjectionSidecars(input.Sidecars),
	}
	return prepareBaseForStatus(input.baseInput(), SemanticProjection, authority, nil)
}

func prepareAblationSemanticProjection(
	input AblationSemanticProjectionInput,
) (preparedBase, error) {
	role, validClass := input.ClaimedClass.publicContentRole()
	contents := []ProjectionContentAuthority{
		input.PublicBundleAuthority, input.SealedEpisodeAuthority,
		input.AblationEvidenceAuthority, input.BrainBootstrapAuthority,
		input.ProviderActivationAuthority,
	}
	if !validClass || role != ChunkedBlobPublicAgentKnowledge ||
		(input.ClaimedClass == AblationProjectionContaminated) != input.PrivateOverlayRequired ||
		validateProjectionContentsForRole(contents, role) != nil ||
		validateProjectionSidecarsForRole(input.Sidecars, role) != nil {
		return preparedBase{}, fmt.Errorf(
			"public ablation semantic projection authorities are invalid or contaminated",
		)
	}
	authority := &AblationProjectionAuthority{
		Schema:                 AblationProjectionAuthoritySchemaV1,
		RegistryID:             AblationSourceRegistryIDV1,
		RegistrySHA256:         ablationSourceRegistrySHA256V1,
		ClaimedClass:           input.ClaimedClass,
		PrivateOverlayRequired: input.PrivateOverlayRequired,
		PublicBundle:           cloneProjectionContent(input.PublicBundleAuthority),
		SealedEpisode:          cloneProjectionContent(input.SealedEpisodeAuthority),
		AblationEvidence:       cloneProjectionContent(input.AblationEvidenceAuthority),
		BrainBootstrap:         cloneProjectionContent(input.BrainBootstrapAuthority),
		ProviderActivation:     cloneProjectionContent(input.ProviderActivationAuthority),
		Sidecars:               cloneProjectionSidecars(input.Sidecars),
	}
	return prepareBaseForStatus(input.baseInput(), SemanticAblationProjection, nil, authority)
}

func prepareBaseForStatus(
	input BaseInput,
	status SemanticStatus,
	projection *ProjectionAuthority,
	ablationProjection *AblationProjectionAuthority,
) (preparedBase, error) {
	if input.TerminalAuthority.Validate() != nil ||
		!validDigest(input.PublicWorldSHA256) || !validDigest(input.PublicAuthoritySHA256) ||
		requireExact(input.PublicWorldSchema, "public world schema") != nil {
		return preparedBase{}, fmt.Errorf("public replay source authority is invalid")
	}
	terminalSHA, err := digestCanonical(input.TerminalAuthority)
	if err != nil {
		return preparedBase{}, err
	}
	result := preparedBase{
		sources: cloneSourceRecords(input.Sources), events: cloneEvents(input.Events),
		chunked: cloneChunkedBlobBindings(input.ChunkedBlobs), blobs: cloneBlobs(input.Blobs),
	}
	checkpoints, err := prepareKnowledgeCheckpoints(input.Checkpoints)
	if err != nil {
		return preparedBase{}, err
	}
	result.checkpoints = checkpoints
	if err := prepareEventChain(result.events); err != nil {
		return preparedBase{}, err
	}
	sort.Slice(result.blobs, func(left, right int) bool {
		return result.blobs[left].SHA256 < result.blobs[right].SHA256
	})
	sort.Slice(result.chunked, func(left, right int) bool {
		return result.chunked[left].Manifest.SHA256 < result.chunked[right].Manifest.SHA256
	})
	sourceSHA, err := digestCanonical(result.sources)
	if err != nil {
		return preparedBase{}, err
	}
	eventSHA, err := digestCanonical(result.events)
	if err != nil {
		return preparedBase{}, err
	}
	checkpointSHA, err := digestCanonical(result.checkpoints)
	if err != nil {
		return preparedBase{}, err
	}
	chunkedSHA, err := digestCanonical(result.chunked)
	if err != nil {
		return preparedBase{}, err
	}
	result.manifest = BaseManifest{
		Schema: BaseSchemaV2, Container: containerBase, SemanticStatus: status,
		PrivateData: false, TerminalAuthority: cloneTerminalAuthority(input.TerminalAuthority),
		TerminalAuthoritySHA256: terminalSHA,
		PublicWorldSHA256:       input.PublicWorldSHA256, PublicWorldSchema: input.PublicWorldSchema,
		PublicAuthoritySHA256:       input.PublicAuthoritySHA256,
		ProjectionAuthority:         projection,
		AblationProjectionAuthority: ablationProjection,
		SourceCount:                 len(result.sources), EventCount: len(result.events),
		CheckpointCount: len(result.checkpoints), BlobCount: len(result.blobs),
		ChunkedBlobCount:  len(result.chunked),
		SourceIndexSHA256: sourceSHA, EventIndexSHA256: eventSHA,
		CheckpointIndexSHA256: checkpointSHA, ChunkedBlobIndexSHA256: chunkedSHA,
		ChunkedBlobs: cloneChunkedBlobBindings(result.chunked), Entries: []ContainerEntry{},
	}
	switch status {
	case SemanticProjection:
		result.manifest.SourceMappings, err = deriveSemanticMappings(result.sources, result.events)
	case SemanticAblationProjection:
		result.manifest.SourceMappings, err = deriveAblationMappings(result.sources, result.events)
	default:
		result.manifest.SourceMappings, err = deriveStructuralMappings(result.sources, result.events)
	}
	if err != nil {
		return preparedBase{}, err
	}
	if err := validatePreparedBase(result.manifest, result.sources, result.events,
		result.checkpoints, result.chunked, blobsByDigest(result.blobs)); err != nil {
		return preparedBase{}, err
	}
	return result, nil
}

func prepareEventChain(events []Event) error {
	previous := ""
	for index := range events {
		event := &events[index]
		if event.PreviousSHA256 != "" || event.EventSHA256 != "" {
			return fmt.Errorf("unprepared replay event %d contains derived identity", index+1)
		}
		event.PreviousSHA256 = previous
		sha, err := eventDigest(*event)
		if err != nil {
			return err
		}
		event.EventSHA256 = sha
		previous = sha
	}
	return nil
}

func eventDigest(event Event) (string, error) {
	event.EventSHA256 = ""
	return digestCanonical(event)
}

func blobsByDigest(values []Blob) map[string]Blob {
	result := make(map[string]Blob, len(values))
	for _, value := range values {
		result[value.SHA256] = value
	}
	return result
}
