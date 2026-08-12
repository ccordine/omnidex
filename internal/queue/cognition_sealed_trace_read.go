package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadCognitionSealedTrace(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	request CognitionTracePageRequest,
) (CognitionSealedTracePage, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return CognitionSealedTracePage{}, fmt.Errorf("sealed cognition trace read requires PostgreSQL and context")
	}
	if err := cognitionEpisodeIdentityValid(episodeID); err != nil {
		return CognitionSealedTracePage{}, err
	}
	if err := validateCognitionTracePageRequest(request); err != nil {
		return CognitionSealedTracePage{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireCognitionTraceSchemaAuthorityTx(ctx, tx); err != nil {
		return CognitionSealedTracePage{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, false)
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	if !found {
		return CognitionSealedTracePage{}, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, episodeID)
	}
	if episode.Status == CognitionEpisodeActive {
		return CognitionSealedTracePage{}, fmt.Errorf("%w: cognition episode is not sealed", ErrCognitionConflict)
	}
	seal, err := loadCognitionTerminalSealTx(ctx, tx, episodeID)
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	trace, raw, err := loadVerifiedCognitionTraceAuthorityTx(ctx, tx, episode, seal)
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episodeID, false)
	if err != nil || !found {
		return CognitionSealedTracePage{}, fmt.Errorf("%w: sealed cognition graph: %v", ErrCognitionConflict, err)
	}
	rebuilt, rebuiltSHA, err := buildCognitionTraceAuthorityTx(
		ctx, tx, episode, graph, seal.LedgerVersion, seal.WorkingSetVersion,
	)
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	if rebuiltSHA != seal.TraceSHA256 || !bytes.Equal(rebuilt, raw) {
		return CognitionSealedTracePage{}, fmt.Errorf("%w: sealed cognition trace no longer matches durable records", ErrCognitionConflict)
	}
	page, err := loadCognitionTracePageTx(ctx, tx, episode, seal, trace, request)
	if err != nil {
		return CognitionSealedTracePage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionSealedTracePage{}, err
	}
	return page, nil
}

func loadVerifiedCognitionTraceAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	seal CognitionTerminalSeal,
) (cognitionTraceAuthority, []byte, error) {
	var raw []byte
	var persistedSHA string
	if err := tx.QueryRow(ctx, `
		SELECT trace_json,trace_sha256 FROM cognition_terminal_seals WHERE episode_id=$1
	`, episode.EpisodeID).Scan(&raw, &persistedSHA); err != nil {
		return cognitionTraceAuthority{}, nil, err
	}
	digest := sha256.Sum256(raw)
	if persistedSHA != hex.EncodeToString(digest[:]) || persistedSHA != seal.TraceSHA256 {
		return cognitionTraceAuthority{}, nil, fmt.Errorf("%w: sealed cognition trace hash changed", ErrCognitionConflict)
	}
	var trace cognitionTraceAuthority
	if err := json.Unmarshal(raw, &trace); err != nil {
		return cognitionTraceAuthority{}, nil, fmt.Errorf("decode sealed cognition trace: %w", err)
	}
	if err := trace.validate(); err != nil {
		return cognitionTraceAuthority{}, nil, err
	}
	if trace.EpisodeID != episode.EpisodeID || trace.Revision != seal.FinalRevision ||
		trace.GraphVersion == 0 || trace.GraphSHA256 != seal.ObligationGraphSHA256 ||
		trace.LedgerVersion != seal.LedgerVersion || trace.WorkingVersion != seal.WorkingSetVersion {
		return cognitionTraceAuthority{}, nil, fmt.Errorf("%w: sealed cognition trace header changed", ErrCognitionConflict)
	}
	return trace, append([]byte(nil), raw...), nil
}

func loadCognitionTracePageTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	seal CognitionTerminalSeal,
	trace cognitionTraceAuthority,
	request CognitionTracePageRequest,
) (CognitionSealedTracePage, error) {
	if request.Offset > len(trace.Records) {
		return CognitionSealedTracePage{}, fmt.Errorf("cognition trace offset %d exceeds %d records", request.Offset, len(trace.Records))
	}
	records := make([]CognitionSealedTraceRecord, 0, request.Limit)
	payloadBytes := 0
	next := request.Offset
	for next < len(trace.Records) && len(records) < request.Limit {
		authority := trace.Records[next]
		payload, err := loadCognitionTracePayloadTx(
			ctx, tx, episode, trace.WorkingVersion, authority,
		)
		if err != nil {
			return CognitionSealedTracePage{}, err
		}
		if len(payload) > MaxCognitionTracePayloadBytes {
			return CognitionSealedTracePage{}, fmt.Errorf("%w: cognition trace payload exceeds the hard cap", ErrCognitionConflict)
		}
		if cognitionPayloadSHA(payload) != authority.SHA256 {
			return CognitionSealedTracePage{}, fmt.Errorf(
				"%w: cognition trace payload hash differs from sealed authority", ErrCognitionConflict,
			)
		}
		if len(records) > 0 && payloadBytes+len(payload) > MaxCognitionTracePageBytes {
			break
		}
		payloadBytes += len(payload)
		records = append(records, CognitionSealedTraceRecord{
			Kind: authority.Kind, CallOrdinal: authority.CallOrdinal,
			Phase: authority.Phase, Sequence: authority.Sequence, ID: authority.ID,
			SHA256: authority.SHA256, Payload: append(json.RawMessage(nil), payload...),
		})
		next++
	}
	if next == len(trace.Records) {
		next = -1
	}
	return CognitionSealedTracePage{
		Schema: CognitionSealedTraceSchemaV2, EpisodeID: episode.EpisodeID,
		TraceSHA256: seal.TraceSHA256, Seal: seal, GraphVersion: trace.GraphVersion,
		GraphSHA256: trace.GraphSHA256, LedgerVersion: trace.LedgerVersion,
		WorkingSetVersion: trace.WorkingVersion, EpisodeStartedAt: episode.CreatedAt,
		SealedAt: seal.CreatedAt, TotalRecords: len(trace.Records),
		Offset: request.Offset, NextOffset: next, Records: records,
	}, nil
}
