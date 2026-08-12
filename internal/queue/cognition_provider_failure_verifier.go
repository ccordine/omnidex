package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
)

// VerifyCognitionProviderProcessFailureTraceIdentity binds the public trace
// record ID to the same provider-process failure authority used by durable
// persistence. It intentionally wraps the one queue-owned derivation.
func VerifyCognitionProviderProcessFailureTraceIdentity(
	recordID string,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
) error {
	if err := bootstrap.Validate(); err != nil {
		return err
	}
	if err := failure.ValidateFor(bootstrap.AttestedBrain); err != nil {
		return err
	}
	authority, err := providerProcessObservationAuthority(failure.Receipt.Actor)
	if err != nil {
		return err
	}
	receiptJSON, err := exactjson.Canonical(failure.Receipt)
	if err != nil {
		return err
	}
	prepared, err := prepareProviderFailureBootstrap(&bootstrap)
	if err != nil {
		return err
	}
	want, _, err := newCognitionProviderFailureAuthority(
		cognitionProviderFailureProcess,
		failure.Receipt.ID,
		failure.Receipt.EpisodeID,
		authority,
		failure.IdentityEvidence.Ref.ID,
		cognitionPayloadSHA(receiptJSON),
		prepared,
	)
	if err != nil {
		return err
	}
	if recordID != want.RecordID {
		return fmt.Errorf("%w: provider process failure trace identity changed", ErrCognitionConflict)
	}
	return nil
}
