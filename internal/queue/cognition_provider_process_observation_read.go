package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadCognitionProviderProcessObservationPage(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	request CognitionProviderProcessObservationPageRequest,
) (CognitionProviderProcessObservationPage, error) {
	if r == nil || r.pool == nil || ctx == nil || episodeID == "" || request.validate() != nil {
		return CognitionProviderProcessObservationPage{},
			fmt.Errorf("provider process observation page requires PostgreSQL, episode, and bounded request")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return CognitionProviderProcessObservationPage{}, err
	}
	defer tx.Rollback(ctx)
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, false)
	if err != nil || !found {
		return CognitionProviderProcessObservationPage{},
			fmt.Errorf("load provider process episode %q: %w", episodeID, err)
	}
	if request.Scope == CognitionProviderObservationPostSealAudit &&
		episode.Status == CognitionEpisodeActive {
		return CognitionProviderProcessObservationPage{},
			fmt.Errorf("%w: post-seal provider observation page requires a terminal episode", ErrCognitionConflict)
	}
	page := CognitionProviderProcessObservationPage{
		EpisodeBrain: episode.AttestedBrain, Scope: request.Scope,
		NextSequence: request.AfterSequence,
	}
	if episode.Status != CognitionEpisodeActive {
		seal, err := loadCognitionTerminalSealTx(ctx, tx, episodeID)
		if err != nil {
			return CognitionProviderProcessObservationPage{}, err
		}
		page.TerminalTraceSHA256 = seal.TraceSHA256
	}
	if err := loadProviderProcessPageAuthorityTx(ctx, tx, episodeID, request, &page); err != nil {
		return CognitionProviderProcessObservationPage{}, err
	}
	page.Records, err = readProviderProcessRecordsPageTx(ctx, tx, episode, request)
	if err != nil {
		return CognitionProviderProcessObservationPage{}, err
	}
	if err := validateProviderProcessObservationPage(request, page); err != nil {
		return CognitionProviderProcessObservationPage{}, err
	}
	if len(page.Records) > 0 {
		page.NextSequence = page.Records[len(page.Records)-1].Sequence
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionProviderProcessObservationPage{}, err
	}
	return page, nil
}

func loadProviderProcessPageAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	request CognitionProviderProcessObservationPageRequest,
	page *CognitionProviderProcessObservationPage,
) error {
	if request.Scope == CognitionProviderObservationTerminalTrace {
		return tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1`,
			episodeID,
		).Scan(&page.TotalRecords)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE((
		SELECT chain_sha256 FROM cognition_provider_postseal_observations
		WHERE episode_id=$1 ORDER BY sequence DESC LIMIT 1
	),'') FROM cognition_provider_postseal_observations WHERE episode_id=$1`, episodeID).Scan(
		&page.TotalRecords, &page.PostSealAuditHeadSHA256,
	); err != nil {
		return err
	}
	if request.AfterSequence == 0 {
		page.PreviousChainSHA256 = page.TerminalTraceSHA256
		return nil
	}
	return tx.QueryRow(ctx, `SELECT chain_sha256 FROM cognition_provider_postseal_observations
		WHERE episode_id=$1 AND sequence=$2`, episodeID, request.AfterSequence).Scan(
		&page.PreviousChainSHA256,
	)
}

func readProviderProcessRecordsPageTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	request CognitionProviderProcessObservationPageRequest,
) ([]CognitionProviderProcessObservationRecord, error) {
	query := `SELECT sequence,'','','','',receipt_json,receipt_sha256,evidence_id
		FROM cognition_provider_process_observations
		WHERE episode_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`
	if request.Scope == CognitionProviderObservationPostSealAudit {
		query = `SELECT sequence,terminal_trace_sha256,previous_chain_sha256,chain_sha256,source_kind,
			receipt_json,receipt_sha256,evidence_id FROM cognition_provider_postseal_observations
			WHERE episode_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`
	}
	rows, err := tx.Query(ctx, query, episode.EpisodeID, request.AfterSequence, request.Limit)
	if err != nil {
		return nil, err
	}
	type persistedRecord struct {
		record     CognitionProviderProcessObservationRecord
		raw        []byte
		evidenceID string
	}
	persisted := make([]persistedRecord, 0, request.Limit)
	for rows.Next() {
		value := persistedRecord{
			record: CognitionProviderProcessObservationRecord{Scope: request.Scope},
		}
		if err := rows.Scan(
			&value.record.Sequence, &value.record.TerminalTraceSHA256,
			&value.record.PreviousChainSHA256, &value.record.ChainSHA256,
			&value.record.PostSealSource,
			&value.raw, &value.record.ReceiptSHA256, &value.evidenceID,
		); err != nil {
			rows.Close()
			return nil, err
		}
		persisted = append(persisted, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	records := make([]CognitionProviderProcessObservationRecord, 0, len(persisted))
	for _, value := range persisted {
		record := value.record
		if err := exactjson.ValidateObject(value.raw, cognitionpolicy.ProviderProcessObservation{},
			"provider process observation"); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCognitionConflict, err)
		}
		if err := json.Unmarshal(value.raw, &record.Activation.Receipt); err != nil {
			return nil, err
		}
		record.Activation.IdentityEvidence, err = loadCognitionProviderIdentityEvidenceTx(
			ctx, tx, value.evidenceID,
		)
		if err != nil {
			return nil, err
		}
		canonical, err := exactjson.Canonical(record.Activation.Receipt)
		if err != nil || string(canonical) != string(value.raw) ||
			cognitionPayloadSHA(canonical) != record.ReceiptSHA256 ||
			record.validate(episode.AttestedBrain) != nil {
			return nil, fmt.Errorf("%w: provider process receipt changed", ErrCognitionConflict)
		}
		records = append(records, record)
	}
	return records, nil
}

func validateProviderProcessObservationPage(
	request CognitionProviderProcessObservationPageRequest,
	page CognitionProviderProcessObservationPage,
) error {
	if request.AfterSequence > page.TotalRecords || len(page.Records) > request.Limit {
		return fmt.Errorf("%w: provider process observation page bounds changed", ErrCognitionConflict)
	}
	previous := page.PreviousChainSHA256
	for index, record := range page.Records {
		if record.Sequence != request.AfterSequence+int64(index)+1 {
			return fmt.Errorf("%w: provider process observation sequence changed", ErrCognitionConflict)
		}
		if request.Scope == CognitionProviderObservationPostSealAudit {
			if page.TerminalTraceSHA256 == "" ||
				record.TerminalTraceSHA256 != page.TerminalTraceSHA256 ||
				record.PreviousChainSHA256 != previous ||
				record.ChainSHA256 != providerPostSealChainSHA(
					page.TerminalTraceSHA256, previous, record.Sequence,
					record.PostSealSource, record.ReceiptSHA256,
				) {
				return fmt.Errorf("%w: post-seal provider observation chain changed", ErrCognitionConflict)
			}
			previous = record.ChainSHA256
		}
	}
	return nil
}
