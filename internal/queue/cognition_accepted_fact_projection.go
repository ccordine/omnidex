package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
)

// ResolveCognitionAcceptedFactProjection binds one selected RoleFact to its
// exact materialized entry and ordered transition-evidence lineage.
func ResolveCognitionAcceptedFactProjection(
	selected taskstate.Ref,
	sourceRefs []taskstate.Ref,
	member CognitionAcceptedFactMaterializationMember,
) ([]cognition.EvidenceRef, error) {
	mappingErr := member.Fact.Mapping.Validate(member.Command)
	wantURI := "task:ledger/" + string(member.Fact.LedgerID) +
		"/entry/" + string(member.Command.ID)
	if member.Fact.validate() != nil || mappingErr != nil || member.Index < 0 ||
		member.Fact.Mapping.SourceKind != cognitionstate.SourceAcceptedFact ||
		member.Fact.Mapping.LedgerID != member.Fact.LedgerID ||
		member.Fact.Mapping.EntryID != member.Command.ID ||
		member.Fact.Mapping.CommandID != member.Command.CommandID ||
		member.EntryURI != wantURI || member.OutputLedgerVersion != member.Command.ExpectedVersion+1 ||
		member.OutputLedgerStatus != taskstate.LedgerActive {
		return nil, fmt.Errorf("%w: accepted-fact projection member is invalid", ErrCognitionConflict)
	}
	digest := sha256.Sum256([]byte(member.Command.Content))
	wantSelected := taskstate.Ref{
		URI: member.EntryURI, Version: strconv.FormatUint(member.OutputLedgerVersion, 10),
		Hash: hex.EncodeToString(digest[:]), Relation: taskstate.RefSource,
	}
	if err := taskstate.ValidateRef(wantSelected); err != nil || selected != wantSelected ||
		!reflect.DeepEqual(sourceRefs, member.Command.Refs) {
		return nil, fmt.Errorf("%w: selected fact differs from its exact materialization", ErrCognitionConflict)
	}
	evidence := append([]cognition.EvidenceRef(nil), member.Fact.EvidenceRefs...)
	if !reflect.DeepEqual(cognitionEvidenceTaskRefs(evidence), sourceRefs) {
		return nil, fmt.Errorf("%w: selected fact lineage differs from materialization evidence", ErrCognitionConflict)
	}
	return evidence, nil
}
