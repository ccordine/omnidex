package worker

func newDirectCodingRustSourceGenerator(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectSourceGenerator, error) {
	return newDirectCodingLanguageSourceGenerator(session, directCodingLanguageSourceConfig{
		Language: "rust", AdapterID: "rust",
		ValidateFragment: validateDirectCodingRustFragment,
	})
}
