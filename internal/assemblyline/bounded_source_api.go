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

func ProjectJavaScriptFragment(candidate string) (PortableResultProjection, error) {
	return projectBoundedSourceFragment(javaScriptSourceLanguage(), candidate)
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

func ProjectJavaFragment(candidate string) (PortableResultProjection, error) {
	return projectBoundedSourceFragment(javaSourceLanguage(), candidate)
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

func ProjectRustFragment(candidate string) (PortableResultProjection, error) {
	return projectBoundedSourceFragment(rustSourceLanguage(), candidate)
}

func ValidatePHPSourceBlueprint(blueprint SourceBlueprint) error {
	return validateBoundedSourceBlueprint(phpSourceLanguage(), blueprint)
}

func ComposePHPDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	return composeBoundedSourceDocument(phpSourceLanguage(), document, composition)
}

func ValidatePHPFragment(signature, candidate string) (string, error) {
	return validateBoundedSourceFragment(phpSourceLanguage(), signature, candidate)
}

func ProjectPHPFragment(candidate string) (PortableResultProjection, error) {
	return projectBoundedSourceFragment(phpSourceLanguage(), candidate)
}
