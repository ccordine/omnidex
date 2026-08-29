package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func bindTestGapSemanticUncertainty(t testing.TB, gap *queue.StationGapOpening) {
	t.Helper()
	contract, err := assemblyline.SemanticUncertaintyContractForWorkKind(
		assemblyline.WorkKind(gap.WorkKind),
	)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := contract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	gap.SemanticUncertaintyContract = contract
	gap.SemanticUncertaintyContractSHA256 = digest
}
