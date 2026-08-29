package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/modelref"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/jackc/pgx/v5"
)

const MaxOllamaDownloadPageSize = 50

var ErrOllamaModelDownloadActive = errors.New("Ollama model download is already active")

type OllamaModelDownloadState string

const (
	OllamaModelDownloadQueued    OllamaModelDownloadState = "queued"
	OllamaModelDownloadRunning   OllamaModelDownloadState = "running"
	OllamaModelDownloadCompleted OllamaModelDownloadState = "completed"
	OllamaModelDownloadFailed    OllamaModelDownloadState = "failed"
)

type OllamaModelDownload struct {
	ID             string                   `json:"id"`
	Model          string                   `json:"model"`
	State          OllamaModelDownloadState `json:"state"`
	Status         string                   `json:"status"`
	Digest         string                   `json:"digest"`
	TotalBytes     int64                    `json:"total_bytes"`
	CompletedBytes int64                    `json:"completed_bytes"`
	Error          string                   `json:"error"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	StartedAt      *time.Time               `json:"started_at,omitempty"`
	FinishedAt     *time.Time               `json:"finished_at,omitempty"`
}

type OllamaModelDownloadPage struct {
	Items   []OllamaModelDownload `json:"items"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

func NewOllamaModelDownloadID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Ollama model download identity: %w", err)
	}
	return "omd_" + hex.EncodeToString(raw), nil
}

func (r *Repository) CreateOllamaModelDownload(ctx context.Context, model string) (OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := modelref.ValidateOllamaName(model); err != nil {
		return OllamaModelDownload{}, err
	}
	id, err := NewOllamaModelDownloadID()
	if err != nil {
		return OllamaModelDownload{}, err
	}
	item, err := scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		INSERT INTO ollama_model_downloads (id,model)
		VALUES ($1,$2)
		ON CONFLICT DO NOTHING
		RETURNING id,model,state,status,digest,total_bytes,completed_bytes,error,
		          created_at,updated_at,started_at,finished_at
	`, id, model))
	if !errors.Is(err, pgx.ErrNoRows) {
		return item, err
	}
	active, activeErr := scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		SELECT id,model,state,status,digest,total_bytes,completed_bytes,error,
		       created_at,updated_at,started_at,finished_at
		FROM ollama_model_downloads
		WHERE model=$1 AND state IN ('queued','running')
	`, model))
	if activeErr != nil {
		return OllamaModelDownload{}, fmt.Errorf("resolve active Ollama download after conflict: %w", activeErr)
	}
	return active, fmt.Errorf("%w: %s", ErrOllamaModelDownloadActive, active.ID)
}

func (r *Repository) StartOllamaModelDownload(ctx context.Context, id string) (OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := validateOllamaDownloadID(id); err != nil {
		return OllamaModelDownload{}, err
	}
	return scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		UPDATE ollama_model_downloads
		SET state='running',status='Connecting to Ollama',started_at=COALESCE(started_at,NOW()),updated_at=NOW()
		WHERE id=$1 AND state IN ('queued','running')
		RETURNING id,model,state,status,digest,total_bytes,completed_bytes,error,
		          created_at,updated_at,started_at,finished_at
	`, id))
}

func (r *Repository) RecordOllamaModelDownloadProgress(
	ctx context.Context,
	id string,
	progress ollama.PullProgress,
) (OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := validateOllamaDownloadID(id); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := progress.Validate(); err != nil {
		return OllamaModelDownload{}, err
	}
	if progress.Error != "" {
		return OllamaModelDownload{}, fmt.Errorf("provider error progress must use the failed transition")
	}
	return scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		UPDATE ollama_model_downloads
		SET status=$2,
		    digest=CASE WHEN $2='success' AND $3='' THEN digest ELSE $3 END,
		    total_bytes=CASE WHEN $2='success' AND $4=0 THEN total_bytes ELSE $4 END,
		    completed_bytes=CASE WHEN $2='success' AND $5=0 THEN completed_bytes ELSE $5 END,
		    updated_at=NOW()
		WHERE id=$1 AND state='running'
		RETURNING id,model,state,status,digest,total_bytes,completed_bytes,error,
		          created_at,updated_at,started_at,finished_at
	`, id, progress.Status, progress.Digest, progress.Total, progress.Completed))
}

func (r *Repository) CompleteOllamaModelDownload(ctx context.Context, id string) (OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := validateOllamaDownloadID(id); err != nil {
		return OllamaModelDownload{}, err
	}
	return scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		UPDATE ollama_model_downloads
		SET state='completed',status='Installed',finished_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND state='running'
		RETURNING id,model,state,status,digest,total_bytes,completed_bytes,error,
		          created_at,updated_at,started_at,finished_at
	`, id))
}

func (r *Repository) FailOllamaModelDownload(ctx context.Context, id, reason string) (OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownload{}, err
	}
	if err := validateOllamaDownloadID(id); err != nil {
		return OllamaModelDownload{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2048 || !utf8.ValidString(reason) || strings.ContainsRune(reason, '\x00') {
		return OllamaModelDownload{}, fmt.Errorf("Ollama model download failure must be bounded canonical text")
	}
	return scanOllamaModelDownload(r.pool.QueryRow(ctx, `
		UPDATE ollama_model_downloads
		SET state='failed',status='Failed',error=$2,finished_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND state IN ('queued','running')
		RETURNING id,model,state,status,digest,total_bytes,completed_bytes,error,
		          created_at,updated_at,started_at,finished_at
	`, id, reason))
}

func (r *Repository) ListOllamaModelDownloads(ctx context.Context, limit, offset int) (OllamaModelDownloadPage, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return OllamaModelDownloadPage{}, err
	}
	if limit < 1 || limit > MaxOllamaDownloadPageSize || offset < 0 {
		return OllamaModelDownloadPage{}, fmt.Errorf("Ollama download page requires limit 1..%d and nonnegative offset", MaxOllamaDownloadPageSize)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id,model,state,status,digest,total_bytes,completed_bytes,error,
		       created_at,updated_at,started_at,finished_at
		FROM ollama_model_downloads
		ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2
	`, limit+1, offset)
	if err != nil {
		return OllamaModelDownloadPage{}, err
	}
	defer rows.Close()
	items := make([]OllamaModelDownload, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanOllamaModelDownload(rows)
		if scanErr != nil {
			return OllamaModelDownloadPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OllamaModelDownloadPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return OllamaModelDownloadPage{Items: items, Offset: offset, HasMore: hasMore}, nil
}

func (r *Repository) ListActiveOllamaModelDownloads(ctx context.Context) ([]OllamaModelDownload, error) {
	if err := r.validateOllamaDownloadContext(ctx); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id,model,state,status,digest,total_bytes,completed_bytes,error,
		       created_at,updated_at,started_at,finished_at
		FROM ollama_model_downloads
		WHERE state IN ('queued','running') ORDER BY created_at,id LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OllamaModelDownload, 0)
	for rows.Next() {
		item, scanErr := scanOllamaModelDownload(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) validateOllamaDownloadContext(ctx context.Context) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("Ollama model downloads require PostgreSQL and context")
	}
	return nil
}

func validateOllamaDownloadID(id string) error {
	if len(id) != 36 || !strings.HasPrefix(id, "omd_") {
		return fmt.Errorf("Ollama model download identity is invalid")
	}
	if raw, err := hex.DecodeString(id[4:]); err != nil || len(raw) != 16 {
		return fmt.Errorf("Ollama model download identity is invalid")
	}
	return nil
}

func scanOllamaModelDownload(row interface{ Scan(...any) error }) (OllamaModelDownload, error) {
	var item OllamaModelDownload
	err := row.Scan(
		&item.ID, &item.Model, &item.State, &item.Status, &item.Digest,
		&item.TotalBytes, &item.CompletedBytes, &item.Error,
		&item.CreatedAt, &item.UpdatedAt, &item.StartedAt, &item.FinishedAt,
	)
	if err != nil {
		return OllamaModelDownload{}, err
	}
	if err := item.Validate(); err != nil {
		return OllamaModelDownload{}, err
	}
	return item, nil
}
