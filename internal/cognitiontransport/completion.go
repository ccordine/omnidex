package cognitiontransport

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func validateCompletionRequest(request cognitionruntime.CompletionRequest) error {
	if err := request.Binding.Validate(); err != nil {
		return err
	}
	if !wireDigest(request.SnapshotSHA256) {
		return fmt.Errorf("%w: snapshot hash is invalid", cognition.ErrInvalidCompletionResult)
	}
	if err := request.Goal.Validate(); err != nil {
		return err
	}
	if err := request.Revision.Validate(); err != nil {
		return err
	}
	if request.Revision.EpisodeID != request.Binding.Episode.ID {
		return fmt.Errorf("%w: completion revision belongs to another episode", cognition.ErrInvalidRevision)
	}
	if err := request.Obligation.Validate(); err != nil {
		return err
	}
	if request.Obligation.Status != cognition.ObligationActive ||
		len(request.EvidenceRefs) > cognition.MaxEvidenceRefs {
		return fmt.Errorf("%w: completion obligation or evidence bounds are invalid", cognition.ErrInvalidCompletionResult)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(request.EvidenceRefs))
	for index, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != request.Binding.Episode.ID ||
			ref.Revision.Number > request.Revision.Number {
			return fmt.Errorf("%w: completion evidence %d is invalid", cognition.ErrInvalidEvidence, index)
		}
		if _, duplicate := available[ref]; duplicate {
			return fmt.Errorf("%w: completion evidence %d is duplicated", cognition.ErrInvalidEvidence, index)
		}
		available[ref] = struct{}{}
	}
	for index, ref := range request.Obligation.SupportingRefs {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf("%w: supporting evidence %d is absent from the packet", cognition.ErrInvalidEvidence, index)
		}
	}
	return nil
}

func wireDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
