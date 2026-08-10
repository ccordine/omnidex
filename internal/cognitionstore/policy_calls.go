package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	if store == nil || store.repository == nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return (queue.CognitionPolicyCallJournal{Repository: store.repository}).Start(ctx, attempt)
}

func (store *Store) Finish(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition call journal is uninitialized")
	}
	return (queue.CognitionPolicyCallJournal{Repository: store.repository}).Finish(
		ctx, attempt, result, evidence,
	)
}

func (store *Store) ReadModelResponseEvidence(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionPolicyEvidencePage{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return store.repository.ReadCognitionModelResponseEvidence(
		ctx, episodeID, evidenceID, offset, limit,
	)
}

func (store *Store) ReadProviderGenerationEvidence(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionPolicyEvidencePage{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return store.repository.ReadCognitionProviderGenerationEvidence(
		ctx, episodeID, evidenceID, offset, limit,
	)
}

func (store *Store) ReadProviderResponseCapture(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionPolicyEvidencePage{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return store.repository.ReadCognitionProviderResponseCapture(
		ctx, episodeID, evidenceID, offset, limit,
	)
}

func (store *Store) ReadProviderIdentityEvidenceManifest(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
) (queue.CognitionProviderIdentityEvidenceManifest, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return store.repository.ReadCognitionProviderIdentityEvidenceManifest(ctx, episodeID, evidenceID)
}

func (store *Store) ReadProviderIdentityEvidenceBody(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	operationIndex int,
	kind queue.CognitionProviderIdentityBodyKind,
	offset int,
	limit int,
) (queue.CognitionProviderIdentityEvidenceBodyPage, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return store.repository.ReadCognitionProviderIdentityEvidenceBody(
		ctx, episodeID, evidenceID, operationIndex, kind, offset, limit,
	)
}
