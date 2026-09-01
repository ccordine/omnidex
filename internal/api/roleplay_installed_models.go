package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/modelref"
	"github.com/gryph/omnidex/internal/ollama"
)

var errRoleplayModelCatalogUnavailable = errors.New("roleplay model catalog is unavailable")

func (s *Server) loadInstalledRoleplayModelNames(ctx context.Context) ([]string, error) {
	if s.ollamaEndpoint() == "" {
		return nil, nil
	}
	listPage := s.installedRoleplayModelPageLoader()
	names := make([]string, 0, ollama.MaxModelPageSize)
	seen := make(map[string]struct{})
	for offset := 0; ; {
		page, err := listPage(ctx, ollama.MaxModelPageSize, offset)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: load installed roleplay models at offset %d: %w",
				errRoleplayModelCatalogUnavailable, offset, err,
			)
		}
		if page.Offset != offset {
			return nil, fmt.Errorf(
				"%w: installed roleplay model page returned offset %d for requested offset %d",
				errRoleplayModelCatalogUnavailable, page.Offset, offset,
			)
		}
		if len(page.Models) > ollama.MaxModelPageSize {
			return nil, fmt.Errorf(
				"%w: installed roleplay model page returned %d models for limit %d",
				errRoleplayModelCatalogUnavailable, len(page.Models), ollama.MaxModelPageSize,
			)
		}
		for _, item := range page.Models {
			name := item.Name
			if err := modelref.ValidateOllamaName(name); err != nil {
				return nil, fmt.Errorf(
					"%w: installed roleplay model at offset %d is invalid: %v",
					errRoleplayModelCatalogUnavailable, offset, err,
				)
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, fmt.Errorf(
					"%w: installed roleplay model %q is duplicated",
					errRoleplayModelCatalogUnavailable, name,
				)
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		if !page.HasMore {
			return names, nil
		}
		nextOffset := page.Offset + len(page.Models)
		if nextOffset <= offset {
			return nil, fmt.Errorf(
				"%w: installed roleplay model pagination did not advance from offset %d",
				errRoleplayModelCatalogUnavailable, offset,
			)
		}
		offset = nextOffset
	}
}

func (s *Server) installedRoleplayModelPageLoader() func(context.Context, int, int) (ollama.ModelPage, error) {
	client := s.ollamaClientWithTimeout(10 * time.Second)
	return client.ListModelPage
}
