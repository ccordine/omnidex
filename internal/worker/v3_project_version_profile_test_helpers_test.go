package worker

import "testing"

func requireDirectCodingVersionProfile(
	t *testing.T,
	id string,
) directCodingProjectVersionProfile {
	t.Helper()
	profile, err := directCodingProjectVersionProfileByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func testVersionProfileIDForStack(t *testing.T, stackID string) string {
	t.Helper()
	stack, err := directCodingProjectStackByID(stackID)
	if err != nil {
		t.Fatal(err)
	}
	return stack.DefaultVersionProfileID
}
