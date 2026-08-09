package queue

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestValidateClaimBatchRequiresOneExactCurrentOwner(t *testing.T) {
	valid := []model.ClaimRecord{
		{JobID: 11, StepID: 21, Text: "First claim", NormalizedText: "first claim", Status: "supported", Confidence: 0.75},
		{JobID: 11, StepID: 21, Text: "Second claim", NormalizedText: "second claim", Status: "unsupported", Confidence: 0},
	}
	if err := validateClaimBatch(valid); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}

	cases := map[string][]model.ClaimRecord{
		"missing owner":       {{Text: "claim", NormalizedText: "claim", Status: "supported"}},
		"mixed jobs":          {valid[0], {JobID: 12, StepID: 21, Text: "claim", NormalizedText: "claim", Status: "supported"}},
		"mixed steps":         {valid[0], {JobID: 11, StepID: 22, Text: "claim", NormalizedText: "claim", Status: "supported"}},
		"nonexact text":       {{JobID: 11, StepID: 21, Text: " claim", NormalizedText: "claim", Status: "supported"}},
		"missing normalized":  {{JobID: 11, StepID: 21, Text: "claim", Status: "supported"}},
		"unregistered status": {{JobID: 11, StepID: 21, Text: "claim", NormalizedText: "claim", Status: "maybe"}},
		"invalid confidence":  {{JobID: 11, StepID: 21, Text: "claim", NormalizedText: "claim", Status: "supported", Confidence: math.NaN()}},
	}
	for name, records := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateClaimBatch(records); err == nil {
				t.Fatal("invalid claim batch was accepted")
			}
		})
	}
}

func TestValidateClaimSupportsRejectsMalformedOrDuplicateLinks(t *testing.T) {
	valid := []model.ClaimSupportRecord{
		{ClaimID: 1, EvidenceID: 10, SupportScore: 0.5, Rationale: "Direct evidence."},
		{ClaimID: 1, EvidenceID: 11, SupportScore: 1, Rationale: "Independent evidence."},
	}
	if err := validateClaimSupports(valid); err != nil {
		t.Fatalf("valid supports rejected: %v", err)
	}

	cases := map[string][]model.ClaimSupportRecord{
		"missing claim":      {{EvidenceID: 10, SupportScore: 0.5, Rationale: "evidence"}},
		"missing evidence":   {{ClaimID: 1, SupportScore: 0.5, Rationale: "evidence"}},
		"missing rationale":  {{ClaimID: 1, EvidenceID: 10, SupportScore: 0.5}},
		"nonexact rationale": {{ClaimID: 1, EvidenceID: 10, SupportScore: 0.5, Rationale: " evidence"}},
		"invalid score":      {{ClaimID: 1, EvidenceID: 10, SupportScore: 1.1, Rationale: "evidence"}},
		"duplicate":          {valid[0], valid[0]},
	}
	for name, records := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateClaimSupports(records); err == nil {
				t.Fatal("invalid claim support batch was accepted")
			}
		})
	}
}

func TestClaimWriteImplementationHasNoSkipOrUpsertFallback(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("repository_claim_writes.go"))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(contents)
	for _, forbidden := range []string{"ON CONFLICT", "continue\n"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("claim persistence contains forbidden fallback %q", forbidden)
		}
	}
}
