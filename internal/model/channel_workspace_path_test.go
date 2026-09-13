package model

import "testing"

func TestChannelWorkspaceRootDoesNotDependOnServerPlatform(t *testing.T) {
	for _, root := range []string{
		"/", "/work/source", "/work/project with spaces", `C:\`, `C:\work\source`,
		`D:\project with spaces`, `\\server\share`, `\\server\share\source`,
	} {
		if err := ValidateChannelWorkspaceRoot(root); err != nil {
			t.Errorf("rejected native absolute path %q: %v", root, err)
		}
	}
	for _, root := range []string{
		"source", "/work/../source", "/work//source", "/work/", `C:source`,
		`C:\work\..\source`, `C:\work\\source`, `C:\work\`, `\\server`,
		`\\server\\source`, `\\server\share\..\source`, `C:/work/source`,
	} {
		if err := ValidateChannelWorkspaceRoot(root); err == nil {
			t.Errorf("accepted noncanonical or relative path %q", root)
		}
	}
}
