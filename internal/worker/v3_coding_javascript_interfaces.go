package worker

const javaScriptTaskResultType = "{output: string, error: string, exitCode: number, state: Object}"

func javaScriptCommandLineFeatureSignature(name string) string {
	return "function " + name + "(" +
		"/** Arguments exclude the executable; standardInput is complete input text. @type {{arguments: string[], standardInput: string}} */ input, " +
		"/** Prior direct capability results. @type {Object<string, " + javaScriptTaskResultType + ">} */ dependencies" +
		")"
}

func javaScriptCommandLineResultAPI() string {
	return "/**\n" +
		" * @param {" + javaScriptTaskResultType + "} value\n" +
		" * output: user-visible standard-output text; error: standard-error text.\n" +
		" * exitCode: integer process status, zero for success; state: resulting values.\n" +
		" * @returns {" + javaScriptTaskResultType + "}\n" +
		" */\nexport function normalizeTaskResult(value)"
}

func javaScriptCommandLineObservationAPI() string {
	return "/**\n" +
		" * @param {{arguments: string[], standardInput: string}} input\n" +
		" * arguments excludes the executable name; standardInput is the complete input text.\n" +
		" * @param {Object<string, " + javaScriptTaskResultType + ">} dependencies Prior direct capability results.\n" +
		" * @returns {" + javaScriptTaskResultType + "} The observed behavior.\n" +
		" * output is standard-output text; error is standard-error text;\n" +
		" * exitCode is the integer process status (zero for success); state holds resulting values.\n" +
		" */\nfunction run(input, dependencies)"
}
