package worker

import "time"

type verificationCommandPurpose string

const (
	verificationSetup  verificationCommandPurpose = "setup"
	verificationSyntax verificationCommandPurpose = "syntax"
	verificationTest   verificationCommandPurpose = "test"
	verificationBuild  verificationCommandPurpose = "build"
	verificationConfig verificationCommandPurpose = "config"
)

type testCommand struct {
	Family          string
	Name            string
	Args            []string
	Purpose         verificationCommandPurpose
	Timeout         time.Duration
	RepositoryProof *repositoryGoTestProof
}
