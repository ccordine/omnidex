package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func assertRepositoryCognitionSealedTrace(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	episodeID cognition.EpisodeID,
	brain cognitionpolicy.BrainRef,
) {
	t.Helper()
	counts := make(map[string]int)
	offset := 0
	for {
		page, err := repository.ReadCognitionSealedTrace(ctx, episodeID, queue.CognitionTracePageRequest{
			Offset: offset, Limit: queue.MaxCognitionTracePageSize,
		})
		if err != nil {
			t.Fatalf("read repository cognition sealed trace: %v", err)
		}
		for _, record := range page.Records {
			counts[record.Kind]++
			if record.Kind == "policy_attempt" &&
				(!strings.Contains(string(record.Payload), brain.Digest) ||
					!strings.Contains(string(record.Payload), brain.SamplingSHA256)) {
				t.Fatal("sealed policy attempt omitted the frozen brain identity")
			}
			if record.Kind == "policy_result" &&
				!strings.Contains(string(record.Payload), `"provider_attestation"`) {
				t.Fatal("sealed policy result omitted fresh provider attestation")
			}
		}
		if page.NextOffset < 0 {
			break
		}
		if page.NextOffset <= offset {
			t.Fatal("sealed repository cognition trace did not advance")
		}
		offset = page.NextOffset
	}
	if counts["policy_attempt"] != 3 || counts["policy_result"] != 3 || counts["action"] != 3 ||
		counts["context_projection"] < 3 || counts["runtime_snapshot"] < 3 ||
		counts["transition"] < 3 || counts["obligation_graph"] == 0 ||
		counts["working_set_snapshot"] == 0 {
		t.Fatalf("sealed repository cognition trace counts=%v", counts)
	}
}
