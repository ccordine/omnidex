package assemblyline

func ValidateJavaScriptSourceBlueprint(blueprint SourceBlueprint) error {
	return validateBoundedSourceBlueprint(javaScriptSourceLanguage(), blueprint)
}

func ComposeJavaScriptDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	return composeBoundedSourceDocument(javaScriptSourceLanguage(), document, composition)
}

func ValidateJavaScriptFragment(signature, candidate string) (string, error) {
	return validateBoundedSourceFragment(javaScriptSourceLanguage(), signature, candidate)
}

func ExtractJavaScriptSourceBodyResponse(signature, response string) (string, error) {
	return extractBoundedSourceBodyResponse(javaScriptSourceLanguage(), signature, response)
}

func ValidateJavaSourceBlueprint(blueprint SourceBlueprint) error {
	return validateBoundedSourceBlueprint(javaSourceLanguage(), blueprint)
}

func ComposeJavaDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	return composeBoundedSourceDocument(javaSourceLanguage(), document, composition)
}

func ValidateJavaFragment(signature, candidate string) (string, error) {
	return validateBoundedSourceFragment(javaSourceLanguage(), signature, candidate)
}

func ExtractJavaSourceBodyResponse(signature, response string) (string, error) {
	return extractBoundedSourceBodyResponse(javaSourceLanguage(), signature, response)
}

func ValidateRustSourceBlueprint(blueprint SourceBlueprint) error {
	return validateBoundedSourceBlueprint(rustSourceLanguage(), blueprint)
}

func ComposeRustDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	return composeBoundedSourceDocument(rustSourceLanguage(), document, composition)
}

func ValidateRustFragment(signature, candidate string) (string, error) {
	return validateBoundedSourceFragment(rustSourceLanguage(), signature, candidate)
}

func ExtractRustSourceBodyResponse(signature, response string) (string, error) {
	return extractBoundedSourceBodyResponse(rustSourceLanguage(), signature, response)
}
