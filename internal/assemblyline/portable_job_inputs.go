package assemblyline

type ApplicationClassificationInput struct {
	UserRequest string `json:"user_request"`
}

type ArtifactHandlingInput struct {
	UserRequest string `json:"user_request"`
	Token       string `json:"token"`
}

type FragmentGenerationInput struct {
	Language         string   `json:"language"`
	Dialect          string   `json:"dialect"`
	Signature        string   `json:"signature"`
	Behavior         string   `json:"behavior"`
	Capabilities     []string `json:"capabilities"`
	PermittedSymbols []string `json:"permitted_symbols"`
}

func NewApplicationClassificationJob(input ApplicationClassificationInput) (PortableJob, error) {
	return newPortableJob(WorkApplicationClassify, input)
}

func NewArtifactHandlingJob(input ArtifactHandlingInput) (PortableJob, error) {
	return newPortableJob(WorkArtifactHandling, input)
}

func NewCapabilityRelationJob(input CapabilityRelationInput) (PortableJob, error) {
	return newPortableJob(WorkCapabilityRelation, input)
}

func NewFragmentGenerationJob(input FragmentGenerationInput) (PortableJob, error) {
	return newPortableJob(WorkFragmentGeneration, input)
}
