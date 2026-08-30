package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestRoutedWebStationsRequireOnlyRelevanceAndGroundedSynthesis(t *testing.T) {
	ids := []station.ID{}
	stations, err := newRoutedWebStations(func(id station.ID) webresearch.PortableRuntime {
		ids = append(ids, id)
		return webresearch.PortableRuntime{
			Resolve: func(
				context.Context, assemblyline.PortableJob,
				webresearch.PortableCandidateValidator,
			) (webresearch.SemanticCallReceipt, error) {
				return webresearch.SemanticCallReceipt{Calls: 1}, nil
			},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []station.ID{
		station.WebRelevance,
		station.WebGroundedSynthesis,
	}
	if fmt.Sprint(ids) != fmt.Sprint(want) || stations.relevance == nil ||
		stations.synthesis == nil {
		t.Fatalf("ids=%v stations=%+v", ids, stations)
	}
}

func TestRoutedWebEvidenceStationsRequireOnlyRelevance(t *testing.T) {
	ids := []station.ID{}
	stations, err := newRoutedWebEvidenceStations(func(id station.ID) webresearch.PortableRuntime {
		ids = append(ids, id)
		return webresearch.PortableRuntime{
			Resolve: func(
				context.Context, assemblyline.PortableJob,
				webresearch.PortableCandidateValidator,
			) (webresearch.SemanticCallReceipt, error) {
				return webresearch.SemanticCallReceipt{Calls: 1}, nil
			},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []station.ID{station.WebRelevance}
	if fmt.Sprint(ids) != fmt.Sprint(want) || stations.relevance == nil {
		t.Fatalf("evidence station ids=%v stations=%+v", ids, stations)
	}
}

func TestObjectiveWebWorkflowBoundFitsProductionAcquisition(t *testing.T) {
	config := objectiveWebResearchConfig()
	if config.MaxFetchCandidates != 2 || config.MaxRelevantCandidates > config.MaxFetchCandidates {
		t.Fatalf("objective web bounds=%+v", config)
	}
}

func TestObjectiveExternalAnswerConsumesExactWebCompletionAuthority(t *testing.T) {
	item := objectiveWebEvidenceFixture(t, "https://example.test/current", "Current source", "Exact acquired evidence.")
	id := item.ID
	rendered := "Current evidence supports the result. [1]\n\nSources:\n[1] Current source — " + item.URL
	answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Current evidence supports the result.", EvidenceIDs: []webresearch.EvidenceID{id}}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: id, CandidateID: item.CandidateID, DocumentID: item.DocumentID,
			URL: item.URL, Title: item.Title, ContentSHA256: item.ContentSHA256,
			ObservedAt: item.ObservedAt, Truncated: item.Truncated,
		}},
		Rendered: rendered, RenderedSHA256: objectiveTestSHA256(rendered), Evidence: []webresearch.Evidence{item},
		SemanticCalls: 8, CallLedger: objectiveWebTestCallLedger(t, 8),
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Text != "Current evidence supports the result." || answer.ModelCalls != 8 ||
		len(answer.Evidence) != 1 || len(answer.EvidenceIDs) != 1 ||
		answer.Evidence[0].SourceRef != item.URL ||
		answer.Evidence[0].SourceSHA256 != item.ContentSHA256 {
		t.Fatalf("answer=%#v", answer)
	}
	if answer.Rendered != rendered ||
		answer.RenderedSHA256 == "" {
		t.Fatalf("rendered claim/source binding was lost: %#v", answer)
	}
}

func TestObjectiveExternalAnswerAcceptsRestoredAndMixedWebLedgers(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name     string
		receipts []webresearch.SemanticCallReceipt
		calls    int
	}{
		{
			name:     "fully restored",
			receipts: []webresearch.SemanticCallReceipt{{Reused: true}},
		},
		{
			name: "restored and fresh",
			receipts: []webresearch.SemanticCallReceipt{
				{Reused: true}, {Calls: 1},
			},
			calls: 1,
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			item := objectiveWebEvidenceFixture(
				t, "https://example.test/restored", "Restored source",
				"Exact restored evidence.",
			)
			rendered := "Restored evidence supports the result. [1]\n\nSources:\n[1] Restored source — " + item.URL
			answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
				Complete: true, Status: webresearch.ObjectiveComplete,
				Paragraphs: []webresearch.GroundedParagraph{{
					Text:        "Restored evidence supports the result.",
					EvidenceIDs: []webresearch.EvidenceID{item.ID},
				}},
				Sources: []webresearch.CitationSource{{
					Number: 1, EvidenceID: item.ID, CandidateID: item.CandidateID,
					DocumentID: item.DocumentID, URL: item.URL, Title: item.Title,
					ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
					Truncated: item.Truncated,
				}},
				Evidence: []webresearch.Evidence{item}, Rendered: rendered,
				RenderedSHA256: objectiveTestSHA256(rendered),
				SemanticCalls:  fixture.calls,
				CallLedger: objectiveWebTestReceiptLedger(
					t, fixture.receipts...,
				),
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := runObjectiveExternalAnswer(
				t.Context(),
				turnAuthority{ModelInstruction: "What does the evidence support?"},
				objectiveTurnResult{ModelCalls: 1},
				func(context.Context, turnAuthority) (objectiveExternalAnswer, error) {
					return answer, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if answer.ModelCalls != fixture.calls || !result.Complete ||
				result.ModelCalls != fixture.calls+1 || result.Output != rendered {
				t.Fatalf("answer=%+v result=%+v", answer, result)
			}
		})
	}
}

func TestObjectiveExternalAnswerRejectsAbsentOrContradictoryWebLedger(t *testing.T) {
	t.Parallel()
	item := objectiveWebEvidenceFixture(
		t, "https://example.test/provenance", "Provenance source",
		"Exact provenance evidence.",
	)
	rendered := "Provenance evidence supports the result. [1]\n\nSources:\n[1] Provenance source — " + item.URL
	valid, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{
			Text: "Provenance evidence supports the result.", EvidenceIDs: []webresearch.EvidenceID{item.ID},
		}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: item.ID, CandidateID: item.CandidateID,
			DocumentID: item.DocumentID, URL: item.URL, Title: item.Title,
			ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
		}},
		Evidence: []webresearch.Evidence{item}, Rendered: rendered,
		RenderedSHA256: objectiveTestSHA256(rendered),
		CallLedger: objectiveWebTestReceiptLedger(
			t, webresearch.SemanticCallReceipt{Reused: true},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name   string
		mutate func(*objectiveExternalAnswer)
	}{
		{
			name: "absent ledger",
			mutate: func(answer *objectiveExternalAnswer) {
				answer.WebCallLedger = webresearch.SemanticCallLedger{}
			},
		},
		{
			name: "counter contradicts restored ledger",
			mutate: func(answer *objectiveExternalAnswer) {
				answer.ModelCalls = 1
			},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			answer := valid
			fixture.mutate(&answer)
			result, err := runObjectiveExternalAnswer(
				t.Context(),
				turnAuthority{ModelInstruction: "What does the evidence support?"},
				objectiveTurnResult{},
				func(context.Context, turnAuthority) (objectiveExternalAnswer, error) {
					return answer, nil
				},
			)
			if err == nil || result.Complete {
				t.Fatalf("answer=%+v result=%+v error=%v", answer, result, err)
			}
		})
	}
}

func TestObjectiveExternalAnswerPropagatesSecondProjectionTruncationAuthority(t *testing.T) {
	item := objectiveWebEvidenceFixture(
		t, "https://example.test/large", "Large source",
		strings.Repeat("Exact acquired evidence. ", 180),
	)
	rendered := "The large source supports the result. [1]\n\nSources:\n[1] Large source — " + item.URL
	answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{
			Text:        "The large source supports the result.",
			EvidenceIDs: []webresearch.EvidenceID{item.ID},
		}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: item.ID, CandidateID: item.CandidateID,
			DocumentID: item.DocumentID, URL: item.URL, Title: item.Title,
			ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
			Truncated: false,
		}},
		Rendered: rendered, RenderedSHA256: objectiveTestSHA256(rendered),
		Evidence: []webresearch.Evidence{item}, SemanticCalls: 8,
		CallLedger: objectiveWebTestCallLedger(t, 8),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Evidence) != 1 || !answer.Evidence[0].Truncated ||
		!strings.HasSuffix(answer.Evidence[0].Capsule.Text, objectiveEvidenceTruncationMarker) {
		t.Fatalf("second projection truncation authority was lost: %#v", answer.Evidence)
	}
}

func TestObjectiveExternalAnswerPreservesPerParagraphCitationBinding(t *testing.T) {
	first := objectiveWebEvidenceFixture(t, "https://first.test/", "First", "First evidence.")
	second := objectiveWebEvidenceFixture(t, "https://second.test/", "Second", "Second evidence.")
	firstID, secondID := first.ID, second.ID
	rendered := fmt.Sprintf("First claim. [1]\n\nSecond claim. [2]\n\nSources:\n[1] First — %s\n[2] Second — %s", first.URL, second.URL)
	answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete, Rendered: rendered,
		RenderedSHA256: objectiveTestSHA256(rendered),
		Paragraphs: []webresearch.GroundedParagraph{
			{Text: "First claim.", EvidenceIDs: []webresearch.EvidenceID{firstID}},
			{Text: "Second claim.", EvidenceIDs: []webresearch.EvidenceID{secondID}},
		},
		Sources: []webresearch.CitationSource{
			{Number: 1, EvidenceID: firstID, CandidateID: first.CandidateID, DocumentID: first.DocumentID, Title: first.Title, URL: first.URL, ContentSHA256: first.ContentSHA256, ObservedAt: first.ObservedAt, Truncated: first.Truncated},
			{Number: 2, EvidenceID: secondID, CandidateID: second.CandidateID, DocumentID: second.DocumentID, Title: second.Title, URL: second.URL, ContentSHA256: second.ContentSHA256, ObservedAt: second.ObservedAt, Truncated: second.Truncated},
		},
		Evidence:      []webresearch.Evidence{first, second},
		SemanticCalls: 15, CallLedger: objectiveWebTestCallLedger(t, 15),
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Rendered != rendered || len(answer.Paragraphs) != 2 ||
		answer.Paragraphs[0].EvidenceIDs[0] != firstID || answer.Paragraphs[1].EvidenceIDs[0] != secondID {
		t.Fatalf("paragraph citation binding changed: %#v", answer)
	}
}

func objectiveWebEvidenceFixture(t *testing.T, rawURL, title, content string) webresearch.Evidence {
	t.Helper()
	url, err := websearch.CanonicalizeURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	candidateID, err := websearch.CandidateIDForURL(url)
	if err != nil {
		t.Fatal(err)
	}
	contentDigest := sha256.Sum256([]byte(content))
	contentSHA := hex.EncodeToString(contentDigest[:])
	documentDigest := sha256.Sum256([]byte("web-document.v1\x00" + url + "\x00" + contentSHA))
	documentID := websearch.DocumentID("document_" + hex.EncodeToString(documentDigest[:]))
	evidenceDigest := sha256.Sum256([]byte("web-evidence.v1\x00" + string(documentID)))
	return webresearch.Evidence{
		ID:          webresearch.EvidenceID("evidence_" + hex.EncodeToString(evidenceDigest[:])),
		CandidateID: candidateID, DocumentID: documentID, URL: url,
		Title: title, Snippet: title + " summary", Content: content, ContentSHA256: contentSHA,
		ObservedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
}

func objectiveTestSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func objectiveWebTestCallLedger(t *testing.T, calls int) webresearch.SemanticCallLedger {
	t.Helper()
	var ledger webresearch.SemanticCallLedger
	if err := ledger.Record(
		"test web completion", webresearch.SemanticCallReceipt{Calls: calls}, calls,
	); err != nil {
		t.Fatal(err)
	}
	return ledger
}

func objectiveWebTestReceiptLedger(
	t *testing.T,
	receipts ...webresearch.SemanticCallReceipt,
) webresearch.SemanticCallLedger {
	t.Helper()
	var ledger webresearch.SemanticCallLedger
	for index, receipt := range receipts {
		if err := ledger.Record(
			fmt.Sprintf("test web leaf %d", index+1), receipt, 1,
		); err != nil {
			t.Fatal(err)
		}
	}
	return ledger
}

func TestObjectiveExternalAnswerRejectsInvalidAcquiredSourceDigest(t *testing.T) {
	id := webresearch.EvidenceID("evidence_" + strings.Repeat("a", 64))
	_, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim.", EvidenceIDs: []webresearch.EvidenceID{id}}},
		Sources:    []webresearch.CitationSource{{EvidenceID: id, URL: "https://example.test", ContentSHA256: "not-a-sha256"}},
		Evidence: []webresearch.Evidence{{
			ID: id, URL: "https://example.test", Content: "Exact acquired evidence.",
			ContentSHA256: "not-a-sha256",
		}},
	})
	if err == nil {
		t.Fatal("invalid acquired-source digest was accepted as citation authority")
	}
}

func TestObjectiveExternalAnswerRejectsUnboundWebSource(t *testing.T) {
	id := webresearch.EvidenceID("evidence_" + strings.Repeat("a", 64))
	_, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim.", EvidenceIDs: []webresearch.EvidenceID{id}}},
		Sources:    []webresearch.CitationSource{{EvidenceID: id, URL: "https://example.test", ContentSHA256: strings.Repeat("b", 64)}},
	})
	if err == nil {
		t.Fatal("web source without acquired evidence was accepted")
	}
}
