package worker

import "fmt"

func directCodingNodeSandboxPrefix() []string {
	return []string{
		"--permission",
		"--allow-fs-read=.",
		"--disable-proto=throw",
	}
}

func javaScriptNodeTestArgs() []string {
	args := directCodingNodeSandboxPrefix()
	return append(args, "--test-isolation=none", "--test")
}

func javaScriptNodeCheckArgs(artifactPath string) []string {
	args := directCodingNodeSandboxPrefix()
	return append(args, "--check", artifactPath)
}

func validateV3Node(args []string) error {
	if slicesEqualStrings(args, javaScriptNodeTestArgs()) {
		return nil
	}
	if len(args) == len(directCodingNodeSandboxPrefix())+2 &&
		slicesEqualStrings(args[:len(args)-2], directCodingNodeSandboxPrefix()) &&
		args[len(args)-2] == "--check" && javascriptArtifactPath(args[len(args)-1]) {
		return nil
	}
	return fmt.Errorf(
		"command.run permits node only through the code-owned permission sandbox for tests or one JavaScript syntax check",
	)
}
