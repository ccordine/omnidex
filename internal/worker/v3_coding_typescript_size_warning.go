package worker

import "fmt"

func directCodingTypeScriptDeclarationSizeWarning(size int) string {
	if size <= directCodingTypeScriptDeclarationReviewBytes {
		return ""
	}
	return fmt.Sprintf(
		"declaration_size_review bytes=%d threshold=%d",
		size, directCodingTypeScriptDeclarationReviewBytes,
	)
}
