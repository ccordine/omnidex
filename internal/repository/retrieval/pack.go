package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

var ErrInsufficientEvidence = errors.New("insufficient repository evidence")

const maxSymbolSearchScan = 50

type Store interface {
	RepositoryAnalysis(context.Context, int64, string) (repositoryfacts.Analysis, error)
	RepositorySnapshot(context.Context, int64, string) (repositoryfacts.Snapshot, error)
	SearchRepositorySymbols(context.Context, int64, string, string, int) ([]repositoryfacts.SymbolMatch, error)
	RepositoryGraphNeighborhood(context.Context, int64, string, []string, int) (repositoryfacts.GraphNeighborhood, error)
}

type Service struct {
	store Store
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("repository retrieval requires durable storage")
	}
	return &Service{store: store}, nil
}

func (service *Service) Build(ctx context.Context, request Request) (EvidencePack, error) {
	if ctx == nil {
		return EvidencePack{}, fmt.Errorf("repository evidence retrieval requires a context")
	}
	if service == nil || service.store == nil {
		return EvidencePack{}, fmt.Errorf("repository retrieval service is unavailable")
	}
	if err := request.validate(); err != nil {
		return EvidencePack{}, err
	}
	analysis, snapshot, err := service.loadAuthority(ctx, request)
	if err != nil {
		return EvidencePack{}, err
	}
	pack, err := newEvidencePack(request, snapshot)
	if err != nil {
		return EvidencePack{}, err
	}
	canonical := symbolsByID(analysis.Symbols)
	switch request.Operation {
	case OperationSemanticExcerpts:
		err = service.buildSemanticExcerpts(ctx, request, analysis, snapshot, canonical, &pack)
	case OperationSymbolDeclaration:
		err = service.buildSymbolDeclaration(ctx, request, snapshot, canonical, &pack)
	case OperationDirectReferences:
		err = service.buildDirectReferences(ctx, request, analysis, snapshot, canonical, &pack)
	default:
		err = request.Operation.Validate()
	}
	if err != nil {
		return EvidencePack{}, err
	}
	if err := FinalizeEvidencePack(&pack); err != nil {
		return EvidencePack{}, err
	}
	return pack, nil
}

func (request Request) validate() error {
	if request.ProjectID < 1 {
		return fmt.Errorf("repository evidence retrieval requires a positive project ID")
	}
	if strings.TrimSpace(request.AnalysisID) == "" {
		return fmt.Errorf("repository evidence retrieval requires one analysis ID")
	}
	if err := request.Operation.Validate(); err != nil {
		return err
	}
	if err := validateRetrievalQuery(request.Query); err != nil {
		return err
	}
	return request.Limits.validate()
}

func (service *Service) loadAuthority(
	ctx context.Context,
	request Request,
) (repositoryfacts.Analysis, repositoryfacts.Snapshot, error) {
	analysis, err := service.store.RepositoryAnalysis(ctx, request.ProjectID, request.AnalysisID)
	if err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if !analysis.Complete {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, fmt.Errorf("repository analysis %q is incomplete", analysis.ID)
	}
	snapshot, err := service.store.RepositorySnapshot(ctx, request.ProjectID, analysis.SnapshotID)
	if err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := analysis.Validate(snapshot); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, fmt.Errorf("repository evidence authority: %w", err)
	}
	return analysis, snapshot, nil
}

func newEvidencePack(request Request, snapshot repositoryfacts.Snapshot) (EvidencePack, error) {
	binding, err := NewQueryBinding(request.Operation, request.Query)
	if err != nil {
		return EvidencePack{}, fmt.Errorf("bind repository retrieval query: %w", err)
	}
	return EvidencePack{
		Schema: EvidencePackSchemaV2, SnapshotID: snapshot.ID, AnalysisID: request.AnalysisID,
		Operation: request.Operation, QueryBinding: binding,
		Symbols: []EvidenceSymbol{}, Relations: []EvidenceRelation{}, SourceOmissions: []SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: request.Limits.MaxPackBytes,
	}, nil
}

func symbolsByID(symbols []repositoryfacts.Symbol) map[string]repositoryfacts.Symbol {
	out := make(map[string]repositoryfacts.Symbol, len(symbols))
	for _, symbol := range symbols {
		out[symbol.ID] = symbol
	}
	return out
}
