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

func TestRoutedWebStationsRequireAllFiveExactStationAuthorities(t *testing.T) {
	ids := []station.ID{}
	stations, err := newRoutedWebStations(func(id station.ID) webresearch.PortableRuntime {
		ids = append(ids, id)
		return webresearch.PortableRuntime{
			Execute: func(context.Context, assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				return assemblyline.PortableResult{}, nil
			},
			Finalize: func(context.Context, assemblyline.PortableJob, assemblyline.PortableResult, error) error {
				return nil
			},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []station.ID{
		station.WebSearchTerms, station.WebRelevance,
		station.WebGroundedSynthesis, station.WebGroundedSynthesisCorrection,
		station.WebClaimEvidenceReview,
	}
	if fmt.Sprint(ids) != fmt.Sprint(want) || stations.terms == nil || stations.relevance == nil ||
		stations.synthesis == nil || stations.correction == nil || stations.review == nil {
		t.Fatalf("ids=%v stations=%+v", ids, stations)
	}
}

func TestRoutedWebEvidenceStationsRequireOnlyTermsAndRelevance(t *testing.T) {
	ids := []station.ID{}
	stations, err := newRoutedWebEvidenceStations(func(id station.ID) webresearch.PortableRuntime {
		ids = append(ids, id)
		return webresearch.PortableRuntime{
			Execute: func(context.Context, assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				return assemblyline.PortableResult{}, nil
			},
			Finalize: func(context.Context, assemblyline.PortableJob, assemblyline.PortableResult, error) error {
				return nil
			},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []station.ID{station.WebSearchTerms, station.WebRelevance}
	if fmt.Sprint(ids) != fmt.Sprint(want) || stations.terms == nil || stations.relevance == nil {
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
		SynthesisCalls: 1, ClaimEvidenceReviewCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Text != "Current evidence supports the result." || answer.ModelCalls != 2 ||
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
		Evidence: []webresearch.Evidence{item}, SynthesisCalls: 1,
		ClaimEvidenceReviewCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Evidence) != 1 || !answer.Evidence[0].Truncated ||
		!strings.HasSuffix(answer.Evidence[0].Capsule.Text, objectiveEvidenceTruncationMarker) {
		t.Fatalf("second projection truncation authority was lost: %#v", answer.Evidence)
	}
}

func TestObjectiveExternalAnswerConsumesOneBoundedCorrectionReviewLedger(t *testing.T) {
	item := objectiveWebEvidenceFixture(t, "https://example.test/corrected", "Corrected", "Version 2 is current.")
	id := item.ID
	rendered := "Version 2 is current. [1]\n\nSources:\n[1] Corrected — " + item.URL
	answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Version 2 is current.", EvidenceIDs: []webresearch.EvidenceID{id}}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: id, CandidateID: item.CandidateID, DocumentID: item.DocumentID,
			URL: item.URL, Title: item.Title, ContentSHA256: item.ContentSHA256,
			ObservedAt: item.ObservedAt, Truncated: item.Truncated,
		}},
		Rendered: rendered, RenderedSHA256: objectiveTestSHA256(rendered), Evidence: []webresearch.Evidence{item},
		SynthesisCalls: 1, SynthesisCorrectionCalls: 1, ClaimEvidenceReviewCalls: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.ModelCalls != 4 {
		t.Fatalf("model calls=%d want synthesis+review+correction+review", answer.ModelCalls)
	}
}

func TestObjectiveExternalAnswerConsumesRecordedZeroDeltaCorrection(t *testing.T) {
	item := objectiveWebEvidenceFixture(t, "https://example.test/unchanged", "Unchanged", "Version 2 is current.")
	rendered := "Version 2 is current. [1]\n\nSources:\n[1] Unchanged — " + item.URL
	answer, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Version 2 is current.", EvidenceIDs: []webresearch.EvidenceID{item.ID}}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: item.ID, CandidateID: item.CandidateID, DocumentID: item.DocumentID,
			URL: item.URL, Title: item.Title, ContentSHA256: item.ContentSHA256,
			ObservedAt: item.ObservedAt, Truncated: item.Truncated,
		}},
		Rendered: rendered, RenderedSHA256: objectiveTestSHA256(rendered), Evidence: []webresearch.Evidence{item},
		SynthesisCalls: 1, SynthesisCorrectionCalls: 1,
		SynthesisCorrectionZeroDeltas: 1, ClaimEvidenceReviewCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.ModelCalls != 3 || answer.Text != "Version 2 is current." {
		t.Fatalf("zero-delta answer=%#v", answer)
	}
}

func TestObjectiveExternalAnswerRejectsUnboundedCorrectionReviewLedger(t *testing.T) {
	for _, result := range []objectiveWebResult{
		{Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim."}}, SynthesisCorrectionCalls: 2, ClaimEvidenceReviewCalls: 3},
		{Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim."}}, SynthesisCorrectionCalls: 1, ClaimEvidenceReviewCalls: 1},
		{Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim."}}, SynthesisCorrectionCalls: 0, ClaimEvidenceReviewCalls: 2},
		{Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim."}}, SynthesisCorrectionZeroDeltas: 1, ClaimEvidenceReviewCalls: 1},
		{Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim."}}, SynthesisCorrectionCalls: 1, SynthesisCorrectionZeroDeltas: 2, ClaimEvidenceReviewCalls: 1},
	} {
		if validWebReviewCallLedger(result) {
			t.Fatalf("invalid correction/review ledger accepted: %#v", result)
		}
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
		Evidence:       []webresearch.Evidence{first, second},
		SynthesisCalls: 1, ClaimEvidenceReviewCalls: 2,
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
		SynthesisCalls: 1, ClaimEvidenceReviewCalls: 1,
	})
	if err == nil {
		t.Fatal("invalid acquired-source digest was accepted as citation authority")
	}
}

func TestObjectiveExternalAnswerRejectsUnboundWebSource(t *testing.T) {
	id := webresearch.EvidenceID("evidence_" + strings.Repeat("a", 64))
	_, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs:     []webresearch.GroundedParagraph{{Text: "Claim.", EvidenceIDs: []webresearch.EvidenceID{id}}},
		Sources:        []webresearch.CitationSource{{EvidenceID: id, URL: "https://example.test", ContentSHA256: strings.Repeat("b", 64)}},
		SynthesisCalls: 1, ClaimEvidenceReviewCalls: 1,
	})
	if err == nil {
		t.Fatal("web source without acquired evidence was accepted")
	}
}

func TestObjectiveExternalAnswerRejectsCompletionWithoutIndependentClaimEvidenceReview(t *testing.T) {
	item := objectiveWebEvidenceFixture(t, "https://example.test/review", "Review", "Exact evidence.")
	rendered := "Claim. [1]\n\nSources:\n[1] Review — " + item.URL
	_, err := objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: true, Status: webresearch.ObjectiveComplete,
		Paragraphs: []webresearch.GroundedParagraph{{Text: "Claim.", EvidenceIDs: []webresearch.EvidenceID{item.ID}}},
		Sources: []webresearch.CitationSource{{
			Number: 1, EvidenceID: item.ID, CandidateID: item.CandidateID, DocumentID: item.DocumentID,
			Title: item.Title, URL: item.URL, ContentSHA256: item.ContentSHA256,
			ObservedAt: item.ObservedAt, Truncated: item.Truncated,
		}},
		Evidence: []webresearch.Evidence{item}, Rendered: rendered,
		RenderedSHA256: objectiveTestSHA256(rendered), SynthesisCalls: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "without code-owned completion") {
		t.Fatalf("missing independent review error=%v", err)
	}
}
