package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

type routedWebStations struct {
	relevance *webresearch.PortableStations
	synthesis *webresearch.PortableStations
}

type routedWebEvidenceStations struct {
	relevance *webresearch.PortableStations
}

type objectiveWebResult struct {
	Complete       bool
	Status         webresearch.ObjectiveStatus
	Paragraphs     []webresearch.GroundedParagraph
	Sources        []webresearch.CitationSource
	Evidence       []webresearch.Evidence
	Rendered       string
	RenderedSHA256 string
	SemanticCalls  int
	CallLedger     webresearch.SemanticCallLedger
}

const (
	maxObjectiveWebRelevantCandidates  = 2
	maxObjectiveWebSynthesisParagraphs = 4
	maxObjectiveWebSemanticCalls       = maxObjectiveWebRelevantCandidates +
		1 + maxObjectiveWebSynthesisParagraphs*(maxObjectiveWebRelevantCandidates+1)
)

func newRoutedWebStations(
	runtimeFor func(station.ID) webresearch.PortableRuntime,
) (routedWebStations, error) {
	evidence, err := newRoutedWebEvidenceStations(runtimeFor)
	if err != nil {
		return routedWebStations{}, err
	}
	result := routedWebStations{relevance: evidence.relevance}
	result.synthesis, err = webresearch.NewPortableStations(runtimeFor(station.WebGroundedSynthesis))
	return result, err
}

func newRoutedWebEvidenceStations(
	runtimeFor func(station.ID) webresearch.PortableRuntime,
) (routedWebEvidenceStations, error) {
	if runtimeFor == nil {
		return routedWebEvidenceStations{}, fmt.Errorf("web evidence portable runtime is unavailable")
	}
	relevance, err := webresearch.NewPortableStations(runtimeFor(station.WebRelevance))
	if err != nil {
		return routedWebEvidenceStations{}, err
	}
	return routedWebEvidenceStations{relevance: relevance}, nil
}

func runtimeWebPortableRuntime(
	runtime *nativeRuntimeV3,
	id station.ID,
) webresearch.PortableRuntime {
	return webresearch.PortableRuntime{
		Resolve: func(
			ctx context.Context,
			job assemblyline.PortableJob,
			validate webresearch.PortableCandidateValidator,
		) (webresearch.SemanticCallReceipt, error) {
			if runtime == nil || runtime.svc == nil {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf("web station %q requires runtime authority", id)
			}
			stationID, err := queue.StationForPortableJob(job)
			if err != nil || stationID != id {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf("web station %q received work for %q", id, stationID)
			}
			if validate == nil {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf("web station %q requires one exact decoder", id)
			}
			_, receipt, err := runObjectiveReusablePortableRawLeafCall(
				ctx,
				runtime,
				"web_"+string(job.Kind),
				job,
				id,
				func() (string, error) {
					return runtime.svc.requiredStationModel(runtime.routing, id)
				},
				func(raw string) (string, error) {
					if err := validate(raw); err != nil {
						return "", err
					}
					return raw, nil
				},
				func(string) error { return nil },
			)
			return webresearch.SemanticCallReceipt{
				Calls: receipt.Calls, Reused: receipt.Reused,
			}, err
		},
	}
}

func objectiveWebResearchConfig() webresearch.Config {
	evidence := objectiveWebEvidenceConfig()
	return webresearch.Config{
		MaxFetchCandidates: evidence.MaxFetchCandidates, MaxProjectionBytes: evidence.MaxProjectionBytes,
		MaxRelevantCandidates:      evidence.MaxRelevantCandidates,
		CandidateSummaryBytes:      evidence.CandidateSummaryBytes,
		MaxSynthesisParagraphs:     maxObjectiveWebSynthesisParagraphs,
		MaxSynthesisParagraphBytes: 2 * 1024,
	}
}

func objectiveWebEvidenceConfig() webresearch.EvidenceConfig {
	return webresearch.EvidenceConfig{
		MaxFetchCandidates: maxObjectiveWebRelevantCandidates,
		MaxProjectionBytes: 8 * 1024, MaxRelevantCandidates: maxObjectiveWebRelevantCandidates,
		CandidateSummaryBytes: 512,
	}
}

func objectiveExternalAnswerFromWeb(result webresearch.Result) (objectiveExternalAnswer, error) {
	return objectiveExternalAnswerFromWebResult(objectiveWebResult{
		Complete: result.Complete, Status: result.Objective.Status,
		Paragraphs: result.Artifact.Paragraphs, Sources: result.Artifact.Sources,
		Evidence: result.Evidence, Rendered: result.Artifact.Rendered,
		RenderedSHA256: result.Artifact.SHA256,
		SemanticCalls:  result.SemanticCalls,
		CallLedger:     result.CallLedger.Clone(),
	})
}

func objectiveExternalAnswerFromWebResult(result objectiveWebResult) (objectiveExternalAnswer, error) {
	if !result.Complete || result.Status != webresearch.ObjectiveComplete ||
		len(result.Paragraphs) == 0 || len(result.Sources) == 0 ||
		result.Rendered == "" || result.Rendered != strings.TrimSpace(result.Rendered) {
		return objectiveExternalAnswer{}, fmt.Errorf("web research returned without code-owned completion")
	}
	if err := webresearch.ValidateCompletionArtifact(webresearch.Artifact{
		Paragraphs: result.Paragraphs, Sources: result.Sources,
		Rendered: result.Rendered, SHA256: result.RenderedSHA256,
	}, result.Evidence); err != nil {
		return objectiveExternalAnswer{}, fmt.Errorf("validate web completion authority: %w", err)
	}
	renderedDigest := sha256.Sum256([]byte(result.Rendered))
	renderedSHA := hex.EncodeToString(renderedDigest[:])
	if result.RenderedSHA256 != renderedSHA {
		return objectiveExternalAnswer{}, fmt.Errorf("web research rendered artifact differs from its exact SHA-256")
	}
	textParts := make([]string, len(result.Paragraphs))
	cited := make(map[webresearch.EvidenceID]struct{})
	for index, paragraph := range result.Paragraphs {
		if strings.TrimSpace(paragraph.Text) == "" {
			return objectiveExternalAnswer{}, fmt.Errorf("web research paragraph %d is blank", index)
		}
		textParts[index] = paragraph.Text
		for _, id := range paragraph.EvidenceIDs {
			cited[id] = struct{}{}
		}
	}
	byID := make(map[webresearch.EvidenceID]webresearch.Evidence, len(result.Evidence))
	for _, item := range result.Evidence {
		byID[item.ID] = item
	}
	evidence := make([]objectiveEvidence, 0, len(result.Sources))
	ids := make([]string, 0, len(result.Sources))
	for _, source := range result.Sources {
		item, exists := byID[source.EvidenceID]
		if !exists || item.ContentSHA256 != source.ContentSHA256 {
			return objectiveExternalAnswer{}, fmt.Errorf("web source %q lost exact acquired evidence", source.EvidenceID)
		}
		if _, used := cited[source.EvidenceID]; !used {
			return objectiveExternalAnswer{}, fmt.Errorf("web source %q was not cited by synthesis", source.EvidenceID)
		}
		capsuleText, capsuleTruncated, err := boundedObjectiveEvidenceText(
			maxObjectiveEvidenceTextBytes, item.Title, item.Snippet, item.Content,
		)
		if err != nil {
			return objectiveExternalAnswer{}, err
		}
		projected, err := newObjectiveEvidence(string(item.ID), capsuleText, "web_document", item.URL)
		if err != nil {
			return objectiveExternalAnswer{}, err
		}
		projected.SourceSHA256 = item.ContentSHA256
		projected.ObservedAt = item.ObservedAt
		projected.Truncated = item.Truncated || capsuleTruncated
		for paragraphIndex, paragraph := range result.Paragraphs {
			for _, paragraphEvidenceID := range paragraph.EvidenceIDs {
				if paragraphEvidenceID == source.EvidenceID {
					projected.ParagraphMask |= 1 << paragraphIndex
					break
				}
			}
		}
		if projected.ParagraphMask == 0 {
			return objectiveExternalAnswer{}, fmt.Errorf("web source %q lost its paragraph binding", source.EvidenceID)
		}
		if err := validateObjectiveEvidence(projected); err != nil {
			return objectiveExternalAnswer{}, err
		}
		evidence = append(evidence, projected)
		ids = append(ids, string(item.ID))
	}
	receipt, err := result.CallLedger.ValidateForMaximum(
		"web research completion", maxObjectiveWebSemanticCalls,
	)
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	if result.SemanticCalls != receipt.Calls {
		return objectiveExternalAnswer{}, fmt.Errorf(
			"web research completion reported %d calls but its exact ledger proves %d",
			result.SemanticCalls, receipt.Calls,
		)
	}
	return objectiveExternalAnswer{
		Text: strings.Join(textParts, "\n\n"), Rendered: result.Rendered,
		RenderedSHA256: renderedSHA, Paragraphs: cloneWebParagraphs(result.Paragraphs),
		Evidence: evidence, EvidenceIDs: ids, ModelCalls: receipt.Calls,
		WebCallLedger: result.CallLedger.Clone(),
	}, nil
}

func cloneWebParagraphs(items []webresearch.GroundedParagraph) []webresearch.GroundedParagraph {
	cloned := make([]webresearch.GroundedParagraph, len(items))
	for index, item := range items {
		cloned[index] = item
		cloned[index].EvidenceIDs = append([]webresearch.EvidenceID(nil), item.EvidenceIDs...)
	}
	return cloned
}
