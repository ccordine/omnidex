package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/jackc/pgx/v5"
)

const MaxCognitionPolicyEvidencePageBytes = 64 * 1024

type CognitionPolicyEvidencePage struct {
	CallID     string
	EvidenceID string
	SHA256     string
	TotalBytes int
	Offset     int
	NextOffset int
	Content    []byte
}

func (r *Repository) ReadCognitionModelResponseEvidence(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (CognitionPolicyEvidencePage, error) {
	return r.readCognitionPolicyEvidencePage(
		ctx, episodeID, evidenceID, offset, limit, "model_response",
	)
}

func (r *Repository) ReadCognitionProviderGenerationEvidence(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (CognitionPolicyEvidencePage, error) {
	return r.readCognitionPolicyEvidencePage(
		ctx, episodeID, evidenceID, offset, limit, "provider_generation",
	)
}

func (r *Repository) ReadCognitionProviderResponseCapture(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
) (CognitionPolicyEvidencePage, error) {
	return r.readCognitionPolicyEvidencePage(
		ctx, episodeID, evidenceID, offset, limit, "provider_response_capture",
	)
}

func (r *Repository) readCognitionPolicyEvidencePage(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	offset int,
	limit int,
	kind string,
) (CognitionPolicyEvidencePage, error) {
	if ctx == nil || r == nil || r.pool == nil || episodeID == "" || evidenceID == "" ||
		offset < 0 || limit < 1 || limit > MaxCognitionPolicyEvidencePageBytes {
		return CognitionPolicyEvidencePage{}, fmt.Errorf("cognition policy evidence page request is invalid")
	}
	query := `SELECT evidence.call_id,evidence.response_sha256,evidence.response_bytes,
		substring(evidence.content FROM $3+1 FOR $4)
		FROM cognition_policy_response_evidence evidence
		JOIN cognition_terminal_seals seals ON seals.episode_id=evidence.episode_id
		WHERE evidence.episode_id=$1 AND evidence.evidence_id=$2`
	if kind == "provider_generation" {
		query = `SELECT evidence.call_id,evidence.generation_sha256,evidence.generation_bytes,
			substring(convert_to(evidence.generation_json,'UTF8') FROM $3+1 FOR $4)
			FROM cognition_policy_provider_generation_evidence evidence
			JOIN cognition_terminal_seals seals ON seals.episode_id=evidence.episode_id
			WHERE evidence.episode_id=$1 AND evidence.evidence_id=$2`
	}
	if kind == "provider_response_capture" {
		query = `SELECT evidence.call_id,evidence.capture_sha256,evidence.capture_bytes,
			substring(evidence.content FROM $3+1 FOR $4)
			FROM cognition_policy_provider_response_captures evidence
			JOIN cognition_terminal_seals seals ON seals.episode_id=evidence.episode_id
			WHERE evidence.episode_id=$1 AND evidence.evidence_id=$2`
	}
	page := CognitionPolicyEvidencePage{EvidenceID: evidenceID, Offset: offset}
	if err := r.pool.QueryRow(ctx, query, episodeID, evidenceID, offset, limit).Scan(
		&page.CallID, &page.SHA256, &page.TotalBytes, &page.Content,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CognitionPolicyEvidencePage{}, fmt.Errorf("%w: sealed cognition policy evidence is unavailable", ErrCognitionConflict)
		}
		return CognitionPolicyEvidencePage{}, err
	}
	if offset > page.TotalBytes || len(page.Content) > limit || offset+len(page.Content) > page.TotalBytes {
		return CognitionPolicyEvidencePage{}, fmt.Errorf("%w: cognition policy evidence page changed", ErrCognitionConflict)
	}
	page.NextOffset = offset + len(page.Content)
	if kind == "model_response" {
		ref := cognitionpolicy.ModelResponseEvidenceRef{
			Schema: cognitionpolicy.ModelResponseEvidenceSchemaV1,
			ID:     evidenceID, SHA256: page.SHA256, Bytes: page.TotalBytes,
		}
		if err := ref.ValidateFor(page.CallID); err != nil {
			return CognitionPolicyEvidencePage{}, err
		}
	} else if kind == "provider_generation" {
		ref := cognitionpolicy.ProviderGenerationEvidenceRef{
			Schema: cognitionpolicy.ProviderGenerationEvidenceSchemaV1,
			ID:     evidenceID, SHA256: page.SHA256, Bytes: page.TotalBytes,
		}
		if err := ref.ValidateFor(page.CallID); err != nil {
			return CognitionPolicyEvidencePage{}, err
		}
	} else if kind == "provider_response_capture" {
		ref := cognitionpolicy.ProviderResponseCaptureEvidenceRef{
			Schema: cognitionpolicy.ProviderResponseCaptureEvidenceSchemaV1,
			ID:     evidenceID, SHA256: page.SHA256, Bytes: page.TotalBytes,
		}
		if err := ref.ValidateFor(page.CallID); err != nil {
			return CognitionPolicyEvidencePage{}, err
		}
	} else {
		return CognitionPolicyEvidencePage{}, fmt.Errorf("cognition policy evidence kind is invalid")
	}
	return page, nil
}
