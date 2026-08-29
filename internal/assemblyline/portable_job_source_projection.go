package assemblyline

import (
	"fmt"
	"strings"
)

func validatePortableJobSourceProjection(job PortableJob) error {
	if job.SourceProjection == "" {
		return nil
	}
	if job.SourceProjection != strings.TrimSpace(job.SourceProjection) {
		return fmt.Errorf("portable job source projection must be one trimmed adapter identity")
	}
	if err := validatePortableSourceDeclarationProjection(job.SourceProjection); err != nil {
		return fmt.Errorf("portable job source projection: %w", err)
	}
	if job.Kind != WorkFragmentCorrection {
		return fmt.Errorf("portable job source projection requires fragment correction work")
	}
	var input FragmentCorrectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return err
	}
	if input.Language != "" || input.CurrentDeclaration == "" || input.RepairRegion != nil {
		return fmt.Errorf(
			"portable job source projection requires one language-blind declaration correction",
		)
	}
	return nil
}

func validatePortableSourceDeclarationProjection(projection string) error {
	if projection == "go" {
		return nil
	}
	if projection == "typescript" {
		return fmt.Errorf(
			"TypeScript requires signature-bound or repair-region projection metadata",
		)
	}
	_, err := boundedSourceLanguageByID(projection)
	return err
}
