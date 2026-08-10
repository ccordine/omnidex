package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

type CognitionProviderIdentityBodyKind string

const (
	CognitionProviderIdentityRequestBody  CognitionProviderIdentityBodyKind = "request"
	CognitionProviderIdentityResponseBody CognitionProviderIdentityBodyKind = "response"
)

type CognitionProviderIdentityOperationMetadata struct {
	Index             int
	Operation         llm.ProviderIdentityOperation
	Method            string
	Endpoint          string
	RequestDispatched bool
	RequestSHA256     string
	RequestBytes      int
	HTTPStatus        int
	Disposition       llm.ProviderIdentityOperationDisposition
	ResponseComplete  bool
	ContentEncoding   llm.ProviderContentEncodingEvidence
	ResponseSHA256    string
	ResponseBytes     int
}

type CognitionProviderIdentityEvidenceManifest struct {
	Ref        llm.ProviderIdentityEvidenceRef
	Operations []CognitionProviderIdentityOperationMetadata
}

type CognitionProviderIdentityEvidenceBodyPage struct {
	Ref            llm.ProviderIdentityEvidenceRef
	OperationIndex int
	Kind           CognitionProviderIdentityBodyKind
	SHA256         string
	TotalBytes     int
	Offset         int
	NextOffset     int
	Content        []byte
}

func (r *Repository) ReadCognitionProviderIdentityEvidenceManifest(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
) (CognitionProviderIdentityEvidenceManifest, error) {
	if ctx == nil || r == nil || r.pool == nil || episodeID == "" || evidenceID == "" {
		return CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("provider identity manifest request is invalid")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT evidence.ref_json,operations.operation_index,operations.operation,
		       operations.method,operations.endpoint,operations.request_dispatched,
		       operations.request_sha256,operations.request_bytes,operations.http_status,
		       operations.disposition,operations.response_complete,
		       operations.content_encoding_json,
		       operations.response_sha256,operations.response_bytes
		FROM cognition_provider_identity_evidence evidence
		JOIN cognition_provider_identity_evidence_operations operations
		  ON operations.evidence_id=evidence.evidence_id
		WHERE evidence.evidence_id=$2 AND EXISTS (
		    SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
		    JOIN cognition_terminal_seals seals ON seals.episode_id=association.episode_id
		    WHERE association.episode_id=$1 AND association.evidence_id=evidence.evidence_id
		)
		ORDER BY operations.operation_index
	`, episodeID, evidenceID)
	if err != nil {
		return CognitionProviderIdentityEvidenceManifest{}, err
	}
	defer rows.Close()
	var value CognitionProviderIdentityEvidenceManifest
	for rows.Next() {
		var refJSON []byte
		var contentEncodingJSON []byte
		var operation CognitionProviderIdentityOperationMetadata
		if err := rows.Scan(&refJSON, &operation.Index, &operation.Operation,
			&operation.Method, &operation.Endpoint, &operation.RequestDispatched,
			&operation.RequestSHA256, &operation.RequestBytes, &operation.HTTPStatus,
			&operation.Disposition, &operation.ResponseComplete,
			&contentEncodingJSON,
			&operation.ResponseSHA256, &operation.ResponseBytes); err != nil {
			return CognitionProviderIdentityEvidenceManifest{}, err
		}
		var ref llm.ProviderIdentityEvidenceRef
		if err := cognitionDecodeExact(refJSON, &ref); err != nil {
			return CognitionProviderIdentityEvidenceManifest{}, err
		}
		if err := cognitionDecodeExact(contentEncodingJSON, &operation.ContentEncoding); err != nil {
			return CognitionProviderIdentityEvidenceManifest{}, err
		}
		if value.Ref == (llm.ProviderIdentityEvidenceRef{}) {
			value.Ref = ref
		} else if value.Ref != ref {
			return CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("%w: identity manifest ref changed", ErrCognitionConflict)
		}
		value.Operations = append(value.Operations, operation)
	}
	if err := rows.Err(); err != nil {
		return CognitionProviderIdentityEvidenceManifest{}, err
	}
	if value.Ref.Validate() != nil || value.Ref.ID != evidenceID || len(value.Operations) != 5 {
		return CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("%w: sealed provider identity manifest is unavailable", ErrCognitionConflict)
	}
	for index, operation := range value.Operations {
		if operation.Index != index {
			return CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("%w: provider identity manifest has a sequence gap", ErrCognitionConflict)
		}
	}
	return value, nil
}

func (r *Repository) ReadCognitionProviderIdentityEvidenceBody(
	ctx context.Context,
	episodeID cognition.EpisodeID,
	evidenceID string,
	operationIndex int,
	kind CognitionProviderIdentityBodyKind,
	offset int,
	limit int,
) (CognitionProviderIdentityEvidenceBodyPage, error) {
	if ctx == nil || r == nil || r.pool == nil || episodeID == "" || evidenceID == "" ||
		operationIndex < 0 || operationIndex > 4 || offset < 0 ||
		limit < 1 || limit > MaxCognitionPolicyEvidencePageBytes ||
		(kind != CognitionProviderIdentityRequestBody && kind != CognitionProviderIdentityResponseBody) {
		return CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("provider identity body page request is invalid")
	}
	columns := `operations.request_sha256,operations.request_bytes,
		substring(operations.request_body FROM $4+1 FOR $5)`
	if kind == CognitionProviderIdentityResponseBody {
		columns = `operations.response_sha256,operations.response_bytes,
			substring(operations.response_body FROM $4+1 FOR $5)`
	}
	query := `SELECT evidence.ref_json,` + columns + `
		FROM cognition_provider_identity_evidence evidence
		JOIN cognition_provider_identity_evidence_operations operations
		  ON operations.evidence_id=evidence.evidence_id
		WHERE evidence.evidence_id=$2 AND EXISTS (
		    SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
		    JOIN cognition_terminal_seals seals ON seals.episode_id=association.episode_id
		    WHERE association.episode_id=$1 AND association.evidence_id=evidence.evidence_id
		)
		  AND operations.operation_index=$3`
	var refJSON []byte
	var body []byte
	page := CognitionProviderIdentityEvidenceBodyPage{
		OperationIndex: operationIndex, Kind: kind, Offset: offset,
	}
	if err := r.pool.QueryRow(
		ctx, query, episodeID, evidenceID, operationIndex, offset, limit,
	).Scan(&refJSON, &page.SHA256, &page.TotalBytes, &body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("%w: sealed provider identity body is unavailable", ErrCognitionConflict)
		}
		return CognitionProviderIdentityEvidenceBodyPage{}, err
	}
	if err := cognitionDecodeExact(refJSON, &page.Ref); err != nil {
		return CognitionProviderIdentityEvidenceBodyPage{}, err
	}
	if page.Ref.Validate() != nil || page.Ref.ID != evidenceID ||
		offset > page.TotalBytes || len(body) > limit || offset+len(body) > page.TotalBytes {
		return CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("%w: provider identity body metadata changed", ErrCognitionConflict)
	}
	page.Content = append([]byte(nil), body...)
	page.NextOffset = offset + len(body)
	return page, nil
}
