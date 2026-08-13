package assemblyline

type ArtifactDisposition string

const (
	ArtifactProtect          ArtifactDisposition = "protect"
	ArtifactRequire          ArtifactDisposition = "require"
	ArtifactReference        ArtifactDisposition = "reference"
	ArtifactForbid           ArtifactDisposition = "forbid"
	ArtifactAbsenceCandidate ArtifactDisposition = "absence_candidate"
)

type ArtifactHandling string

const (
	ArtifactPreserveUnchanged        ArtifactHandling = "preserve_unchanged"
	ArtifactMustExist                ArtifactHandling = "must_exist"
	ArtifactMustBeAbsent             ArtifactHandling = "must_be_absent"
	ArtifactPossibleAbsenceCandidate ArtifactHandling = "possible_absence_candidate"
	ArtifactMentionedOnly            ArtifactHandling = "mentioned_only"
)

type ArtifactDirective struct {
	Token       string
	Disposition ArtifactDisposition
}

type ArtifactIdentity struct {
	Token string
	Value string
}
