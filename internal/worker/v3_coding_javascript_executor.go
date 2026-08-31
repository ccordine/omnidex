package worker

func newDirectCodingJavaScriptSourceGenerator(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectSourceGenerator, error) {
	return newDirectCodingLanguageSourceGenerator(session, directCodingLanguageSourceConfig{
		Language: "javascript", AdapterID: "javascript",
		ValidateFragment: validateDirectCodingJavaScriptFragment,
	})
}
