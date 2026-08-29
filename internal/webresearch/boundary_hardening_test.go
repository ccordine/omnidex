package webresearch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestMachineRunRejectsNilAndCanceledContextsBeforeAcquisition(t *testing.T) {
	acquisition := &countingAcquisition{}
	machine := newFixtureMachine(t, boundaryObjective(), acquisition,
		&recordingRelevanceStation{}, &recordingSynthesisStation{}, 1_000)
	if _, err := machine.Run(nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Run nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := machine.Run(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run canceled context error=%v", err)
	}
	if acquisition.discoveryCalls != 0 || acquisition.fetchCalls != 0 {
		t.Fatalf("acquisition calls=%d/%d", acquisition.discoveryCalls, acquisition.fetchCalls)
	}
}

func TestAcquisitionReportBoundsFailBeforeKernelClone(t *testing.T) {
	acquisition := &countingAcquisition{discovery: websearch.CandidateReport{
		Query: "deterministic query",
		Candidates: []websearch.Candidate{{
			ID: "candidate_untrusted", URL: "https://public.example/",
			Sources: make([]websearch.CandidateSource, maxAcquisitionProviders+1),
		}},
	}}
	machine := newFixtureMachine(t, boundaryObjective(), acquisition,
		&recordingRelevanceStation{}, &recordingSynthesisStation{}, 1_000)
	_, err := machine.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "discovery report bounds") {
		t.Fatalf("Run error=%v", err)
	}
	if acquisition.fetchCalls != 0 {
		t.Fatalf("fetch calls=%d; oversized discovery crossed the boundary", acquisition.fetchCalls)
	}
}

func boundaryObjective() Objective {
	return Objective{
		ID: "objective_boundary", Question: "What does the evidence establish?",
		InitialQuery: "deterministic query", Status: ObjectivePending,
	}
}

type countingAcquisition struct {
	discovery      websearch.CandidateReport
	discoveryCalls int
	fetchCalls     int
}

func (*countingAcquisition) Limits() websearch.AcquisitionLimits {
	return websearch.AcquisitionLimits{MaxDocuments: 8}
}

func (acquisition *countingAcquisition) Discover(context.Context, websearch.QueryRequest) (websearch.CandidateReport, error) {
	acquisition.discoveryCalls++
	return acquisition.discovery, nil
}

func (acquisition *countingAcquisition) Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error) {
	acquisition.fetchCalls++
	return websearch.DocumentReport{}, nil
}
