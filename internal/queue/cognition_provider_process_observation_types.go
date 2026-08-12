package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

type CognitionProviderObservationScope string

type CognitionProviderPostSealSource string

const (
	CognitionProviderObservationTerminalTrace CognitionProviderObservationScope = "terminal_trace"
	CognitionProviderObservationPostSealAudit CognitionProviderObservationScope = "post_seal_audit"

	CognitionProviderPostSealDirectAudit   CognitionProviderPostSealSource = "direct_audit"
	CognitionProviderPostSealEpisodeReplay CognitionProviderPostSealSource = "episode_replay"
)

type CognitionProviderProcessObservationRecord struct {
	Scope               CognitionProviderObservationScope
	Sequence            int64
	TerminalTraceSHA256 string
	PreviousChainSHA256 string
	ChainSHA256         string
	ReceiptSHA256       string
	PostSealSource      CognitionProviderPostSealSource
	Activation          cognitionpolicy.ProviderProcessActivation
}

const MaxCognitionProviderObservationPageSize = 32

type CognitionProviderProcessObservationPageRequest struct {
	Scope         CognitionProviderObservationScope
	AfterSequence int64
	Limit         int
}

type CognitionProviderProcessObservationPage struct {
	EpisodeBrain            cognitionpolicy.AttestedBrain
	Scope                   CognitionProviderObservationScope
	TerminalTraceSHA256     string
	TotalRecords            int64
	PreviousChainSHA256     string
	PostSealAuditHeadSHA256 string
	NextSequence            int64
	Records                 []CognitionProviderProcessObservationRecord
}

func (request CognitionProviderProcessObservationPageRequest) validate() error {
	if request.Scope != CognitionProviderObservationTerminalTrace &&
		request.Scope != CognitionProviderObservationPostSealAudit ||
		request.AfterSequence < 0 || request.Limit < 1 ||
		request.Limit > MaxCognitionProviderObservationPageSize {
		return fmt.Errorf("provider process observation page request is invalid")
	}
	return nil
}

func (record CognitionProviderProcessObservationRecord) validate(
	brain cognitionpolicy.AttestedBrain,
) error {
	if record.Sequence < 1 || record.ReceiptSHA256 == "" ||
		record.Activation.ValidateFor(brain) != nil {
		return fmt.Errorf("%w: provider process observation record is invalid", ErrCognitionConflict)
	}
	switch record.Scope {
	case CognitionProviderObservationTerminalTrace:
		if record.TerminalTraceSHA256 != "" || record.PreviousChainSHA256 != "" ||
			record.ChainSHA256 != "" || record.PostSealSource != "" {
			return fmt.Errorf("%w: pre-terminal provider observation claims post-seal authority", ErrCognitionConflict)
		}
	case CognitionProviderObservationPostSealAudit:
		if !cognitionDigestPattern.MatchString(record.TerminalTraceSHA256) ||
			!cognitionDigestPattern.MatchString(record.PreviousChainSHA256) ||
			!cognitionDigestPattern.MatchString(record.ChainSHA256) ||
			!validCognitionProviderPostSealSource(record.PostSealSource) {
			return fmt.Errorf("%w: post-seal provider observation chain is invalid", ErrCognitionConflict)
		}
	default:
		return fmt.Errorf("%w: provider observation scope is not registered", ErrCognitionConflict)
	}
	return nil
}

func validCognitionProviderPostSealSource(source CognitionProviderPostSealSource) bool {
	return source == CognitionProviderPostSealDirectAudit ||
		source == CognitionProviderPostSealEpisodeReplay
}
