package worker

type testCommand struct {
	Family          string
	Name            string
	Args            []string
	RepositoryProof *repositoryGoTestProof
}
