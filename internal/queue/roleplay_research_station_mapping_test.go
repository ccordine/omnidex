package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRoleplayGroundedResponseUsesTheBoundedResponseStation(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRoleplayGroundedResponseParagraphInventory,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization,
	} {
		got, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		if got != station.ConversationResponse {
			t.Fatalf("kind=%q station=%q want %q", kind, got, station.ConversationResponse)
		}
	}
}
