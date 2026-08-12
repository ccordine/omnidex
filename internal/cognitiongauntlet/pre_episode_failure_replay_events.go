package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func fixedProviderFailureReplaySources(
	episodeID string,
	record queue.CognitionProviderActivationFailureRecord,
	public cognitionreplay.Blob,
	authority cognitionreplay.Blob,
	receipt cognitionreplay.Blob,
	evidence cognitionreplay.Blob,
) []cognitionreplay.SourceRecord {
	return []cognitionreplay.SourceRecord{
		{
			Ordinal: 1, CallOrdinal: 0, Phase: 1, Sequence: 1,
			Kind: cognitionreplay.SourcePublicRunAuthority, ID: episodeID, Payload: public.Ref(),
		},
		{
			Ordinal: 2, CallOrdinal: 0, Phase: 1, Sequence: 2,
			Kind: cognitionreplay.SourceProviderFailureAuthority,
			ID:   record.RecordID, Payload: authority.Ref(),
		},
		{
			Ordinal: 3, CallOrdinal: 0, Phase: 1, Sequence: 3,
			Kind: cognitionreplay.SourceBrainBootstrapFailureReceipt,
			ID:   record.Bootstrap.ID, Payload: receipt.Ref(),
		},
		{
			Ordinal: 4, CallOrdinal: 0, Phase: 1, Sequence: 4,
			Kind: cognitionreplay.SourceProviderIdentityEvidence,
			ID:   record.Evidence.Ref.ID, Payload: evidence.Ref(),
		},
	}
}

func providerFailureReplayEvents(
	evidence cognitionreplay.ProviderIdentityEvidenceReplay,
	publicSource cognitionreplay.SourceRecord,
	authoritySource cognitionreplay.SourceRecord,
	receiptSource cognitionreplay.SourceRecord,
	evidenceSource cognitionreplay.SourceRecord,
	bodySources []cognitionreplay.SourceRecord,
) ([]cognitionreplay.Event, []cognitionreplay.Blob, error) {
	byOrdinal := make(map[uint64]cognitionreplay.SourceRecord, len(bodySources))
	for _, source := range bodySources {
		byOrdinal[source.Ordinal] = source
	}
	events := make([]cognitionreplay.Event, 0, 6)
	blobs := make([]cognitionreplay.Blob, 0, 5)
	for index, operation := range evidence.Operations {
		payload, err := cognitionreplay.NewCanonicalJSONBlob(
			cognitionreplay.ProviderRequestDispositionReplay{
				Schema:     cognitionreplay.ProviderRequestDispositionReplaySchemaV1,
				EvidenceID: evidence.Ref.ID, OperationIndex: index,
				Operation: operation.Operation, RequestDisposition: operation.RequestDisposition,
				Disposition: operation.Disposition, HTTPStatus: operation.HTTPStatus,
				ResponseComplete: operation.ResponseComplete,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		refs := []cognitionreplay.SourceRef{evidenceSource.Ref()}
		for _, binding := range []cognitionreplay.ProviderIdentityBodyBinding{
			operation.Request, operation.Response,
		} {
			if binding.Source == nil {
				continue
			}
			if source, exists := byOrdinal[binding.Source.Ordinal]; !exists || source.Ref() != *binding.Source {
				return nil, nil, fmt.Errorf("provider failure event body source changed")
			}
			refs = append(refs, *binding.Source)
		}
		sort.Slice(refs, func(left, right int) bool { return refs[left].Ordinal < refs[right].Ordinal })
		events = append(events, cognitionreplay.Event{
			Sequence: uint64(index + 1), Kind: cognitionreplay.EventProviderRequestDisposition,
			MappingSchema: cognitionreplay.StructuralMappingSchemaV1,
			Sources:       refs, Payload: payload.Ref(),
		})
		blobs = append(blobs, payload)
	}
	failureSources := []cognitionreplay.SourceRef{
		publicSource.Ref(),
		authoritySource.Ref(), receiptSource.Ref(), evidenceSource.Ref(),
	}
	return append(events, cognitionreplay.Event{
		Sequence: 6, Kind: cognitionreplay.EventFailureRecorded,
		MappingSchema: cognitionreplay.StructuralMappingSchemaV1,
		Sources:       failureSources, Payload: receiptSource.Payload,
	}), blobs, nil
}

func providerFailureReplayCheckpoints(
	recordID string,
	receipt cognitionreplay.BlobRef,
) []cognitionreplay.KnowledgeCheckpoint {
	entry := cognitionreplay.KnowledgeEntry{
		Kind: cognitionreplay.KnowledgeFailure, Ref: "failure://" + recordID,
		Status: cognitionreplay.KnowledgeFailed, Authority: cognitionreplay.AuthorityTool,
		Content: receipt, SourceEvents: []uint64{6},
	}
	return []cognitionreplay.KnowledgeCheckpoint{
		{
			Sequence: 1, AfterEvent: 0,
			State: cognitionreplay.KnowledgeState{
				Schema:  cognitionreplay.KnowledgeStateSchemaV1,
				Entries: []cognitionreplay.KnowledgeEntry{},
			},
		},
		{
			Sequence: 2, AfterEvent: 6,
			State: cognitionreplay.KnowledgeState{
				Schema:  cognitionreplay.KnowledgeStateSchemaV1,
				Entries: []cognitionreplay.KnowledgeEntry{entry},
			},
			Delta: &cognitionreplay.KnowledgeDelta{
				Schema:    cognitionreplay.KnowledgeDeltaSchemaV1,
				FromEvent: 1, ThroughEvent: 6,
				Upserts:  []cognitionreplay.KnowledgeEntry{entry},
				Releases: []cognitionreplay.KnowledgeRelease{},
			},
		},
	}
}
