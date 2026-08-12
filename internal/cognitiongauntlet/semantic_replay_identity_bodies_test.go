package cognitiongauntlet

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticProviderIdentityPagerReconstructsEveryExactBody(t *testing.T) {
	evidence := semanticReplayIdentityFixture(t, queue.MaxCognitionPolicyEvidencePageBytes+7)
	reader := &semanticReplayFakeEvidenceReader{identities: map[string]llm.ProviderIdentityEvidence{
		evidence.Ref.ID: evidence,
	}}
	var supplement semanticReplaySupplement
	got, err := readSemanticProviderIdentity(
		t.Context(), reader,
		cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), evidence.Ref, &supplement,
	)
	if err != nil || got.Ref != evidence.Ref || len(reader.identityRequest) != 11 {
		t.Fatalf("ref=%+v requests=%d err=%v", got.Ref, len(reader.identityRequest), err)
	}
	if !bytes.Equal(got.Operations[2].ResponseCapture, evidence.Operations[2].ResponseCapture) {
		t.Fatal("multi-page provider identity body changed")
	}
	if len(supplement.sidecars) != 11 {
		t.Fatalf("identity sidecars=%d want 11", len(supplement.sidecars))
	}
}

func TestSemanticProviderIdentityPagerRejectsManifestAndBodyDrift(t *testing.T) {
	evidence := semanticReplayIdentityFixture(t, 32)
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	t.Run("manifest ref", func(t *testing.T) {
		reader := &semanticReplayFakeEvidenceReader{
			identities: map[string]llm.ProviderIdentityEvidence{evidence.Ref.ID: evidence},
			manifestMutate: func(value *queue.CognitionProviderIdentityEvidenceManifest) {
				value.Ref.SHA256 = strings.Repeat("f", 64)
			},
		}
		if _, err := readSemanticProviderIdentity(
			t.Context(), reader, episode, evidence.Ref, &semanticReplaySupplement{},
		); err == nil {
			t.Fatal("changed identity manifest ref was accepted")
		}
	})
	t.Run("operation reorder", func(t *testing.T) {
		reader := &semanticReplayFakeEvidenceReader{
			identities: map[string]llm.ProviderIdentityEvidence{evidence.Ref.ID: evidence},
			manifestMutate: func(value *queue.CognitionProviderIdentityEvidenceManifest) {
				value.Operations[0], value.Operations[1] = value.Operations[1], value.Operations[0]
			},
		}
		if _, err := readSemanticProviderIdentity(
			t.Context(), reader, episode, evidence.Ref, &semanticReplaySupplement{},
		); err == nil {
			t.Fatal("reordered identity operations were accepted")
		}
	})
	mutations := map[string]func(*queue.CognitionProviderIdentityEvidenceBodyPage){
		"ref": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.Ref.ID += "-changed"
		},
		"operation": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.OperationIndex++
		},
		"kind": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.Kind = "changed"
		},
		"SHA": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.SHA256 = strings.Repeat("f", 64)
		},
		"bytes": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.TotalBytes++
		},
		"offset": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.Offset++
		},
		"next": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			page.NextOffset--
		},
		"truncate": func(page *queue.CognitionProviderIdentityEvidenceBodyPage) {
			if len(page.Content) > 0 {
				page.Content = page.Content[:len(page.Content)-1]
				page.NextOffset--
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			reader := &semanticReplayFakeEvidenceReader{
				identities:     map[string]llm.ProviderIdentityEvidence{evidence.Ref.ID: evidence},
				identityMutate: mutate,
			}
			if _, err := readSemanticProviderIdentity(
				t.Context(), reader, episode, evidence.Ref, &semanticReplaySupplement{},
			); err == nil {
				t.Fatal("changed provider identity body page was accepted")
			}
		})
	}
}

func TestSemanticProviderIdentityRejectsUnsafeManifestBytesBeforePaging(t *testing.T) {
	evidence := semanticReplayIdentityFixture(t, 32)
	for name, bytesValue := range map[string]int{
		"negative":           -1,
		"over component cap": llm.MaxProviderIdentityComponentBytes + 1,
		"huge":               int(^uint(0) >> 1),
	} {
		t.Run(name, func(t *testing.T) {
			reader := &semanticReplayFakeEvidenceReader{
				identities: map[string]llm.ProviderIdentityEvidence{evidence.Ref.ID: evidence},
				manifestMutate: func(value *queue.CognitionProviderIdentityEvidenceManifest) {
					value.Operations[0].RequestBytes = bytesValue
				},
			}
			if _, err := readSemanticProviderIdentity(
				t.Context(), reader,
				cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), evidence.Ref,
				&semanticReplaySupplement{},
			); err == nil {
				t.Fatal("unsafe identity manifest byte count was accepted")
			}
			if len(reader.identityRequest) != 0 {
				t.Fatal("unsafe identity manifest reached the body pager")
			}
		})
	}
}

func TestSemanticProviderIdentityLargeBodyUsesChunkedAuthority(t *testing.T) {
	evidence := semanticReplayIdentityFixture(t, cognitionreplay.MaxDirectBlobBytes+1)
	reader := &semanticReplayFakeEvidenceReader{identities: map[string]llm.ProviderIdentityEvidence{
		evidence.Ref.ID: evidence,
	}}
	var supplement semanticReplaySupplement
	if _, err := readSemanticProviderIdentity(
		t.Context(), reader,
		cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), evidence.Ref, &supplement,
	); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sidecar := range supplement.sidecars {
		if sidecar.Kind == semanticSidecarProviderIdentityResponse &&
			sidecar.ID == semanticIdentityBodyID(evidence.Ref.ID, 2) {
			found = sidecar.Content.Storage == cognitionreplay.ProjectionContentChunked
		}
	}
	if !found {
		t.Fatal("large provider identity body did not use chunked authority")
	}
}

func semanticReplayIdentityFixture(t *testing.T, tokenizerBytes int) llm.ProviderIdentityEvidence {
	t.Helper()
	evidence, err := llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(`{"version":"1"}`), []byte(`{"models":[]}`), []byte(`{"model":"m"}`),
		bytes.Repeat([]byte("t"), tokenizerBytes), []byte(`{"model":"m","prompt":""}`),
		[]byte(`{"done":true}`), []byte(`{"models":[]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
