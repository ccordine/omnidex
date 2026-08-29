package worker

import "fmt"

func directCodingTypeScriptDeclarationSizeWarning(size int) string {
	if size <= directCodingTypeScriptDeclarationWarningBytes {
		return ""
	}
	return fmt.Sprintf(
		"declaration_size_warning bytes=%d threshold=%d",
		size, directCodingTypeScriptDeclarationWarningBytes,
	)
}
