package queue

import (
	"strings"
	"testing"
)

func TestRepositoryMutationDurableStateRoundTripPreservesPresence(t *testing.T) {
	t.Parallel()
	digest := repositoryMutationDigest("state")
	size := int64(17)
	mode := int32(0o644)

	sha, gotSize, gotMode, err := assignRepositoryMutationSQLState(
		"file_"+digest, "source", true, &digest, &size, &mode,
	)
	if err != nil || sha != digest || gotSize != size || gotMode != 0o644 {
		t.Fatalf("present durable state=%q/%d/%o error=%v", sha, gotSize, gotMode, err)
	}
	sha, gotSize, gotMode, err = assignRepositoryMutationSQLState(
		"file_"+digest, "source", false, nil, nil, nil,
	)
	if err != nil || sha != "" || gotSize != 0 || gotMode != 0 {
		t.Fatalf("absent durable state=%q/%d/%o error=%v", sha, gotSize, gotMode, err)
	}
}

func TestRepositoryMutationDurableStateRejectsPresenceShapeDrift(t *testing.T) {
	t.Parallel()
	digest := repositoryMutationDigest("state-drift")
	size := int64(17)
	mode := int32(0o644)
	tests := []struct {
		name    string
		present bool
		sha     *string
		size    *int64
		mode    *int32
		want    string
	}{
		{name: "absent hash", sha: &digest, want: "nonempty absent"},
		{name: "absent size", size: &size, want: "nonempty absent"},
		{name: "absent mode", mode: &mode, want: "nonempty absent"},
		{name: "present hash missing", present: true, size: &size, mode: &mode, want: "incomplete present"},
		{name: "present size missing", present: true, sha: &digest, mode: &mode, want: "incomplete present"},
		{name: "present mode missing", present: true, sha: &digest, size: &size, want: "incomplete present"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, _, err := assignRepositoryMutationSQLState(
				"file_"+digest, "post-patch", test.present,
				test.sha, test.size, test.mode,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("durable state shape error=%v want %q", err, test.want)
			}
		})
	}
}
