package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/queue"
)

type rejectedExactStationProvider struct {
	startupTestLLM
}

type pointerWorkerTransport struct{}

func (*pointerWorkerTransport) RequireExactPreparedContract() error { return nil }

func (*pointerWorkerTransport) ValidateExactPreparedProvider(
	llm.ProviderIdentityExpectation,
) error {
	return nil
}

func (*pointerWorkerTransport) ValidateExactPreparedContract(llm.PreparedModel) error {
	return nil
}

func (*pointerWorkerTransport) GeneratePreparedExact(
	context.Context,
	llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	return llm.PreparedGeneration{}, nil
}

func (*pointerWorkerTransport) DiscoverProviderIdentityEvidence(
	context.Context,
	llm.ProviderIdentitySelection,
	string,
) (llm.ObservedProviderIdentity, error) {
	return llm.ObservedProviderIdentity{}, nil
}

func (*pointerWorkerTransport) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (rejectedExactStationProvider) RequireExactPreparedContract() error {
	return errors.New("exact contract unavailable")
}

func TestNewRequiresExactStationsAtConstruction(t *testing.T) {
	service, err := New(
		&queue.Repository{}, nil, startupTestLLM{}, nil, validWorkerOptions(),
	)
	if err == nil || !strings.Contains(err.Error(), "exact station client is required") {
		t.Fatalf("worker construction service=%v error=%v, want exact station failure", service, err)
	}
}

func TestNewRequiresEmbeddingsAtConstruction(t *testing.T) {
	service, err := New(
		&queue.Repository{}, startupTestLLM{}, nil, nil, validWorkerOptions(),
	)
	if err == nil || !strings.Contains(err.Error(), "embedding client is required") {
		t.Fatalf("worker construction service=%v error=%v, want embedding failure", service, err)
	}
}

func TestNewDefersExactStationContractValidation(t *testing.T) {
	service, err := New(
		&queue.Repository{}, rejectedExactStationProvider{}, startupTestLLM{}, nil, validWorkerOptions(),
	)
	if err != nil {
		t.Fatalf("worker construction eagerly validated dormant exact station contract: %v", err)
	}
	if service.stationClient == nil {
		t.Fatal("worker lost deferred exact station transport")
	}
}

func TestNewAcceptsLazyAbsentProviderAuthority(t *testing.T) {
	transports := llmprovider.NewLazyFromConfig(config.Config{
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	opts := validWorkerOptions()
	opts.EmbeddingProvider = ""
	opts.EmbeddingModel = ""
	service, err := New(
		&queue.Repository{}, transports.Stations, transports.Embeddings, nil, opts,
	)
	if err != nil {
		t.Fatalf("worker startup resolved absent provider authority: %v", err)
	}
	if service.stationClient == nil || service.embeddings == nil {
		t.Fatalf("worker lost lazy provider resolvers: %+v", service)
	}
}

func TestNewRetainsExplicitObjectiveAdvisoryMode(t *testing.T) {
	opts := validWorkerOptions()
	opts.ObjectiveAdvisoryMode = "shadow"
	service, err := New(
		&queue.Repository{}, startupTestLLM{}, startupTestLLM{}, nil, opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.objectiveAdvisoryMode != "shadow" {
		t.Fatalf("service advisory mode=%q want shadow", service.objectiveAdvisoryMode)
	}
	if service.objectiveAdvisoryProvider != llm.ExactPreparedProviderBackend {
		t.Fatalf("service advisory provider=%q want exact backend", service.objectiveAdvisoryProvider)
	}
}

func TestNewRejectsTypedNilTransportsAtConstruction(t *testing.T) {
	var missing *pointerWorkerTransport
	for _, test := range []struct {
		name       string
		stations   llm.ExactStationClient
		embeddings llm.EmbeddingClient
		want       string
	}{
		{name: "stations", stations: missing, embeddings: startupTestLLM{}, want: "exact station client is required"},
		{name: "embeddings", stations: startupTestLLM{}, embeddings: missing, want: "embedding client is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := New(
				&queue.Repository{}, test.stations, test.embeddings, nil, validWorkerOptions(),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("worker construction service=%v error=%v want %q", service, err, test.want)
			}
		})
	}
}
