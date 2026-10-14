package modelcontext

import "testing"

func TestPathIdentitiesIgnoreQuotedTrailingOutputEscapes(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"Print exactly `valid\\n`.",
		"Print exactly `completed ID\\r\\n`.",
		"Render \"value\\t\".",
	} {
		if matches := PathIdentities(value, ArtifactIdentityProvenance{}); len(matches) != 0 {
			t.Errorf("PathIdentities(%q) = %#v, want no filesystem identity", value, matches)
		}
	}
}

func TestPathIdentitiesKeepQuotedFilesystemPaths(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"Read `C:\\work\\main.go`.",
		"Read `src\\next.go`.",
		"Read `docs\\guide`.",
		"Read `src/main.go`.",
	} {
		if matches := PathIdentities(value, ArtifactIdentityProvenance{}); len(matches) == 0 {
			t.Errorf("PathIdentities(%q) did not find a filesystem identity", value)
		}
	}
}
