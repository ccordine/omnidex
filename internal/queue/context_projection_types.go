package queue

import (
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/contextbuilder"
)

const (
	maxContextProjectionPageSize = 100
	maxContextProjectionSelected = 64
	maxContextProjectionRecords  = 4096
)

var (
	ErrInvalidContextProjection  = errors.New("invalid context projection")
	ErrContextProjectionNotFound = errors.New("context projection not found")
	ErrContextProjectionConflict = errors.New("context projection identity conflict")
)

type ContextProjectionAuthority struct {
	JobID      int64
	Generation int64
	StepID     int64
	WorkKind   string
	Mode       ContextProjectionMode
}

type ContextProjectionMode string

const (
	ContextProjectionModeShadow ContextProjectionMode = "shadow"
)

type ContextProjectionRecord struct {
	RecordID   int64
	Authority  ContextProjectionAuthority
	Projection contextbuilder.Projection
	CreatedAt  time.Time
}

type ContextProjectionSummary struct {
	RecordID          int64
	ProjectionID      string
	Authority         ContextProjectionAuthority
	WorkID            string
	SpecName          string
	SpecVersion       string
	SpecSHA256        string
	RendererVersion   string
	WorkingSetID      string
	WorkingSetVersion uint64
	SelectedCount     int
	OmittedCount      int
	RenderedSHA256    string
	RenderedBytes     int
	EstimatedTokens   int
	TokenEstimator    string
	CreatedAt         time.Time
}
