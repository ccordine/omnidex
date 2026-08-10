package labyrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestExtendedGeneratorSealsEveryCapabilityAndKeepsOraclePrivate(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder, SuiteRogue} {
		suite := suite
		t.Run(string(suite), func(t *testing.T) {
			t.Parallel()
			generated, err := GenerateExtended(ExtendedGeneratorConfig{
				Suite: suite, Seed: 91_000 + uint64(len(suite)),
				GeneratorVersion: ExtendedGeneratorVersionV1,
				GrammarVersion:   ExtendedGrammarVersionV1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := generated.Validate(); err != nil {
				t.Fatal(err)
			}
			publicRaw, err := generated.MarshalPublicJSON()
			if err != nil {
				t.Fatal(err)
			}
			oracleRaw, err := generated.MarshalOracleJSON()
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range [][]byte{
				[]byte(`"witness"`), []byte(`"invalid_rails"`), []byte(`"omission_rails"`),
				[]byte(`"definition_sha256"`),
			} {
				if bytes.Contains(publicRaw, secret) {
					t.Fatalf("public artifact exposed private field %s", secret)
				}
			}
			if !bytes.Contains(oracleRaw, []byte(`"witness"`)) ||
				!bytes.Contains(oracleRaw, []byte(`"invalid_rails"`)) {
				t.Fatal("private oracle omitted its exact execution authority")
			}
			if _, err := json.Marshal(generated); !errors.Is(err, ErrArtifactSeparation) {
				t.Fatalf("aggregate serialization error=%v", err)
			}
		})
	}
}

func TestExtendedPublicAffordancesUseOpaqueSeededIdentitiesAndExactWorldSize(t *testing.T) {
	t.Parallel()
	opaque := regexp.MustCompile(`^entity-[0-9a-f]{20}$`)
	for _, suite := range []Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder, SuiteRogue} {
		generated, err := GenerateExtended(ExtendedGeneratorConfig{
			Suite: suite, Seed: 92_000 + uint64(len(suite)),
			GeneratorVersion: ExtendedGeneratorVersionV1, GrammarVersion: ExtendedGrammarVersionV1,
		})
		if err != nil {
			t.Fatal(err)
		}
		public := generated.PublicArtifact()
		if public.World.Descriptor.Difficulty.WorldSize != len(public.World.Descriptor.Records) ||
			len(public.World.Descriptor.Records) != MinGeneratedWorldSize {
			t.Fatalf("suite=%s declared=%d records=%d", suite,
				public.World.Descriptor.Difficulty.WorldSize, len(public.World.Descriptor.Records))
		}
		for _, entity := range public.World.Entities {
			if !opaque.MatchString(string(entity.ID)) {
				t.Fatalf("suite=%s exposed role-bearing entity ID %q", suite, entity.ID)
			}
		}
		assertNoExtendedAnswerLabels(t, public.World.Descriptor.Goal)
		environment, err := NewEnvironment(
			generated.ExecutionScenario(), cognition.EpisodeRef{ID: cognition.EpisodeID("opaque-" + string(suite))},
			func(_ context.Context, candidate cognition.AttemptRef) error {
				if candidate != extendedTestActor() {
					return cognition.ErrAuthorityDenied
				}
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		started, err := environment.Start(t.Context(), generated.ExecutionScenario().Ref())
		if err != nil {
			t.Fatal(err)
		}
		assertNoExtendedAnswerLabels(t, started.Observations[0].Content)
	}
}

func TestExtendedSeedsVaryPlacementWithoutChangingLatentOperationContract(t *testing.T) {
	t.Parallel()
	var baselineKinds []cognition.ActionKind
	placements := make(map[string]struct{})
	for seed := uint64(1); seed <= 16; seed++ {
		generated, err := GenerateExtended(ExtendedGeneratorConfig{
			Suite: SuiteRogue, Seed: seed,
			GeneratorVersion: ExtendedGeneratorVersionV1, GrammarVersion: ExtendedGrammarVersionV1,
		})
		if err != nil {
			t.Fatal(err)
		}
		kinds := extendedWitnessKinds(generated.PrivateOracle().Witness)
		if seed == 1 {
			baselineKinds = kinds
		} else if !reflect.DeepEqual(kinds, baselineKinds) {
			t.Fatal("seed changed the latent operation contract")
		}
		placements[extendedPlacementSignature(generated.PublicArtifact())] = struct{}{}
	}
	if len(placements) < 2 {
		t.Fatal("seeded worlds changed only labels and never changed record placement")
	}
}

func assertNoExtendedAnswerLabels(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"fragment-a", "fragment-b", "record.contradiction", "route-token", "phase.commit",
		"traverse", "binding", "revision", "ordered-action", "rogue",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("model-visible value exposed evaluator label %q: %s", forbidden, value)
		}
	}
}

func extendedPlacementSignature(public GeneratedScenario) string {
	counts := make(map[EntityID]int)
	for _, record := range public.World.Descriptor.Records {
		counts[record.Location]++
	}
	values := make([]int, 0, len(counts))
	for _, count := range counts {
		values = append(values, count)
	}
	sort.Ints(values)
	return fmt.Sprint(values)
}

func TestExtendedGeneratorIsStablePerSeedAndVariesPublicWorldAcrossSeeds(t *testing.T) {
	t.Parallel()
	config := ExtendedGeneratorConfig{
		Suite: SuiteTraverse, Seed: 71_001,
		GeneratorVersion: ExtendedGeneratorVersionV1, GrammarVersion: ExtendedGrammarVersionV1,
	}
	first, err := GenerateExtended(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateExtended(config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.PublicArtifact(), second.PublicArtifact()) ||
		!reflect.DeepEqual(first.PrivateOracle(), second.PrivateOracle()) {
		t.Fatal("same extended seed was not deterministic")
	}
	config.Seed++
	changed, err := GenerateExtended(config)
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicArtifact().Scenario == changed.PublicArtifact().Scenario {
		t.Fatal("different extended seeds produced the same public world authority")
	}
}

func TestExtendedGeneratorRejectsInitialAndUnregisteredSuites(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteRetrieve, Suite("unknown")} {
		_, err := GenerateExtended(ExtendedGeneratorConfig{
			Suite: suite, Seed: 1, GeneratorVersion: ExtendedGeneratorVersionV1,
			GrammarVersion: ExtendedGrammarVersionV1,
		})
		if !errors.Is(err, ErrInvalidGeneratorConfig) {
			t.Fatalf("suite=%q error=%v", suite, err)
		}
	}
}
