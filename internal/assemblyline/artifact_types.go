package assemblyline

type ArtifactDisposition string

const (
	ArtifactProtect   ArtifactDisposition = "protect"
	ArtifactRequire   ArtifactDisposition = "require"
	ArtifactReference ArtifactDisposition = "reference"
)

type ArtifactHandling string

const (
	ArtifactPreserveUnchanged ArtifactHandling = "preserve_unchanged"
	ArtifactMustExist         ArtifactHandling = "must_exist"
	ArtifactMentionedOnly     ArtifactHandling = "mentioned_only"
)

type ArtifactDirective struct {
	Token       string
	Disposition ArtifactDisposition
}

type ArtifactIdentity struct {
	Token string
	Value string
}
