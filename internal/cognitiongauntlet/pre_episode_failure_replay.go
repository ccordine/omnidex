package cognitiongauntlet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func ExportPreEpisodeBrainBootstrapFailureReplay(
	ctx context.Context,
	repository *queue.Repository,
	bundle PublicInferenceBundle,
	authority model.StepAttemptAuthority,
) (cognitionreplay.Artifact, error) {
	if ctx == nil || repository == nil {
		return cognitionreplay.Artifact{}, fmt.Errorf(
			"pre-episode replay requires PostgreSQL and context",
		)
	}
	if err := bundle.Validate(); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	if _, err := repository.CognitionEpisode(ctx, episode.ID); err == nil {
		return cognitionreplay.Artifact{}, fmt.Errorf(
			"pre-episode replay refuses an existing cognition episode %q", episode.ID,
		)
	} else if !errors.Is(err, queue.ErrCognitionEpisodeNotFound) {
		return cognitionreplay.Artifact{}, fmt.Errorf("check pre-episode replay boundary: %w", err)
	}
	record, err := readOneBrainBootstrapFailure(ctx, repository, authority, episode.ID)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	publicRaw, err := json.Marshal(bundle.Authority)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil || digestExactBytes(publicRaw) != publicSHA {
		return cognitionreplay.Artifact{}, fmt.Errorf("public replay authority changed: %v", err)
	}
	publicBlob, err := cognitionreplay.NewBlob("application/json", publicRaw)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	authorityBlob, err := cognitionreplay.NewBlob("application/json", record.AuthorityJSON)
	if err != nil || authorityBlob.SHA256 != record.AuthoritySHA256 {
		return cognitionreplay.Artifact{}, fmt.Errorf("provider failure authority bytes changed: %v", err)
	}
	receiptBlob, err := cognitionreplay.NewBlob("application/json", record.ReceiptJSON)
	if err != nil || receiptBlob.SHA256 != record.ReceiptSHA256 {
		return cognitionreplay.Artifact{}, fmt.Errorf("provider failure receipt bytes changed: %v", err)
	}
	evidence, bodySources, chunked, evidenceBlobs, err := prepareProviderFailureReplayEvidence(
		ctx, repository, authority, episode.ID, record,
	)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	evidenceBlob, err := cognitionreplay.NewCanonicalJSONBlob(evidence)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	sources := fixedProviderFailureReplaySources(
		string(episode.ID), record, publicBlob, authorityBlob, receiptBlob, evidenceBlob,
	)
	sources = append(sources, bodySources...)
	terminal, err := cognitionreplay.NewPreEpisodeBrainBootstrapFailureTerminal(
		cognitionreplay.PreEpisodeBrainBootstrapFailureTerminal{
			RecordID: record.RecordID, RequestedEpisodeID: string(episode.ID), Actor: record.Actor,
			FailureID:          record.Bootstrap.ID,
			PublicRunAuthority: sources[0].Ref(), FailureAuthority: sources[1].Ref(),
			FailureReceipt: sources[2].Ref(), IdentityEvidence: sources[3].Ref(),
		},
	)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	events, eventBlobs, err := providerFailureReplayEvents(
		evidence, sources[0], sources[1], sources[2], sources[3], bodySources,
	)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	checkpoints := providerFailureReplayCheckpoints(record.RecordID, receiptBlob.Ref())
	blobs, err := uniqueReplayBlobs(append(
		[]cognitionreplay.Blob{publicBlob, authorityBlob, receiptBlob, evidenceBlob},
		append(evidenceBlobs, eventBlobs...)...,
	))
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	return cognitionreplay.ExportStructuralBase(cognitionreplay.BaseInput{
		TerminalAuthority: terminal, PublicWorldSHA256: bundle.Authority.Scenario.SHA256,
		PublicWorldSchema: labyrinth.PublicWorldSchemaV1, PublicAuthoritySHA256: publicSHA,
		Sources: sources, Events: events, Checkpoints: checkpoints,
		ChunkedBlobs: chunked, Blobs: blobs,
	})
}

func readOneBrainBootstrapFailure(
	ctx context.Context,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
) (queue.CognitionProviderActivationFailureRecord, error) {
	page, err := repository.ReadCognitionProviderActivationFailurePage(
		ctx, queue.CognitionProviderActivationFailurePageRequest{
			Authority: authority, EpisodeID: episodeID, AfterRecordNumber: 0, Limit: 1,
		},
	)
	if err != nil {
		return queue.CognitionProviderActivationFailureRecord{}, err
	}
	if page.TotalRecords != 1 || len(page.Records) != 1 ||
		page.NextRecordNumber != page.Records[0].RecordNumber {
		return queue.CognitionProviderActivationFailureRecord{}, fmt.Errorf(
			"pre-episode replay requires exactly one provider activation failure",
		)
	}
	record := page.Records[0]
	if record.Kind != "brain_bootstrap" || record.Bootstrap == nil || record.Process != nil ||
		record.SuccessfulBootstrap != nil || record.BootstrapEvidence != nil ||
		record.EpisodeID != episodeID || len(record.AuthorityJSON) == 0 || len(record.ReceiptJSON) == 0 {
		return queue.CognitionProviderActivationFailureRecord{}, fmt.Errorf(
			"pre-episode replay requires one exact Brain bootstrap failure outcome",
		)
	}
	return record, nil
}

func uniqueReplayBlobs(values []cognitionreplay.Blob) ([]cognitionreplay.Blob, error) {
	result := make([]cognitionreplay.Blob, 0, len(values))
	seen := make(map[string]cognitionreplay.Blob, len(values))
	for _, blob := range values {
		if prior, exists := seen[blob.SHA256]; exists {
			if prior.MediaType != blob.MediaType || !bytes.Equal(prior.Data, blob.Data) {
				return nil, fmt.Errorf("replay blob digest has conflicting content")
			}
			continue
		}
		seen[blob.SHA256] = blob
		result = append(result, blob)
	}
	return result, nil
}
