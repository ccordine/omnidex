package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

type applicationJobSpecificationProgress struct {
	seen map[string]struct{}
}

func newApplicationJobSpecificationProgress(
	initial assemblyline.ApplicationJobSpecification,
) (*applicationJobSpecificationProgress, error) {
	progress := &applicationJobSpecificationProgress{seen: make(map[string]struct{})}
	if err := progress.Observe(initial); err != nil {
		return nil, err
	}
	return progress, nil
}

func (progress *applicationJobSpecificationProgress) Observe(
	candidate assemblyline.ApplicationJobSpecification,
) error {
	if progress == nil || progress.seen == nil {
		return fmt.Errorf("application job specification progress state is unavailable")
	}
	if err := assemblyline.ValidateApplicationJobSpecification(candidate); err != nil {
		return err
	}
	raw, err := exactjson.Canonical(candidate)
	if err != nil {
		return fmt.Errorf("canonicalize application job specification progress: %w", err)
	}
	identity := string(raw)
	if _, repeated := progress.seen[identity]; repeated {
		return fmt.Errorf("repeated specification state rejected; review and repair entered a cycle")
	}
	progress.seen[identity] = struct{}{}
	return nil
}
