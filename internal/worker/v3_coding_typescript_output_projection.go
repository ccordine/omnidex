package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingTypeScriptDeclarationSizeWarning(size int) string {
	if size <= directCodingTypeScriptDeclarationReviewBytes {
		return ""
	}
	return fmt.Sprintf(
		"declaration_size_review bytes=%d threshold=%d",
		size, directCodingTypeScriptDeclarationReviewBytes,
	)
}

func directCodingTypeScriptProjectionWarning(
	projection *assemblyline.PortableResultProjection,
) string {
	if projection == nil || (projection.DiscardedBytes == 0 &&
		projection.RawBytes <= maxTypeScriptOutputReviewBytes) {
		return ""
	}
	return fmt.Sprintf(
		"output_projection kind=%s raw_bytes=%d source_bytes=%d discarded_bytes=%d start_byte=%d end_byte=%d raw_sha256=%s source_sha256=%s",
		projection.Kind, projection.RawBytes, len(projection.Source), projection.DiscardedBytes,
		projection.StartByte, projection.EndByte, projection.SourceResponseSHA256, projection.SourceSHA256,
	)
}

func joinTypedWorkerWarnings(warnings ...string) string {
	joined := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning = strings.TrimSpace(warning); warning != "" {
			joined = append(joined, warning)
		}
	}
	return strings.Join(joined, " ")
}
