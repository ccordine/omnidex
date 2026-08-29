package worker

const (
	directCodingPlainTextEnvironmentID = "plain_text_artifact"
	directCodingDockerCLIImage         = "docker:29.5.1-cli@sha256:b40b3737eb3bf588d25bb856d3564dd3f9fdb32ac2fc19ebe85cc58d761692a5"
)

func directCodingPlainTextEnvironmentSpec() directCodingDockerEnvironmentSpec {
	return directCodingDockerEnvironmentSpec{
		ID: directCodingPlainTextEnvironmentID,
		Dockerfile: "FROM " + directCodingDockerCLIImage + "\n" +
			"LABEL org.omnidex.project-environment=" + directCodingPlainTextEnvironmentID + "\n" +
			"WORKDIR " + directCodingProjectEnvironmentWorkdir + "\n",
		Programs:          []string{"sha256sum", "test"},
		WorkspaceReadOnly: true,
	}
}
