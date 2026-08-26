package worker

import "testing"

func TestArtifactRegistryValidationAcceptsAdditiveQualifiedGoProfile(t *testing.T) {
	profiles := registeredDirectCodingProjectVersionProfiles()
	base := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	future := syntheticFutureGoVersionProfile(base)

	qualifications := registeredDirectCodingParserQualifications()
	futureQualification := cloneTestParserQualification(
		t,
		qualifications,
		base.ParserQualification,
	)
	futureQualification.ID = "go-parser-go1.25-profile-test"
	futureQualification.SourceDialects = []string{future.SourceDialect}
	future.ParserQualification = futureQualification.ID

	profiles = append(profiles, future)
	qualifications = append(qualifications, futureQualification)
	if err := validateDirectCodingArtifactRegistriesFrom(
		registeredDirectCodingArtifactAdapters(),
		profiles,
		registeredDirectCodingProjectStacks(),
		qualifications,
	); err != nil {
		t.Fatalf("full registry gate rejected an additive parser-qualified Go profile: %v", err)
	}
}

func cloneTestParserQualification(
	t *testing.T,
	qualifications []directCodingParserQualification,
	id string,
) directCodingParserQualification {
	t.Helper()
	for _, qualification := range qualifications {
		if qualification.ID != id {
			continue
		}
		qualification.SourceDialects = append([]string(nil), qualification.SourceDialects...)
		qualification.Probes = append([]directCodingParserProbe(nil), qualification.Probes...)
		return qualification
	}
	t.Fatalf("parser qualification %q is not registered", id)
	return directCodingParserQualification{}
}
