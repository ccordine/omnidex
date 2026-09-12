package worker

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

// Fixed provider text and real PostgreSQL prove execution, intake boundaries,
// and retained-result consumption, not live narrative interpretation quality.
func TestRoleplayCanonInventoryRecordsOnlyNeededCallsAndReplays(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for canon-inventory evidence")
	}
	for _, fixture := range roleplayCanonEvidenceFixtures() {
		for _, empty := range []bool{false, true} {
			name := fixture.input.Source.AttributedPersonaName + "/facts"
			if empty {
				name = fixture.input.Source.AttributedPersonaName + "/empty"
			}
			t.Run(name, func(t *testing.T) {
				pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
				config, err := modelconfig.Freeze(modelconfig.Config{"roleplay_semantic_model": "fixture-canon"})
				if err != nil {
					t.Fatal(err)
				}
				repository := queue.New(pool, config)
				ctx := context.Background()
				job, err := repository.EnqueueCodingJob(ctx, "exercise exact-source canon intake", t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "canon-evidence-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%#v error=%v", claim, err)
				}
				input := fixture.input
				input.Context = assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
					Sources: []assemblyline.ObjectiveContextSource{{Namespace: "fictional_canon", CandidateID: "CTX_1"}},
					Content: fixture.continuity,
				}}}
				responses := []string{
					strings.Join([]string{fixture.first, fixture.first, fixture.unsupported, fixture.equivalent, fixture.second}, "\n"),
					"A", "B", "A", "A", "A", "B",
				}
				kinds := []assemblyline.WorkKind{
					assemblyline.WorkRoleplayCanonFactInventory,
					assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
					assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
					assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
					assemblyline.WorkRoleplayCanonFactCandidateRelation,
					assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
					assemblyline.WorkRoleplayCanonFactCandidateRelation,
				}
				want := []string{fixture.first, fixture.second}
				if empty {
					input.Source.ExactContribution = "What should happen next?"
					responses = []string{assemblyline.RoleplayNoCanonFactCandidates}
					kinds, want = kinds[:1], []string{}
				}
				provider := &exactEvidenceStationClient{}
				for _, response := range responses {
					provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: response})
				}
				for attempt := range 2 {
					// Rebuild runtime-local state to require consumption of the
					// existing database results, not an in-memory result cache.
					service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
					station := portableObjectiveRoleplayCanonStation{runtime: &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}}
					facts, calls, err := extractRoleplayCanonSource(ctx, station, input)
					wantCalls := 0
					if attempt == 0 {
						wantCalls = len(responses)
					}
					if err != nil || calls != wantCalls || provider.calls != len(responses) || !reflect.DeepEqual(facts, want) {
						t.Fatalf("attempt=%d facts=%q calls=%d provider=%d error=%v", attempt, facts, calls, provider.calls, err)
					}
				}
				calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
				if err != nil || len(calls) != len(responses) {
					t.Fatalf("recorded calls=%d error=%v", len(calls), err)
				}
				for index, recorded := range calls {
					assertPortableLeafRecordedCall(t, recorded, provider.prepared[index], kinds[index], responses[index], "fixture-canon")
					assertGroundedCallOmits(t, recorded.ModelInput, "CTX_1", string(kinds[index]), `"schema"`, `"candidates"`)
					if kinds[index] == assemblyline.WorkRoleplayCanonFactCandidateRelation {
						candidate := fixture.equivalent
						if index == 6 {
							candidate = fixture.second
						}
						assertGroundedCallContains(t, recorded.ModelInput, candidate, fixture.first)
						assertGroundedCallOmits(t, recorded.ModelInput, input.Source.ExactContribution, fixture.continuity, fixture.unsupported)
						if input.AntecedentUserTurn != nil {
							assertGroundedCallOmits(t, recorded.ModelInput, input.AntecedentUserTurn.ContributionContext)
						}
					} else {
						assertGroundedCallContains(t, recorded.ModelInput, input.Source.AttributedPersonaName, input.Source.ExactContribution, fixture.continuity)
						if input.AntecedentUserTurn != nil {
							assertGroundedCallContains(t, recorded.ModelInput, input.AntecedentUserTurn.ContributionContext)
						}
						if index > 1 {
							assertGroundedCallOmits(t, recorded.ModelInput, fixture.first)
						}
					}
					t.Logf("%s: %d model-input bytes, exact response %q", recorded.WorkKind, recorded.ModelInputBytes, recorded.Candidate)
				}
			})
		}
	}
}

func TestRoleplayCanonMalformedInventoryRecordsRejectionWithoutRetry(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for canon rejection evidence")
	}
	for _, fixture := range roleplayCanonEvidenceFixtures() {
		t.Run(fixture.input.Source.AttributedPersonaName, func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			config, err := modelconfig.Freeze(modelconfig.Config{"roleplay_semantic_model": "fixture-canon"})
			if err != nil {
				t.Fatal(err)
			}
			repository := queue.New(pool, config)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise malformed canon intake", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "canon-rejection-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v error=%v", claim, err)
			}
			raw := assemblyline.RoleplayNoCanonFactCandidates + "\n" + fixture.first
			provider := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: raw}}}
			for attempt := range 2 {
				service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
				station := portableObjectiveRoleplayCanonStation{runtime: &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}}
				facts, dispatches, err := extractRoleplayCanonSource(ctx, station, fixture.input)
				if err == nil || provider.calls != 1 || dispatches != 1-attempt || facts != nil {
					t.Fatalf("attempt=%d facts=%v dispatches=%d provider=%d error=%v", attempt, facts, dispatches, provider.calls, err)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != 1 {
				t.Fatalf("recorded calls=%d error=%v", len(calls), err)
			}
			work, err := assemblyline.NewRoleplayCanonFactInventoryJob(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := assemblyline.RenderPortableJob(work)
			if err != nil {
				t.Fatal(err)
			}
			request, err := llm.ExactPreparedRequestBytes(provider.prepared[0])
			if err != nil {
				t.Fatal(err)
			}
			generation, err := exactEvidenceSuccessfulGeneration(provider.prepared[0], raw)
			if err != nil {
				t.Fatal(err)
			}
			recorded := calls[0]
			if recorded.WorkKind != string(work.Kind) || recorded.ModelInput != prompt || recorded.ModelInputBytes != len(prompt) ||
				recorded.Model != "fixture-canon" || recorded.RequestedModel != "fixture-canon" || recorded.Candidate != raw ||
				!bytes.Equal(recorded.ProviderRequest, request) || !recorded.RawResponsePresent ||
				!bytes.Equal(recorded.RawResponse, generation.ProviderResponseCapture) || recorded.Outcome == nil ||
				recorded.Outcome.Status != queue.LLMCallRejected || !strings.Contains(recorded.Outcome.ValidationError, "cannot mix") {
				t.Fatal("canon rejection evidence differs from the actual request, response, or validation failure")
			}
		})
	}
}

type roleplayCanonEvidenceFixture struct {
	input                                              assemblyline.RoleplayCanonExtractionInput
	first, second, equivalent, unsupported, continuity string
}

func roleplayCanonEvidenceFixtures() []roleplayCanonEvidenceFixture {
	return []roleplayCanonEvidenceFixture{
		{
			input: roleplayCanonWorkerTestInput(),
			first: "Mara locks the observatory door.", second: "Mara pockets the brass key.",
			equivalent: "The observatory door is locked by Mara.", unsupported: "Mara opens the telescope shutter.",
			continuity: "Mara is alone in the observatory.",
		},
		{
			input: assemblyline.RoleplayCanonExtractionInput{
				Source: assemblyline.RoleplayCanonSource{
					Kind: assemblyline.RoleplayCanonSourceAssistantResponse, AttributedPersonaName: "Ivo",
					ExactContribution: "I add flour to the dough and set the oven to low heat.",
				},
				AntecedentUserTurn: &assemblyline.RoleplayCanonAntecedent{
					PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
					ContributionKind: roleplay.UserContributionDirection, ContributionContext: "Describe Ivo preparing the dough.",
				},
			},
			first: "Ivo adds flour to the dough.", second: "Ivo sets the oven to low heat.",
			equivalent: "Flour is added to the dough by Ivo.", unsupported: "Ivo decorates the cake.",
			continuity: "Ivo is alone in the bakery.",
		},
	}
}
