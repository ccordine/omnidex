package worker

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/specialists"
)

const activeSkillRegistryPageSize = 200

func (s *Service) refreshSkillRegistry(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("worker skill registry requires a PostgreSQL repository")
	}
	bootstrap, err := bootstrapSkillSpecs(s.bootstrapRegistry)
	if err != nil {
		return err
	}
	if _, err := s.repo.SyncBootstrapSkills(ctx, bootstrap); err != nil {
		return fmt.Errorf("synchronize bootstrap skills: %w", err)
	}
	registry := &specialists.Registry{Specs: map[string]specialists.Spec{}}
	for offset := 0; ; offset += activeSkillRegistryPageSize {
		versions, err := s.repo.ListActiveSkills(ctx, activeSkillRegistryPageSize, offset)
		if err != nil {
			return fmt.Errorf("load active worker skills at offset %d: %w", offset, err)
		}
		for _, version := range versions {
			if version.Status != specialists.SkillStatusActive {
				return fmt.Errorf("active worker skill query returned %s status for %s", version.Status, version.Spec.ID)
			}
			if _, duplicate := registry.Specs[version.Spec.ID]; duplicate {
				return fmt.Errorf("active worker skill registry repeats %q", version.Spec.ID)
			}
			registry.Specs[version.Spec.ID] = version.Spec
		}
		if len(versions) < activeSkillRegistryPageSize {
			break
		}
	}
	if len(registry.Specs) == 0 {
		return fmt.Errorf("authoritative worker skill registry contains no active skills")
	}
	s.skillMu.Lock()
	s.v3Registry = registry
	s.skillMu.Unlock()
	s.logger.Printf("authoritative worker skill registry loaded active=%d bootstrap=%d", len(registry.Specs), len(bootstrap))
	return nil
}

func bootstrapSkillSpecs(registry *specialists.Registry) ([]specialists.Spec, error) {
	if registry == nil || len(registry.Specs) == 0 {
		return nil, fmt.Errorf("bootstrap worker skill registry is empty")
	}
	ids := make([]string, 0, len(registry.Specs))
	for id := range registry.Specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	specs := make([]specialists.Spec, 0, len(ids))
	for _, id := range ids {
		spec := registry.Specs[id]
		if spec.ID != id {
			return nil, fmt.Errorf("bootstrap worker skill map key %q does not match spec id %q", id, spec.ID)
		}
		if err := spec.Validate(); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}
